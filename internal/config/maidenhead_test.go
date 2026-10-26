package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidGridsquare(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"4 chars", "CM87", true},
		{"6 chars", "CM87wj", true},
		{"8 chars", "CM87WJ12", true},
		{"lowercase", "cm87wj", true},
		{"corner low", "AA00aa00", true},
		{"corner high", "RR99XX99", true},
		{"empty", "", false},
		{"2 chars", "CM", false},
		{"odd length", "CM87W", false},
		{"10 chars", "CM87WJ12AB", false},
		{"field out of range", "SM87", false},
		{"field digit", "1M87", false},
		{"square letter", "CMA7", false},
		{"subsquare out of range", "CM87YJ", false},
		{"subsquare digit", "CM8712", false},
		{"extended letter", "CM87WJA1", false},
		{"non-ascii", "CМ87", false},
		{"space", "CM 7", false},
		{"non-ascii that grows when uppercased", "ɐɐɐɐ", false},
		{"non-ascii that shrinks when uppercased", "ıM8", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ValidGridsquare(tt.in), "ValidGridsquare(%q)", tt.in)
		})
	}
}
