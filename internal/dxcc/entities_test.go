package dxcc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEntityName(t *testing.T) {
	tests := []struct {
		name   string
		code   int
		want   string
		wantOK bool
	}{
		{"current", 291, "UNITED STATES OF AMERICA", true},
		{"first", 1, "CANADA", true},
		{"last", 522, "REPUBLIC OF KOSOVO", true},
		{"name with comma", 255, "ST. MAARTEN, SABA, ST. EUSTATIUS", true},
		{"deleted", 2, "ABU AIL IS.", true},
		{"no entity", 0, "", false},
		{"unknown", 999, "", false},
		{"negative", -1, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := EntityName(tt.code)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEntityNamesComplete(t *testing.T) {
	for code, name := range entityNames {
		assert.NotEmpty(t, name, "code %d", code)
		assert.NotZero(t, code)
	}
	assert.Len(t, entityNames, 402)
}
