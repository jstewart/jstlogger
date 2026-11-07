package callsign

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQRZURL(t *testing.T) {
	tests := []struct {
		name string
		call string
		want string
	}{
		{name: "plain", call: "W1AW", want: "https://www.qrz.com/db/W1AW"},
		{name: "portable", call: "VE3/W1AW/P", want: "https://www.qrz.com/db/W1AW"},
		{name: "lowercase", call: "w1aw", want: "https://www.qrz.com/db/W1AW"},
		{name: "escaped hash", call: "W1#AW", want: "https://www.qrz.com/db/W1%23AW"},
		{name: "escaped space and question mark", call: "W1 AW?", want: "https://www.qrz.com/db/W1%20AW%3F"},
		{name: "empty", call: "", want: ""},
		{name: "modifier only", call: "/P", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, QRZURL(tt.call))
		})
	}
}

func TestHamQTHURL(t *testing.T) {
	tests := []struct {
		name string
		call string
		want string
	}{
		{name: "plain", call: "W1AW", want: "https://www.hamqth.com/W1AW"},
		{name: "portable", call: "VE3/W1AW/P", want: "https://www.hamqth.com/W1AW"},
		{name: "lowercase", call: "w1aw", want: "https://www.hamqth.com/W1AW"},
		{name: "escaped hash", call: "W1#AW", want: "https://www.hamqth.com/W1%23AW"},
		{name: "escaped space and question mark", call: "W1 AW?", want: "https://www.hamqth.com/W1%20AW%3F"},
		{name: "empty", call: "", want: ""},
		{name: "modifier only", call: "/P", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HamQTHURL(tt.call))
		})
	}
}

func TestEQSLURL(t *testing.T) {
	tests := []struct {
		name string
		call string
		want string
	}{
		{name: "plain", call: "W1AW", want: "https://www.eqsl.cc/Member.cfm?W1AW"},
		{name: "portable", call: "VE3/W1AW/P", want: "https://www.eqsl.cc/Member.cfm?W1AW"},
		{name: "lowercase", call: "w1aw", want: "https://www.eqsl.cc/Member.cfm?W1AW"},
		{name: "escaped hash", call: "W1#AW", want: "https://www.eqsl.cc/Member.cfm?W1%23AW"},
		{name: "escaped space and ampersand", call: "W1 A&W", want: "https://www.eqsl.cc/Member.cfm?W1+A%26W"},
		{name: "empty", call: "", want: ""},
		{name: "modifier only", call: "/P", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, EQSLURL(tt.call))
		})
	}
}
