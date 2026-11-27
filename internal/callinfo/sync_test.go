package callinfo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/callbook"
	"jstlogger/internal/logbook"
)

// countingStore counts the updates made to the store it wraps.
type countingStore struct {
	logbook.Store
	updates int
}

func (s *countingStore) Update(ctx context.Context, q logbook.QSO) error {
	s.updates++
	return s.Store.Update(ctx, q)
}

// syncStore returns a store holding qsos that counts updates, and the ID
// of the last QSO added.
func syncStore(t *testing.T, qsos ...logbook.QSO) (*countingStore, int64) {
	t.Helper()
	s := newStore(t)
	var id int64
	for _, q := range qsos {
		stored, err := s.Add(t.Context(), q)
		require.NoError(t, err)
		id = stored.ID
	}
	return &countingStore{Store: s}, id
}

// offlineQSO is a QSO logged while the callbook was unreachable.
func offlineQSO() logbook.QSO {
	return logbook.QSO{
		Call: "W1AW", StartUTC: time.Date(2026, 11, 23, 13, 1, 2, 0, time.UTC),
		Band: "20m", Mode: "FT8",
	}
}

// hiram is the callbook's answer for W1AW.
var hiram = callbook.Result{
	Call: "W1AW", Name: "Hiram Maxim", QTH: "Newington", Grid: "FN31pr", State: "CT", County: "Hartford",
	Service: callbook.ServiceQRZ,
}

func TestSyncUpdated(t *testing.T) {
	s, id := syncStore(t, offlineQSO())
	l := Looker{DXCC: stubDXCC{usa}, Callbook: looker(t, "W1AW", hiram, nil)}

	got, err := l.Sync(t.Context(), s, id)
	require.NoError(t, err)
	assert.Equal(t, SyncUpdated, got)
	assert.Equal(t, 1, s.updates)

	q, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, "Hiram Maxim", q.Name)
	assert.Equal(t, "FN31PR", q.Gridsquare, "the store normalizes the grid")
	assert.Equal(t, "Newington", q.QTH)
	assert.Equal(t, "CT,Hartford", q.Cnty)
	assert.Equal(t, 291, q.DXCC.V)
	assert.Equal(t, "NA", q.Cont)
}

func TestSyncKeepsHandEnteredValues(t *testing.T) {
	in := offlineQSO()
	in.Name = "Hi"
	s, id := syncStore(t, in)
	l := Looker{Callbook: looker(t, "W1AW", hiram, nil)}

	got, err := l.Sync(t.Context(), s, id)
	require.NoError(t, err)
	assert.Equal(t, SyncUpdated, got)

	q, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, "Hi", q.Name, "a hand-entered name is kept")
	assert.Equal(t, "FN31PR", q.Gridsquare, "an empty grid is filled")
}

