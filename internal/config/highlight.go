package config

import (
	"fmt"
	"image/color"
	"strconv"
)

// ParseColor parses a "#rrggbb" color, ignoring case. An empty s is no
// color: nil and no error.
func ParseColor(s string) (color.Color, error) {
	if s == "" {
		return nil, nil
	}
	if len(s) != 7 || s[0] != '#' {
		return nil, fmt.Errorf("want a color such as %q, got %q", "#fff59d", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("want a color such as %q, got %q", "#fff59d", s)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, nil
}
