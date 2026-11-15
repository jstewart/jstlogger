package dxcc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreTableFallsBackToDefault(t *testing.T) {
	s := newTestStore(t)

	table, err := s.Table(t.Context())

	require.NoError(t, err)
	assert.Same(t, Default(), table, "nothing stored uses the embedded snapshot")
	_, ok, err := s.LastSync(t.Context())
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestStoreReplaceAndTable(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	meta := Meta{ETag: `"v1"`, LastModified: "Sat, 14 Nov 2026 14:38:28 GMT", SyncedAt: utc(2026, 11, 15, 10, 0, 0)}
	require.NoError(t, s.replace(ctx, []byte("K,United States,291,NA,5,8,37.60,91.87,5.0,K;\n"), meta))
	require.NoError(t, s.replace(ctx, fixtureCSV(t), meta))

	table, err := s.Table(ctx)

	require.NoError(t, err)
	got, ok := table.Lookup("KH6ABC")
	require.True(t, ok, "the second replace wins")
	assert.Equal(t, hawaii, got)
	_, ok = table.Lookup("JA1ABC")
	assert.False(t, ok, "the stored file, not the embedded snapshot")
	gotMeta, ok, err := s.LastSync(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, meta, gotMeta)
}

func TestStoreTouchKeepsFile(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	require.NoError(t, s.replace(ctx, fixtureCSV(t), Meta{ETag: `"v1"`, SyncedAt: utc(2026, 11, 15, 10, 0, 0)}))
	later := Meta{ETag: `"v1"`, SyncedAt: utc(2026, 11, 22, 10, 0, 0)}

	require.NoError(t, s.touch(ctx, later))

	meta, _, err := s.LastSync(ctx)
	require.NoError(t, err)
	assert.Equal(t, later, meta)
	table, err := s.Table(ctx)
	require.NoError(t, err)
	_, ok := table.Lookup("KH6ABC")
	assert.True(t, ok)
}

func TestStoreTableReportsCorruptFile(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	require.NoError(t, s.replace(ctx, []byte("not,a,country,file\n"), Meta{SyncedAt: utc(2026, 11, 15, 10, 0, 0)}))

	_, err := s.Table(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dxcc: load country file")
}
