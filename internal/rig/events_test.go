package rig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConnStatusString(t *testing.T) {
	tests := []struct {
		status ConnStatus
		want   string
	}{
		{Connecting, "connecting"},
		{Connected, "connected"},
		{Disconnected, "disconnected"},
		{ConnStatus(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.String())
		})
	}
}
