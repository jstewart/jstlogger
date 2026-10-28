package logbook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedBare adds n QSOs with no tags and returns their IDs.
func seedBare(t *testing.T, s *SQLiteStore, n int) []int64 {
	t.Helper()
	ids := make([]int64, 0, n)
	for i := range n {
		q := seed(t, s, QSO{StartUTC: queryT0.Add(time.Duration(i) * time.Minute), Call: "W1AW"})[0]
		ids = append(ids, q.ID)
	}
	return ids
}

// tagsOf returns the tags of the QSO with the given ID.
func tagsOf(t *testing.T, s *SQLiteStore, id int64) []string {
	t.Helper()
	q, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	return q.Tags
}

// tagNames returns the names of every tag in s, in Tags order.
func tagNames(t *testing.T, s *SQLiteStore) []string {
	t.Helper()
	tags, err := s.Tags(t.Context())
	require.NoError(t, err)
	names := make([]string, 0, len(tags))
	for _, tg := range tags {
		assert.NotZero(t, tg.ID)
		names = append(names, tg.Name)
	}
	return names
}

func TestSQLiteStoreTagMultipleQSOs(t *testing.T) {
	s := newTestStore(t)
	ids := seedBare(t, s, 3)

	require.NoError(t, s.Tag(t.Context(), ids[:2], "pota", " DX ", "dx", ""))

	assert.Equal(t, []string{"DX", "pota"}, tagsOf(t, s, ids[0]))
	assert.Equal(t, []string{"DX", "pota"}, tagsOf(t, s, ids[1]))
	assert.Nil(t, tagsOf(t, s, ids[2]))
	assert.Equal(t, 4, countRows(t, s, "qso_tag"))
}

func TestSQLiteStoreTagIdempotent(t *testing.T) {
	s := newTestStore(t)
	ids := seedBare(t, s, 2)

	require.NoError(t, s.Tag(t.Context(), ids, "pota"))
	require.NoError(t, s.Tag(t.Context(), ids, "pota"))

	assert.Equal(t, 2, countRows(t, s, "qso_tag"))
	assert.Equal(t, 1, countRows(t, s, "tag"))
}

func TestSQLiteStoreTagReusesExistingTagIgnoringCase(t *testing.T) {
	s := newTestStore(t)
	ids := seedBare(t, s, 2)

	require.NoError(t, s.Tag(t.Context(), ids[:1], "pota"))
	require.NoError(t, s.Tag(t.Context(), ids[1:], "POTA"))

	assert.Equal(t, []string{"pota"}, tagNames(t, s), "existing spelling is kept")
	assert.Equal(t, []string{"pota"}, tagsOf(t, s, ids[1]))
}

func TestSQLiteStoreTagUnknownIDRollsBack(t *testing.T) {
	s := newTestStore(t)
	ids := seedBare(t, s, 1)

	err := s.Tag(t.Context(), []int64{ids[0], 999}, "new")

	require.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, tagsOf(t, s, ids[0]), "no partial links")
	assert.Equal(t, 0, countRows(t, s, "qso_tag"))
	assert.Equal(t, 0, countRows(t, s, "tag"), "no tag created")
}

func TestSQLiteStoreTagNoOp(t *testing.T) {
	s := newTestStore(t)
	ids := seedBare(t, s, 1)

	require.NoError(t, s.Tag(t.Context(), nil, "pota"))
	require.NoError(t, s.Tag(t.Context(), ids))
	require.NoError(t, s.Tag(t.Context(), ids, " ", ""))
	require.NoError(t, s.Untag(t.Context(), nil, "pota"))
	require.NoError(t, s.Untag(t.Context(), ids))

	assert.Equal(t, 0, countRows(t, s, "tag"))
}

func TestSQLiteStoreUntag(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	ids := seedBare(t, s, 2)
	require.NoError(t, s.Tag(ctx, ids, "pota"))
	require.NoError(t, s.Tag(ctx, ids[:1], "solo"))

	require.NoError(t, s.Untag(ctx, ids[:1], "POTA", "Solo"))

	assert.Nil(t, tagsOf(t, s, ids[0]))
	assert.Equal(t, []string{"pota"}, tagsOf(t, s, ids[1]), "other QSO keeps its tag")
	assert.Equal(t, []string{"pota"}, tagNames(t, s), "orphaned tag row is deleted, shared one kept")
}

func TestSQLiteStoreUntagUnknownIsNotAnError(t *testing.T) {
	s := newTestStore(t)
	ids := seedBare(t, s, 1)
	require.NoError(t, s.Tag(t.Context(), ids, "pota"))

	require.NoError(t, s.Untag(t.Context(), []int64{999}, "pota"))
	require.NoError(t, s.Untag(t.Context(), ids, "nope"))

	assert.Equal(t, []string{"pota"}, tagsOf(t, s, ids[0]))
}

func TestSQLiteStoreTagsOrderedIgnoringCase(t *testing.T) {
	s := newTestStore(t)
	ids := seedBare(t, s, 1)

	assert.Empty(t, tagNames(t, s))
	require.NoError(t, s.Tag(t.Context(), ids, "b", "A", "c", "Ba"))

	assert.Equal(t, []string{"A", "b", "Ba", "c"}, tagNames(t, s))
}
