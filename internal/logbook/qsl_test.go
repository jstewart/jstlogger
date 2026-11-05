package logbook

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQSLSentText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"yes", "Y", "Yes"},
		{"no", "N", "No"},
		{"requested", "R", "Requested"},
		{"queued", "Q", "Queued"},
		{"ignored", "I", "Ignored"},
		{"lower case", "q", "Queued"},
		{"surrounding space", " y ", "Yes"},
		{"rcvd-only letter is unknown", "V", "V"},
		{"unknown", "X", "X"},
		{"unknown trimmed", " x ", "x"},
		{"empty", "", ""},
		{"blank", "  ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, QSLSentText(tt.in))
		})
	}
}

func TestQSLRcvdText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"yes", "Y", "Yes"},
		{"no", "N", "No"},
		{"requested", "R", "Requested"},
		{"ignored", "I", "Ignored"},
		{"verified", "V", "Verified"},
		{"lower case", "v", "Verified"},
		{"surrounding space", " n ", "No"},
		{"sent-only letter is unknown", "Q", "Q"},
		{"unknown", "X", "X"},
		{"unknown trimmed", " x ", "x"},
		{"empty", "", ""},
		{"blank", "  ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, QSLRcvdText(tt.in))
		})
	}
}
