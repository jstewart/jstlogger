package qsoentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseNumbers(t *testing.T) {
	tests := []struct {
		name   string
		parse  func(string) (int, bool)
		in     string
		want   int
		wantOK bool
	}{
		{"DXCC none", parseDXCC, "0", 0, true},
		{"DXCC", parseDXCC, "291", 291, true},
		{"DXCC max", parseDXCC, "999", 999, true},
		{"DXCC too large", parseDXCC, "1000", 0, false},
		{"DXCC negative", parseDXCC, "-1", 0, false},
		{"DXCC not a number", parseDXCC, "USA", 0, false},
		{"DXCC signed", parseDXCC, "+291", 0, false},
		{"CQ zone", parseCQZone, "5", 5, true},
		{"CQ zone max", parseCQZone, "40", 40, true},
		{"CQ zone zero", parseCQZone, "0", 0, false},
		{"CQ zone too large", parseCQZone, "41", 0, false},
		{"ITU zone", parseITUZone, "8", 8, true},
		{"ITU zone max", parseITUZone, "90", 90, true},
		{"ITU zone zero", parseITUZone, "0", 0, false},
		{"ITU zone too large", parseITUZone, "91", 0, false},
		{"ITU zone decimal", parseITUZone, "8.5", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.parse(tt.in)
			assert.Equal(t, tt.wantOK, ok, "is %q valid", tt.in)
			if tt.wantOK {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParsePower(t *testing.T) {
	tests := []struct {
		in     string
		want   float64
		wantOK bool
	}{
		{"100", 100, true},
		{"5.5", 5.5, true},
		{"0.5", 0.5, true},
		{".5", 0.5, true},
		{"1500.", 1500, true},
		{"0", 0, false},
		{"0.0", 0, false},
		{"-5", 0, false},
		{"5W", 0, false},
		{"1e3", 0, false},
		{"NaN", 0, false},
		{"0x10", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := parsePower(tt.in)
			assert.Equal(t, tt.wantOK, ok, "is %q a power in watts", tt.in)
			if tt.wantOK {
				assert.InDelta(t, tt.want, got, 1e-9)
			}
		})
	}
}

func TestReferenceValidators(t *testing.T) {
	tests := []struct {
		name  string
		valid func(string) bool
		good  []string
		bad   []string
	}{
		{
			name: "continent", valid: isContinent,
			good: []string{"AF", "AN", "AS", "EU", "NA", "OC", "SA", "na"},
			bad:  []string{"", "AM", "N", "NAX", "EU "},
		},
		{
			name: "IOTA", valid: isIOTA,
			good: []string{"NA-001", "EU-005", "oc-123"},
			bad:  []string{"", "NA-1", "NA-0001", "XX-001", "NA001", "NA-00A"},
		},
		{
			name: "SOTA", valid: isSOTA,
			good: []string{"W1/HA-001", "G/LD-001", "VK3/VC-012", "w7w/lc-123"},
			bad:  []string{"", "W1HA-001", "W1/H-001", "W1/HA-01", "W1/HA-0001", "ABCDE/HA-001"},
		},
		{
			name: "WWFF", valid: isWWFF,
			good: []string{"KFF-1234", "GFF-0001", "ONFF-0123", "vkff-0001"},
			bad:  []string{"", "KFF-123", "KFF-12345", "K-1234", "KFF1234"},
		},
		{
			name: "POTA", valid: isPOTA,
			good: []string{"K-1234", "US-12345", "VE-0001", "K-1234@US-MA", "k-1234@us-ma"},
			bad:  []string{"", "K-123", "K-123456", "K1234", "K-1234@", "ABCDE-1234", "K-1234,K-5678"},
		},
		{
			name: "email", valid: isEmail,
			good: []string{"ann@example.com", "a@b", "first.last+tag@mail.example.org"},
			bad:  []string{"", "ann", "@example.com", "ann@", "ann@@example.com", "a@b@c", "ann @example.com", "ann@exa mple.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, s := range tt.good {
				assert.True(t, tt.valid(s), "%q is a valid %s", s, tt.name)
			}
			for _, s := range tt.bad {
				assert.False(t, tt.valid(s), "%q is not a valid %s", s, tt.name)
			}
		})
	}
}

func TestNormalizePOTA(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantBad string
	}{
		{in: "", want: ""},
		{in: " k-1234 ", want: "K-1234"},
		{in: "k-1234, K-5678 ,,us-0001@us-ma", want: "K-1234,K-5678,US-0001@US-MA"},
		{in: "K-1234, nope", wantBad: "NOPE"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, bad := normalizePOTA(tt.in)
			assert.Equal(t, tt.wantBad, bad, "the first invalid reference is reported")
			if tt.wantBad == "" {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
