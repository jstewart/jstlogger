package pattern

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

func TestParse(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	band := func(b string) logbook.Expr { return logbook.Filter{Bands: []string{b}} }

	tests := []struct {
		name string
		in   string
		want logbook.Expr
	}{
		{"all", "~A", logbook.All{}},
		{"all surrounded by space", "  ~A\t", logbook.All{}},
		{"call", "~c W1AW", logbook.CallContains{Call: "W1AW"}},
		{"band without space", "~b20m", band("20m")},
		{"band", "~b 20m", band("20m")},
		{"mode", "~m FT8", logbook.ModeIs{Mode: "FT8"}},
		{"tag", "~y pota", logbook.Filter{Tags: []string{"pota"}}},
		{"country", "~C Japan", logbook.Filter{Countries: []string{"Japan"}}},
		{"quoted country", `~C "United States"`, logbook.Filter{Countries: []string{"United States"}}},
		{"call and country are distinct", "~c W1 ~C Canada", logbook.And{
			logbook.CallContains{Call: "W1"}, logbook.Filter{Countries: []string{"Canada"}},
		}},
		{"state", "~s MA", logbook.Filter{States: []string{"MA"}}},
		{"continent", "~o EU", logbook.Filter{Continents: []string{"EU"}}},
		{"continent ignores case", "~o oc", logbook.Filter{Continents: []string{"oc"}}},
		{"double-quoted tag", `~y "field day"`, logbook.Filter{Tags: []string{"field day"}}},
		{"single-quoted tag", `~y 'field day'`, logbook.Filter{Tags: []string{"field day"}}},
		{"escaped quote", `~y "say \"hi\""`, logbook.Filter{Tags: []string{`say "hi"`}}},
		{"escaped backslash", `~y 'a\\b'`, logbook.Filter{Tags: []string{`a\b`}}},
		{"quote adjacent to next term", `~y "a b"~m CW`, logbook.And{
			logbook.Filter{Tags: []string{"a b"}}, logbook.ModeIs{Mode: "CW"},
		}},
		{"date", "~d 2024-03-05", logbook.Filter{From: day(2024, 3, 5), To: day(2024, 3, 6)}},
		{"implicit and", "~c W1 ~b 20m ~m CW", logbook.And{
			logbook.CallContains{Call: "W1"}, band("20m"), logbook.ModeIs{Mode: "CW"},
		}},
		{"or", "~b 20m | ~b 40m|~b 80m", logbook.Or{band("20m"), band("40m"), band("80m")}},
		{"and binds tighter than or", "~c W1 ~b 20m | ~m CW", logbook.Or{
			logbook.And{logbook.CallContains{Call: "W1"}, band("20m")},
			logbook.ModeIs{Mode: "CW"},
		}},
		{"not", "!~m CW", logbook.Not{X: logbook.ModeIs{Mode: "CW"}}},
		{"not with space", "! ~m CW", logbook.Not{X: logbook.ModeIs{Mode: "CW"}}},
		{"double not", "!!~A", logbook.Not{X: logbook.Not{X: logbook.All{}}}},
		{"not binds tighter than and", "!~m CW ~b 20m", logbook.And{
			logbook.Not{X: logbook.ModeIs{Mode: "CW"}}, band("20m"),
		}},
		{"parens are transparent", "(~b 20m)", band("20m")},
		{"nested parens", "((~b 20m))", band("20m")},
		{"grouped or", "~c W1 (~b 20m | ~b 40m)", logbook.And{
			logbook.CallContains{Call: "W1"}, logbook.Or{band("20m"), band("40m")},
		}},
		{"value ends at paren", "(~c W1)", logbook.CallContains{Call: "W1"}},
		{"value ends at bar", "~c W1|~c K1", logbook.Or{
			logbook.CallContains{Call: "W1"}, logbook.CallContains{Call: "K1"},
		}},
		{"not group", "!(~b 20m | ~b 40m)", logbook.Not{X: logbook.Or{band("20m"), band("40m")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string // substrings of the error message
	}{
		{"empty", "", []string{"empty pattern"}},
		{"whitespace only", "  \t", []string{"empty pattern"}},
		{"unknown term", "  ~q x", []string{"~q", "column 3"}},
		{"lowercase all is unknown", "~a", []string{"~a", "column 1"}},
		{"tilde at end", "~A ~", []string{"~", "column 4"}},
		{"missing value at end", "~c", []string{"~c needs a value", "column 1"}},
		{"missing value before paren", "(~b)", []string{"~b needs a value", "column 2"}},
		{"empty quoted value", `~c ""`, []string{"~c needs a value", "column 1"}},
		{"unterminated double quote", `~y "abc`, []string{"unterminated", "column 4"}},
		{"unterminated single quote", `~y 'abc\'`, []string{"unterminated", "column 4"}},
		{"unclosed paren", "~A (~b 20m", []string{"(", "column 4"}},
		{"extra close paren", "~A )", []string{")", "column 4"}},
		{"leading close paren", ")", []string{")", "column 1"}},
		{"dangling bar at end", "~A |", []string{"|", "column 4"}},
		{"dangling bar at start", "| ~A", []string{"|", "column 1"}},
		{"double bar", "~A || ~A", []string{"|", "column 4"}},
		{"bar before close paren", "(~A |)", []string{"|", "column 5"}},
		{"dangling not", "~A !", []string{"!", "column 4"}},
		{"not before bar", "! | ~A", []string{"!", "column 1"}},
		{"empty parens", "~A ( )", []string{"()", "column 4"}},
		{"bare word", "W1AW", []string{"expected ~ term", "column 1"}},
		{"value for all", "~A foo", []string{"expected ~ term", "column 4"}},
		{"value swallows a term", "~c ~b 20m", []string{"expected ~ term", "column 7"}},
		{"unknown continent", "~b 20m ~o EUR", []string{"~o", "column 8", "EUR", "AF, AN, AS, EU, NA, OC, SA"}},
		{"missing state", "~s", []string{"~s needs a value", "column 1"}},
		{"bad date", "~d 2024-13-01", []string{"~d", "column 1", "2024-13-01"}},
		{"reversed range", "~b 20m ~d 2024-03-05-2024-03-01", []string{"~d", "column 8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			require.ErrorIs(t, err, ErrSyntax)
			assert.Nil(t, got)
			for _, w := range tt.want {
				assert.Contains(t, err.Error(), w)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"~A", "~c W1 (~b 20m | ~b 40m)", `!~y "a \" b"`, "~d 2024-01-01-", "((", "~", `'`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		e, err := Parse(s)
		if err == nil && e == nil {
			t.Fatalf("Parse(%q) returned nil Expr and nil error", s)
		}
		if err != nil && e != nil {
			t.Fatalf("Parse(%q) returned both Expr and error", s)
		}
	})
}
