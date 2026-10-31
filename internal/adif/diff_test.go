package adif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiff(t *testing.T) {
	k1 := Record{{"CALL", "K1ABC"}, {"BAND", "20m"}}
	w1 := Record{{"CALL", "W1AW"}, {"BAND", "40m"}}
	tests := []struct {
		name   string
		a, b   []Record
		ignore []string
		want   []string
	}{
		{
			name: "both empty",
		},
		{
			name: "equal with record and field reordering",
			a:    []Record{k1, w1},
			b:    []Record{{{"band", "40m"}, {"call", "W1AW"}}, {{"BAND", "20m"}, {"CALL", "K1ABC"}}},
		},
		{
			name:   "ignore list",
			a:      []Record{{{"CALL", "K1ABC"}, {"APP_X", "1"}}},
			b:      []Record{{{"CALL", "K1ABC"}, {"app_x", "2"}, {"PROGRAMID", "z"}}},
			ignore: []string{"App_X", "programid"},
		},
		{
			name: "empty value treated as absent",
			a:    []Record{{{"CALL", "K1ABC"}, {"NAME", ""}}},
			b:    []Record{{{"CALL", "K1ABC"}}},
		},
		{
			name: "differing value",
			a:    []Record{{{"CALL", "K1ABC"}, {"BAND", "20m"}}},
			b:    []Record{{{"CALL", "K1ABC"}, {"BAND", "20M"}}},
			want: []string{
				`only in first: BAND="20m" CALL="K1ABC"`,
				`only in second: BAND="20M" CALL="K1ABC"`,
			},
		},
		{
			name: "extra record",
			a:    []Record{k1},
			b:    []Record{k1, w1},
			want: []string{`only in second: BAND="40m" CALL="W1AW"`},
		},
		{
			name: "duplicate records counted",
			a:    []Record{k1, k1, w1},
			b:    []Record{w1, k1},
			want: []string{`only in first: BAND="20m" CALL="K1ABC"`},
		},
		{
			name: "spaces in values cannot collide",
			a:    []Record{{{"A", "1 B=2"}}},
			b:    []Record{{{"A", "1"}, {"B", "2"}}},
			want: []string{
				`only in first: A="1 B=2"`,
				`only in second: A="1" B="2"`,
			},
		},
		{
			name: "duplicates reported with count",
			a:    []Record{w1, w1, w1},
			b:    nil,
			want: []string{`only in first (x3): BAND="40m" CALL="W1AW"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Diff(tt.a, tt.b, tt.ignore...)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDiffDeterministic(t *testing.T) {
	a := []Record{{{"CALL", "Z1"}}, {{"CALL", "A1"}}, {{"CALL", "M1"}}}
	b := []Record{{{"CALL", "Y2"}}, {{"CALL", "B2"}}}
	want := []string{
		`only in first: CALL="A1"`,
		`only in first: CALL="M1"`,
		`only in first: CALL="Z1"`,
		`only in second: CALL="B2"`,
		`only in second: CALL="Y2"`,
	}
	for range 5 {
		assert.Equal(t, want, Diff(a, b))
	}
}
