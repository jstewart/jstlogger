package adif

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModes(t *testing.T) {
	modes := Modes()

	for _, m := range []string{"AM", "CW", "FM", "FT8", "MFSK", "PSK", "RTTY", "SSB"} {
		assert.Contains(t, modes, m)
	}
	assert.True(t, slices.IsSorted(modes), "modes are in ADIF's alphabetical order")
	assert.NotContains(t, modes, "FT4", "FT4 is a submode of MFSK")

	modes[0] = "changed"
	assert.NotEqual(t, "changed", Modes()[0], "the caller gets a copy")
}

func TestSubmodes(t *testing.T) {
	tests := []struct {
		mode    string
		want    []string
		without []string
	}{
		{mode: "MFSK", want: []string{"FT4", "JS8", "Q65"}, without: []string{"FT8"}},
		{mode: "mfsk", want: []string{"FT4"}},
		{mode: "SSB", want: []string{"LSB", "USB"}},
		{mode: "CW", want: []string{"PCW"}},
		{mode: "FT8"},
		{mode: "NOSUCH"},
		{mode: ""},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got := Submodes(tt.mode)
			for _, s := range tt.want {
				assert.Contains(t, got, s)
			}
			for _, s := range tt.without {
				assert.NotContains(t, got, s)
			}
			if len(tt.want) == 0 {
				assert.Empty(t, got)
			}
		})
	}

	Submodes("SSB")[0] = "changed"
	assert.Equal(t, "LSB", Submodes("SSB")[0], "the caller gets a copy")
}

func TestModesHaveUniqueSubmodes(t *testing.T) {
	seen := map[string]string{}
	for _, m := range Modes() {
		for _, s := range Submodes(m) {
			if other, ok := seen[s]; ok {
				t.Errorf("submode %s is under both %s and %s", s, other, m)
			}
			seen[s] = m
		}
	}
}
