package eqsl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreIsAG(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	require.NoError(t, s.replace(ctx, []string{"W1AW", "KH6/K1ABC"}, Meta{SyncedAt: utc(2026, 11, 8, 0, 0, 0)}))

	tests := []struct {
		name string
		call string
		want bool
	}{
		{name: "exact", call: "W1AW", want: true},
		{name: "lower case and spaces", call: "  w1aw ", want: true},
		{name: "prefix and suffix normalized to base", call: "ve3/w1aw/p", want: true},
		{name: "slashed call stored verbatim falls back to exact match", call: "kh6/k1abc", want: true},
		{name: "base of a slashed entry alone is not listed", call: "K1ABC"},
		{name: "not a member", call: "K9XYZ"},
		{name: "empty", call: ""},
		{name: "only modifiers", call: "/P"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.IsAG(ctx, tt.call)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStoreReplaceRemovesOldRows(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	require.NoError(t, s.replace(ctx, []string{"W1AW", "K1ABC"}, Meta{SyncedAt: utc(2026, 1, 2, 0, 0, 0)}))

	require.NoError(t, s.replace(ctx, []string{"W1AW", "W1AW"}, Meta{ETag: `"v2"`, SyncedAt: utc(2026, 2, 2, 0, 0, 0)}))

	ok, err := s.IsAG(ctx, "K1ABC")
	require.NoError(t, err)
	assert.False(t, ok, "members missing from the new list are removed")
	ok, err = s.IsAG(ctx, "W1AW")
	require.NoError(t, err)
	assert.True(t, ok, "a duplicate in the list is stored once")
}

func TestStoreLastSync(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	_, ok, err := s.LastSync(ctx)
	require.NoError(t, err)
	assert.False(t, ok, "never synced")

	meta := Meta{ETag: `"abc"`, LastModified: "Sun, 08 Nov 2026 10:00:00 GMT", SyncedAt: utc(2026, 11, 8, 10, 5, 0)}
	require.NoError(t, s.replace(ctx, []string{"W1AW"}, meta))

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
	found, err := s.IsAG(ctx, "W1AW")
	require.NoError(t, err)
	assert.True(t, found, "touch keeps the members")
}

func TestStoreCancelledContext(t *testing.T) {
	s := newTestStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := s.IsAG(ctx, "W1AW")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	require.Error(t, s.replace(ctx, nil, Meta{}))
	_, _, err = s.LastSync(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
