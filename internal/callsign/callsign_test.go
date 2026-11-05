package callsign

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBase(t *testing.T) {
	tests := []struct {
		name string
		call string
		want string
	}{
		{name: "plain", call: "W1AW", want: "W1AW"},
		{name: "lowercase", call: "w1aw", want: "W1AW"},
		{name: "surrounding spaces", call: " W1AW ", want: "W1AW"},
		{name: "prefix", call: "VE3/W1AW", want: "W1AW"},
		{name: "portable", call: "W1AW/P", want: "W1AW"},
		{name: "mobile", call: "W1AW/M", want: "W1AW"},
		{name: "qrp", call: "W1AW/QRP", want: "W1AW"},
		{name: "maritime mobile", call: "W1AW/MM", want: "W1AW"},
		{name: "aeronautical mobile", call: "w1aw/am", want: "W1AW"},
		{name: "prefix and suffix", call: "VE3/W1AW/P", want: "W1AW"},
		{name: "suffix country", call: "W1AW/VE3", want: "W1AW"},
		{name: "longer prefix", call: "KH6/W1AW", want: "W1AW"},
		{name: "call area digit", call: "W1AW/7", want: "W1AW"},
		{name: "tie picks last", call: "K1A/W1B", want: "W1B"},
		{name: "empty parts", call: "W1AW//P", want: "W1AW"},
		{name: "empty", call: "", want: ""},
		{name: "spaces only", call: "   ", want: ""},
		{name: "slash only", call: "/", want: ""},
		{name: "modifier only", call: "P", want: ""},
		{name: "modifiers only", call: "P/QRP/7", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Base(tt.call))
		})
	}
}
