package lotw

import (
	"context"
	"testing"

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
