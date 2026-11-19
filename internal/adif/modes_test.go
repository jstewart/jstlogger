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

func TestDescribeMode(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"MFSK", "multiple frequency-shift keying: FT4, JS8, Q65…"},
		{"DIGITALVOICE", "digital voice: C4FM, DMR, D-STAR…"},
		{"C4FM", "Yaesu System Fusion"},
		{"c4fm", "Yaesu System Fusion"},
		{"SSB", ""},
		{"NOSUCH", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DescribeMode(tt.name))
		})
	}
}

func TestModeOf(t *testing.T) {
	tests := []struct{ name, want string }{
		{"FT4", "MFSK"},
		{"ft4", "MFSK"},
		{"PSK31", "PSK"},
		{"USB", "SSB"},
		{"SSB", "SSB"},
		{"FT8", "FT8"},
		{"NOSUCH", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ModeOf(tt.name))
		})
	}
}

func TestCommonModes(t *testing.T) {
	common := CommonModes()
	assert.Equal(t, []string{"SSB", "CW", "FT8", "FT4", "FM", "AM", "RTTY", "PSK31", "JS8"}, Codes(common))
	for _, m := range common {
		assert.NotEmpty(t, ModeOf(m.Code), "%s is a mode or submode", m.Code)
	}

	common[0].Code = "changed"
	assert.Equal(t, "SSB", CommonModes()[0].Code, "the caller gets a copy")
}

func TestModeDescriptionsAreModes(t *testing.T) {
	for name := range modeDescriptions {
		assert.NotEmpty(t, ModeOf(name), "%s is a mode or submode", name)
	}
}
