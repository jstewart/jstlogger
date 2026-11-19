package tui

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/dxcc"
	"jstlogger/internal/qsoentry"
)

// testCTY is a country file with the United States and Canada.
const testCTY = `K,United States,291,NA,5,8,37.60,91.87,5.0,AA K N W;
VE,Canada,1,NA,5,9,44.35,78.75,5.0,VA VE VE2[4];
`

// newDXCCTable returns the DXCC table parsed from testCTY.
func newDXCCTable(t *testing.T) *dxcc.Table {
	t.Helper()
	tbl, err := dxcc.Parse(strings.NewReader(testCTY))
	require.NoError(t, err)
	return tbl
}

// newEntryDXCCModel returns a model with the New QSO screen open, focus on
// the call and the DXCC table of testCTY.
func newEntryDXCCModel(t *testing.T) Model {
	t.Helper()
	m := newEntryModel(t, newEntryStore(t, nil), WithDXCC(newDXCCTable(t)))
	return focusOn(t, m, qsoentry.Call)
}

// dxccValues returns the DXCC, continent, CQ zone and ITU zone in the form.
func dxccValues(m Model) [4]string {
	return [4]string{
		entryValue(m, qsoentry.DXCC), entryValue(m, qsoentry.Cont),
		entryValue(m, qsoentry.CQZ), entryValue(m, qsoentry.ITUZ),
	}
}

func TestEntryDXCCFillsIn(t *testing.T) {
	m := typeCall(t, newEntryDXCCModel(t), "W1AW")
	assert.Equal(t, [4]string{"291", "NA", "5", "8"}, dxccValues(m), "the call's entity is filled in")

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "VE3ABC"})
	assert.Equal(t, [4]string{"1", "NA", "5", "9"}, dxccValues(m), "a changed call updates the entity")
}

func TestEntryDXCCKeepsTypedValues(t *testing.T) {
	m := typeCall(t, newEntryDXCCModel(t), "W1AW")
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.CQZ: "4"})
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "VE3ABC"})

	assert.Equal(t, [4]string{"1", "NA", "4", "9"}, dxccValues(m), "a typed CQ zone is kept")
}

func TestEntryDXCCClearsWithoutEntity(t *testing.T) {
	for _, call := range []string{"W1AW/MM", "W1AW/AM", "ZZ1ZZ", ""} {
		t.Run(call, func(t *testing.T) {
			m := typeCall(t, newEntryDXCCModel(t), "W1AW")
			m = fill(t, m, map[qsoentry.Field]string{qsoentry.ITUZ: "7"})
			m = focusOn(t, m, qsoentry.Call)
			m = press(t, m, clearKey)
			m = typeCall(t, m, call)

			assert.Equal(t, [4]string{"", "", "", "7"}, dxccValues(m),
				"filled values are cleared and a typed ITU zone is kept")
			assert.NotContains(t, entryCallLine(t, m), "United States", "no entity is shown")
		})
	}
}

func TestEntryDXCCShowsEntity(t *testing.T) {
	m := typeCall(t, newEntryDXCCModel(t), "W1AW")
	assert.Contains(t, entryCallLine(t, m), "W1AW", "the call is shown")
	assert.Contains(t, entryCallLine(t, m), "United States · NA · CQ 5 · ITU 8")
}

func TestEntryDXCCWithoutLookup(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Call)
	m = typeCall(t, m, "W1AW")

	assert.Equal(t, [4]string{}, dxccValues(m), "nothing is filled in")
	assert.NotContains(t, entryCallLine(t, m), "United States", "no entity is shown")
}

func TestEditDXCCFollowsCall(t *testing.T) {
	m := openEditModel(t, newEditStore(t, editedQSO(t)), WithDXCC(newDXCCTable(t)))
	require.Equal(t, [4]string{"291", "NA", "5", "8"}, dxccValues(m))

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "VE3ABC"})
	assert.Equal(t, [4]string{"1", "NA", "5", "9"}, dxccValues(m), "stored values matching the call are updated")
}

func TestEditDXCCKeepsStoredValues(t *testing.T) {
	q := editedQSO(t)
	q.CQZ = sql.Null[int]{V: 4, Valid: true}
	m := openEditModel(t, newEditStore(t, q), WithDXCC(newDXCCTable(t)))

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "VE3ABC"})
	assert.Equal(t, [4]string{"1", "NA", "4", "9"}, dxccValues(m),
		"a stored CQ zone that differs from the call's is kept")
}

func TestEntryDXCCShowsEntityBeforeLoTW(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{"W1AW": {last: entryDaysAgo(3)}}}
	m := newEntryModel(t, newEntryStore(t, nil), WithDXCC(newDXCCTable(t)), WithLoTW(f))
	m = focusOn(t, m, qsoentry.Call)
	m = typeCall(t, m, "W1AW")

	assert.Contains(t, entryCallLine(t, m), "United States · NA · CQ 5 · ITU 8  [L] last upload 3 days ago",
		"the entity comes first, then the LoTW status")
}