func TestSyncUnchanged(t *testing.T) {
	in := offlineQSO()
	in.Name, in.QTH, in.Gridsquare, in.State, in.Cnty = "Hi", "Here", "FN31", "CT", "CT,Hartford"
	s, id := syncStore(t, in)
	before, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	l := Looker{Callbook: looker(t, "W1AW", hiram, nil)}

	got, err := l.Sync(t.Context(), s, id)
	require.NoError(t, err)
	assert.Equal(t, SyncUnchanged, got)
	assert.Zero(t, s.updates, "nothing to change is not saved")

	after, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestSyncNotFound(t *testing.T) {
	s, id := syncStore(t, offlineQSO())
	l := Looker{DXCC: stubDXCC{usa}, Callbook: looker(t, "W1AW", callbook.Result{}, fmt.Errorf("qrz: %w", callbook.ErrNotFound))}

	got, err := l.Sync(t.Context(), s, id)
	require.NoError(t, err)
	assert.Equal(t, SyncNotFound, got)
	assert.Zero(t, s.updates)

	q, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	assert.False(t, q.DXCC.Valid, "a QSO not found is left untouched")
	assert.Empty(t, q.Cont)
	assert.Empty(t, q.Country)
}

func TestSyncNoBaseCallIsNotFound(t *testing.T) {
	in := offlineQSO()
	in.Call = "/P"
	s, id := syncStore(t, in)
	l := Looker{DXCC: stubDXCC{usa}}

	got, err := l.Sync(t.Context(), s, id)
	require.NoError(t, err)
	assert.Equal(t, SyncNotFound, got)
	assert.Zero(t, s.updates)
}

func TestSyncNeverUsesTheLogbook(t *testing.T) {
	older := offlineQSO()
	older.StartUTC = older.StartUTC.AddDate(-1, 0, 0)
	older.Name = "Hiram"
	s, id := syncStore(t, older, offlineQSO())
	l := Looker{
		Callbook: looker(t, "W1AW", callbook.Result{}, fmt.Errorf("qrz: %w", callbook.ErrNotFound)),
		Logbook:  s,
	}

	got, err := l.Sync(t.Context(), s, id)
	require.NoError(t, err)
	assert.Equal(t, SyncNotFound, got)

	q, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	assert.Empty(t, q.Name, "the earlier QSO's name is not used")
}

func TestSyncNetworkError(t *testing.T) {
	s, id := syncStore(t, offlineQSO())
	before, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	netErr := &callbook.NetworkError{Service: callbook.ServiceQRZ, Err: errors.New("connection refused")}
	l := Looker{DXCC: stubDXCC{usa}, Callbook: looker(t, "W1AW", callbook.Result{}, netErr), Logbook: s}

	got, err := l.Sync(t.Context(), s, id)
	require.Error(t, err)
	assert.Equal(t, SyncFailed, got)
	assert.True(t, Unreachable(err))
	assert.True(t, StopsSync(err))
	assert.Zero(t, s.updates)

	after, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestSyncPreservesTagsAndConfirmations(t *testing.T) {
	in := offlineQSO()
	in.Tags = []string{"pota", "contest"}
	in.Comment = "nice signal"
	in.Fields = map[string]string{"SIG": "POTA"}
	in.Confirmations = []logbook.Confirmation{{Service: logbook.ServiceLoTW, Sent: "Y"}}
	s, id := syncStore(t, in)
	before, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	l := Looker{Callbook: looker(t, "W1AW", hiram, nil)}

	got, err := l.Sync(t.Context(), s, id)
	require.NoError(t, err)
	require.Equal(t, SyncUpdated, got)

	after, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	assert.ElementsMatch(t, before.Tags, after.Tags)
	assert.Equal(t, before.Confirmations, after.Confirmations)
	assert.Equal(t, "nice signal", after.Comment)
	assert.Equal(t, "POTA", after.Fields["SIG"])
	assert.Equal(t, before.UUID, after.UUID)
}

func TestSyncUnknownID(t *testing.T) {
	s, _ := syncStore(t)
	got, err := Looker{}.Sync(t.Context(), s, 42)
	require.ErrorIs(t, err, logbook.ErrNotFound)
	assert.Equal(t, SyncFailed, got)
}

func TestUnreachableAndStopsSync(t *testing.T) {
	netErr := &callbook.NetworkError{Service: callbook.ServiceQRZ, Err: errors.New("no route")}
	notFound := fmt.Errorf("hamqth: %w", callbook.ErrNotFound)
	tests := []struct {
		name        string
		err         error
		unreachable bool
		stops       bool
	}{
		{"nil", nil, false, false},
		{"network error", netErr, true, true},
		{"wrapped network error", fmt.Errorf("look up W1AW: %w", netErr), true, true},
		{"joined network error", errors.Join(notFound, netErr), true, true},
		{"deadline", fmt.Errorf("qrz: %w", context.DeadlineExceeded), true, true},
		{"joined deadline", errors.Join(notFound, context.DeadlineExceeded), true, true},
		{"rate limit", fmt.Errorf("qrz: %w", callbook.ErrRateLimit), false, true},
		{"joined auth", errors.Join(notFound, fmt.Errorf("qrz: %w", callbook.ErrAuth)), false, true},
		{"canceled", fmt.Errorf("look up W1AW: %w", context.Canceled), false, true},
		{"not found", notFound, false, false},
		{"plain error", errors.New("boom"), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.unreachable, Unreachable(tt.err), "Unreachable")
			assert.Equal(t, tt.stops, StopsSync(tt.err), "StopsSync")
		})
	}
}

func TestSyncRemovesPendingTag(t *testing.T) {
	updated := offlineQSO()
	unchanged := offlineQSO()
	unchanged.Name, unchanged.QTH, unchanged.Gridsquare, unchanged.State, unchanged.Cnty = "Hi", "Here", "FN31", "CT", "CT,Hartford"
	tests := []struct {
		name string
		in   logbook.QSO
		res  callbook.Result
		err  error
		want SyncOutcome
		kept bool
	}{
		{"updated", updated, hiram, nil, SyncUpdated, false},
		{"unchanged", unchanged, hiram, nil, SyncUnchanged, false},
		{"not found", updated, callbook.Result{}, callbook.ErrNotFound, SyncNotFound, true},
		{"unreachable", updated, callbook.Result{}, &callbook.NetworkError{Service: callbook.ServiceQRZ, Err: errors.New("no route to host")}, SyncFailed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			in.Tags = []string{PendingTag, "pota"}
			s, id := syncStore(t, in)
			l := Looker{Callbook: looker(t, "W1AW", tt.res, tt.err)}

			got, _ := l.Sync(t.Context(), s, id)
			assert.Equal(t, tt.want, got)

			q, err := s.Get(t.Context(), id)
			require.NoError(t, err)
			if tt.kept {
				assert.ElementsMatch(t, []string{PendingTag, "pota"}, q.Tags)
			} else {
				assert.Equal(t, []string{"pota"}, q.Tags, "a synced QSO is no longer pending")
			}
		})
	}
}
