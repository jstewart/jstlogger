package config

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseColor(t *testing.T) {
	c, err := ParseColor("#12aBcD")
	require.NoError(t, err)
	assert.Equal(t, color.RGBA{R: 0x12, G: 0xab, B: 0xcd, A: 0xff}, c)

	c, err = ParseColor("")
	require.NoError(t, err)
	assert.Nil(t, c, "an empty color is no color")

	for _, bad := range []string{"12abcd", "#12abc", "#12abcde", "#12abcg", "red", " #12abcd"} {
		_, err := ParseColor(bad)
		assert.Error(t, err, bad)
	}
}
