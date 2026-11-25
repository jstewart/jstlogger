package callinfo

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/callbook"
	cbmock "jstlogger/internal/callbook/mock"
	"jstlogger/internal/dxcc"
	"jstlogger/internal/logbook"
)

// stubDXCC resolves every call to entity.
type stubDXCC struct{ entity dxcc.Entity }

func (s stubDXCC) Lookup(string) (dxcc.Entity, bool) { return s.entity, true }

// usa is the DXCC entity stubDXCC answers in tests.
var usa = dxcc.Entity{DXCC: 291, Name: "United States", Continent: "NA", CQZone: 5, ITUZone: 8}

// wsjtxQSO is a QSO as WSJT-X logs it.
func wsjtxQSO() logbook.QSO {
	return logbook.QSO{
		Call: "W1AW", StartUTC: time.Date(2026, 11, 23, 13, 1, 2, 0, time.UTC),
		Band: "20m", Mode: "FT8", Gridsquare: "FN31",
	}
}

func TestFillFillsEmptyFields(t *testing.T) {
	l := Looker{
		DXCC: stubDXCC{usa},
		Callbook: looker(t, "W1AW", callbook.Result{
			Name: "Hiram Maxim", QTH: "Newington", Grid: "FN31pr", State: "CT", County: "Hartford",
			Email: "w1aw@arrl.org", Service: callbook.ServiceQRZ,
		}, nil),
	}
	in := wsjtxQSO()

	q, err := l.Fill(t.Context(), in)
	require.NoError(t, err)

	assert.Equal(t, sql.Null[int]{V: 291, Valid: true}, q.DXCC)
	assert.Equal(t, "NA", q.Cont)
	assert.Equal(t, "UNITED STATES OF AMERICA", q.Country)
	assert.Equal(t, sql.Null[int]{V: 5, Valid: true}, q.CQZ)
	assert.Equal(t, sql.Null[int]{V: 8, Valid: true}, q.ITUZ)
	assert.Equal(t, "Hiram Maxim", q.Name)
	assert.Equal(t, "Newington", q.QTH)
	assert.Equal(t, "FN31", q.Gridsquare, "the grid the station sent wins")
	assert.Equal(t, "CT", q.State)
	assert.Equal(t, "CT,Hartford", q.Cnty)
	assert.Equal(t, map[string]string{"EMAIL": "w1aw@arrl.org"}, q.Fields)
	assert.Nil(t, in.Fields, "the caller's QSO is not changed")
}

func TestFillKeepsValuesTheQSOHas(t *testing.T) {
	l := Looker{
		DXCC:     stubDXCC{usa},
		Callbook: looker(t, "W1AW", callbook.Result{Name: "Hiram Maxim", Email: "new@arrl.org", Service: callbook.ServiceQRZ}, nil),
	}
	in := wsjtxQSO()
	in.Name, in.Cont = "Hi", "EU"
	in.CQZ = sql.Null[int]{V: 14, Valid: true}
	in.Fields = map[string]string{"EMAIL": "old@arrl.org", "COMMENT_INTL": "x"}

	q, err := l.Fill(t.Context(), in)
	require.NoError(t, err)

	assert.Equal(t, "Hi", q.Name)
	assert.Equal(t, "EU", q.Cont)
	assert.Equal(t, 14, q.CQZ.V)
	assert.Equal(t, 291, q.DXCC.V, "an empty field is still filled")
	assert.Equal(t, "old@arrl.org", q.Fields["EMAIL"])
}

func TestFillCopiesFieldsBeforeAddingEmail(t *testing.T) {
	l := Looker{Callbook: looker(t, "W1AW", callbook.Result{Email: "w1aw@arrl.org", Service: callbook.ServiceQRZ}, nil)}
	in := wsjtxQSO()
	in.Fields = map[string]string{"PROP_MODE": "F2"}

	q, err := l.Fill(t.Context(), in)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"PROP_MODE": "F2", "EMAIL": "w1aw@arrl.org"}, q.Fields)
	assert.Equal(t, map[string]string{"PROP_MODE": "F2"}, in.Fields, "the caller's map is not changed")
}

func TestFillCallbookFailure(t *testing.T) {
	l := Looker{
		DXCC:     stubDXCC{usa},
		Callbook: looker(t, "W1AW", callbook.Result{}, fmt.Errorf("qrz: %w", callbook.ErrRateLimit)),
		Logbook:  newStore(t, earlierQSOs...),
	}

	q, err := l.Fill(t.Context(), wsjtxQSO())

	require.ErrorIs(t, err, callbook.ErrRateLimit)
	assert.Contains(t, err.Error(), "W1AW")
	assert.Equal(t, 291, q.DXCC.V, "the DXCC entity is still filled in")
	assert.Equal(t, "Hiram P", q.Name, "the name comes from the logbook")
}

func TestFillCallWithoutBase(t *testing.T) {
	// The callbook expects no lookup, so asking it fails the test.
	l := Looker{DXCC: stubDXCC{usa}, Callbook: cbmock.NewMockLooker(gomock.NewController(t))}

	in := wsjtxQSO()
	in.Call = " / "
	q, err := l.Fill(t.Context(), in)
	require.NoError(t, err)
	assert.Equal(t, in, q)
}

func TestFillNothingConfigured(t *testing.T) {
	in := wsjtxQSO()
	q, err := Looker{}.Fill(t.Context(), in)
	require.NoError(t, err)
	assert.Equal(t, in, q)
}

func TestFillNoDXCCEntity(t *testing.T) {
	in := wsjtxQSO()
	in.Call = "W1AW/MM"
	q, err := Looker{DXCC: dxcc.Default()}.Fill(t.Context(), in)
	require.NoError(t, err)
	assert.False(t, q.DXCC.Valid, "maritime mobile is in no entity")
	assert.Empty(t, q.Cont)
	assert.Empty(t, q.Country)
}

func TestFillWithCountryFile(t *testing.T) {
	q, err := Looker{DXCC: dxcc.Default()}.Fill(t.Context(), wsjtxQSO())
	require.NoError(t, err)
	assert.Equal(t, 291, q.DXCC.V)
	assert.Equal(t, "NA", q.Cont)
	assert.Equal(t, "UNITED STATES OF AMERICA", q.Country)
}

func TestFillCountry(t *testing.T) {
	code := func(n int) sql.Null[int] { return sql.Null[int]{V: n, Valid: true} }
	tests := []struct {
		name    string
		looker  Looker
		dxcc    sql.Null[int]
		country string
		want    string
	}{
		{"from the entity found", Looker{DXCC: stubDXCC{usa}}, sql.Null[int]{}, "", "UNITED STATES OF AMERICA"},
		{"an existing country is kept", Looker{DXCC: stubDXCC{usa}}, sql.Null[int]{}, "USA", "USA"},
		{"a blank country is filled", Looker{DXCC: stubDXCC{usa}}, sql.Null[int]{}, "  ", "UNITED STATES OF AMERICA"},
		{"a code WSJT-X sent", Looker{}, code(1), "", "CANADA"},
		{"the code the QSO has wins", Looker{DXCC: stubDXCC{usa}}, code(1), "", "CANADA"},
		{"no entity", Looker{}, code(0), "", ""},
		{"an unknown code", Looker{}, code(999), "", ""},
		{"no code", Looker{}, sql.Null[int]{}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := wsjtxQSO()
			in.DXCC, in.Country = tt.dxcc, tt.country
			q, err := tt.looker.Fill(t.Context(), in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, q.Country)
		})
	}
}
