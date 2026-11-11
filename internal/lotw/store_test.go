package lotw

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreLastUpload(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	w1aw := utc(2026, 11, 1, 12, 0, 0)
	slashed := utc(2026, 10, 1, 8, 30, 0)
	require.NoError(t, s.replace(ctx, []User{
		{Call: "W1AW", LastUpload: w1aw},
		{Call: "KH6/K1ABC", LastUpload: slashed},
	}, Meta{SyncedAt: utc(2026, 11, 8, 0, 0, 0)}))

	tests := []struct {
		name string
		call string
		want bool
	}{
		{name: "exact", call: "W1AW", want: true},
		{name: "lower case and spaces", call: "  w1aw ", want: true},
		{name: "prefix and suffix normalized to base", call: "ve3/w1aw/p", want: true},
		{name: "not a user", call: "K9XYZ"},
		{name: "empty", call: ""},
		{name: "only modifiers", call: "/P"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := s.LastUpload(ctx, tt.call)

			require.NoError(t, err)
			assert.Equal(t, tt.want, ok)
			if tt.want {
				assert.True(t, w1aw.Equal(got), "got %v", got)
			} else {
				assert.True(t, got.IsZero())
			}
		})
	}

	t.Run("slashed call stored verbatim falls back to exact match", func(t *testing.T) {
		got, ok, err := s.LastUpload(ctx, "kh6/k1abc")

		require.NoError(t, err)
		assert.True(t, ok)
		assert.True(t, slashed.Equal(got))
		assert.Equal(t, "UTC", got.Location().String())
	})
}

func TestStoreReplaceRemovesOldRows(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	require.NoError(t, s.replace(ctx, []User{
		{Call: "W1AW", LastUpload: utc(2026, 1, 1, 0, 0, 0)},
		{Call: "K1ABC", LastUpload: utc(2026, 1, 1, 0, 0, 0)},
	}, Meta{SyncedAt: utc(2026, 1, 2, 0, 0, 0)}))

	require.NoError(t, s.replace(ctx, []User{
		{Call: "W1AW", LastUpload: utc(2026, 2, 1, 0, 0, 0)},
	}, Meta{ETag: `"v2"`, SyncedAt: utc(2026, 2, 2, 0, 0, 0)}))

	_, ok, err := s.LastUpload(ctx, "K1ABC")
	require.NoError(t, err)
	assert.False(t, ok, "rows missing from the new feed are removed")
	got, ok, err := s.LastUpload(ctx, "W1AW")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, utc(2026, 2, 1, 0, 0, 0).Equal(got))
}

func TestStoreLastSync(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	_, ok, err := s.LastSync(ctx)
	require.NoError(t, err)
	assert.False(t, ok, "never synced")

	meta := Meta{ETag: `"abc"`, LastModified: "Sun, 08 Nov 2026 10:00:00 GMT", SyncedAt: utc(2026, 11, 8, 10, 5, 0)}
	require.NoError(t, s.replace(ctx, []User{{Call: "W1AW", LastUpload: utc(2026, 11, 1, 0, 0, 0)}}, meta))

	got, ok, err := s.LastSync(ctx)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, meta, got)

	touched := meta
	touched.SyncedAt = utc(2026, 11, 9, 0, 0, 0)
	require.NoError(t, s.touch(ctx, touched))

	got, ok, err = s.LastSync(ctx)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, touched, got)
	_, found, err := s.LastUpload(ctx, "W1AW")
	require.NoError(t, err)
	assert.True(t, found, "touch keeps the users")
}

func TestStoreCancelledContext(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, err := s.LastUpload(ctx, "W1AW")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	require.Error(t, s.replace(ctx, nil, Meta{}))
}

func TestStoreLastUploads(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	w1aw := utc(2026, 11, 1, 12, 0, 0)
	slashed := utc(2026, 10, 1, 8, 30, 0)
	k1abc := utc(2025, 1, 1, 0, 0, 0)
	require.NoError(t, s.replace(ctx, []User{
		{Call: "W1AW", LastUpload: w1aw},
		{Call: "KH6/N0XYZ", LastUpload: slashed},
		{Call: "K1ABC", LastUpload: k1abc},
	}, Meta{SyncedAt: utc(2026, 11, 8, 0, 0, 0)}))

	tests := []struct {
		name  string
		calls []string
		want  map[string]time.Time
	}{
		{name: "none", calls: nil, want: map[string]time.Time{}},
		{name: "exact", calls: []string{"W1AW"}, want: map[string]time.Time{"W1AW": w1aw}},
		{
			name:  "keyed by the calls as given",
			calls: []string{"  w1aw ", "ve3/w1aw/p", "K1ABC"},
			want:  map[string]time.Time{"  w1aw ": w1aw, "ve3/w1aw/p": w1aw, "K1ABC": k1abc},
		},
		{
			name:  "slashed call stored verbatim falls back to exact match",
			calls: []string{"kh6/n0xyz"},
			want:  map[string]time.Time{"kh6/n0xyz": slashed},
		},
		{
			name:  "non-users and calls without a base are left out",
			calls: []string{"K9XYZ", "", "/P", "W1AW"},
			want:  map[string]time.Time{"W1AW": w1aw},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.LastUploads(ctx, tt.calls)

			require.NoError(t, err)
			require.Len(t, got, len(tt.want))
			for call, at := range tt.want {
				require.Contains(t, got, call)
				assert.True(t, at.Equal(got[call]), "%s: got %v, want %v", call, got[call], at)
				assert.Equal(t, "UTC", got[call].Location().String())
			}
		})
	}
}

func TestStoreLastUploadsManyCalls(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	at := utc(2026, 11, 1, 12, 0, 0)
	var users []User
	var calls []string
	for i := range 2500 {
		call := fmt.Sprintf("K%dAB", i)
		calls = append(calls, call)
		if i%2 == 0 {
			users = append(users, User{Call: call, LastUpload: at})
		}
	}
	require.NoError(t, s.replace(ctx, users, Meta{SyncedAt: at}))

	got, err := s.LastUploads(ctx, calls)

	require.NoError(t, err)
	assert.Len(t, got, len(users), "more calls than one query takes are looked up in chunks")
	assert.Contains(t, got, "K2498AB")
	assert.NotContains(t, got, "K2499AB")
}

func TestStoreLastUploadsCancelledContext(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := s.LastUploads(ctx, []string{"W1AW"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "lotw: ")
}
