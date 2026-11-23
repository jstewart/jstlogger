package callinfo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/callbook"
	cbmock "jstlogger/internal/callbook/mock"
	"jstlogger/internal/logbook"
)

// newStore opens a logbook in a temp dir holding qsos.
func newStore(t *testing.T, qsos ...logbook.QSO) *logbook.SQLiteStore {
	t.Helper()
	s, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, s.Close()) })
	for _, q := range qsos {
		_, err := s.Add(t.Context(), q)
		require.NoError(t, err)
	}
	return s
}

// looker returns a callbook that answers call with r and err.
func looker(t *testing.T, call string, r callbook.Result, err error) *cbmock.MockLooker {
	t.Helper()
	l := cbmock.NewMockLooker(gomock.NewController(t))
	l.EXPECT().Lookup(gomock.Any(), call).Return(r, err)
	return l
}

// earlierQSOs are worked before with W1AW, the latest portable.
var earlierQSOs = []logbook.QSO{
	{
		Call: "W1AW", StartUTC: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Band: "20m", Mode: "SSB",
		Name: "Hiram", QTH: "Newington", Gridsquare: "FN31pr", State: "CT", Cnty: "CT,Hartford",
		Fields: map[string]string{"EMAIL": "w1aw@arrl.org"},
	},
	{
		Call: "W1AW/P", StartUTC: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), Band: "40m", Mode: "CW",
		Name: "Hiram P", QTH: "Somewhere", Gridsquare: "FN42", Fields: map[string]string{"EMAIL": "p@arrl.org"},
	},
}

func TestLookerDetails(t *testing.T) {
	qrz := callbook.Result{
		Call: "W1AW", Name: "Hiram Maxim", QTH: "Newington", Grid: "FN31pr", State: "CT", County: "Hartford",
		Email: "w1aw@arrl.org", Service: callbook.ServiceQRZ, Limited: true,
	}
	notFound := fmt.Errorf("qrz: %w", callbook.ErrNotFound)
	authFailed := fmt.Errorf("qrz: %w", callbook.ErrAuth)

	tests := []struct {
		name    string
		call    string
		result  callbook.Result
		err     error
		noCB    bool
		want    Details
		wantErr error
	}{
		{
			name: "callbook", call: "W1AW", result: qrz,
			want: Details{
				Name: "Hiram Maxim", QTH: "Newington", Grid: "FN31pr", State: "CT", Cnty: "CT,Hartford",
				Email: "w1aw@arrl.org", Source: callbook.ServiceQRZ, Limited: true,
			},
		},
		{
			name: "county without state is left out", call: "W1AW",
			result: callbook.Result{Name: "Hiram", County: "Hartford", Service: callbook.ServiceHamQTH},
			want:   Details{Name: "Hiram", Source: callbook.ServiceHamQTH},
		},
		{name: "not found has no details", call: "W1AW", err: notFound, want: Details{}},
		{
			name: "failure falls back to the latest QSO with the base call", call: "W1AW", err: authFailed,
			want:    Details{Name: "Hiram P", Email: "p@arrl.org", Source: SourceLogbook},
			wantErr: callbook.ErrAuth,
		},
		{
			name: "no callbook, exact call gives the location", call: "W1AW/P", noCB: true,
			want: Details{Name: "Hiram P", QTH: "Somewhere", Grid: "FN42", Email: "p@arrl.org", Source: SourceLogbook},
		},
		{name: "no callbook, never worked", call: "K1ABC", noCB: true, want: Details{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := Looker{Logbook: newStore(t, earlierQSOs...)}
			if !tt.noCB {
				l.Callbook = looker(t, tt.call, tt.result, tt.err)
			}
			got, err := l.Details(t.Context(), tt.call)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLookerDetailsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	l := Looker{
		Callbook: looker(t, "W1AW", callbook.Result{}, fmt.Errorf("qrz: %w", context.Canceled)),
		Logbook:  newStore(t, earlierQSOs...),
	}
	got, err := l.Details(ctx, "W1AW")
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, Details{}, got, "a canceled lookup does not fall back")
}

func TestLookerDetailsCallbookTimeout(t *testing.T) {
	cb := cbmock.NewMockLooker(gomock.NewController(t))
	cb.EXPECT().Lookup(gomock.Any(), "W1AW").DoAndReturn(func(ctx context.Context, call string) (callbook.Result, error) {
		<-ctx.Done()
		return callbook.Result{}, fmt.Errorf("lookup %s: %w", call, ctx.Err())
	})
	l := Looker{Callbook: cb, Logbook: newStore(t, earlierQSOs...), CallbookTimeout: 10 * time.Millisecond}

	got, err := l.Details(t.Context(), "W1AW")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, "Hiram P", got.Name, "the logbook still answers")
}

func TestLookerDetailsNothingConfigured(t *testing.T) {
	got, err := Looker{}.Details(t.Context(), "W1AW")
	require.NoError(t, err)
	assert.Equal(t, Details{}, got)
}

func TestNotFoundOnly(t *testing.T) {
	notFound := fmt.Errorf("qrz: lookup W1AW: %w", callbook.ErrNotFound)
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"not found", notFound, true},
		{"every service not found", errors.Join(notFound, fmt.Errorf("hamqth: %w", callbook.ErrNotFound)), true},
		{"one service failed", errors.Join(notFound, fmt.Errorf("hamqth: %w", callbook.ErrAuth)), false},
		{"nested join", errors.Join(errors.Join(notFound, notFound), notFound), true},
		{"other error", callbook.ErrRateLimit, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, notFoundOnly(tt.err))
		})
	}
}
