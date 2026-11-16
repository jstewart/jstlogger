package callbook

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseCall(t *testing.T) {
	tests := []struct {
		call     string
		base     string
		portable bool
	}{
		{"W1AW", "W1AW", false},
		{"w1aw ", "W1AW", false},
		{"VE3/W1AW", "W1AW", true},
		{"W1AW/P", "W1AW", true},
		{"W1AW/7", "W1AW", true},
		{" ve3/w1aw/p ", "W1AW", true},
	}
	for _, tt := range tests {
		t.Run(tt.call, func(t *testing.T) {
			base, portable := baseCall(tt.call)
			assert.Equal(t, tt.base, base)
			assert.Equal(t, tt.portable, portable)
		})
	}
}

func TestNormalize(t *testing.T) {
	assert.Equal(t, "W1AW/P", normalize("  w1aw/p\t"))
}

func TestResultPortable(t *testing.T) {
	r := Result{
		Call: "VE3/W1AW", Name: "Hiram", QTH: "Newington", Grid: "FN31",
		State: "CT", County: "Hartford", Email: "w1aw@arrl.org",
		Service: ServiceQRZ, Limited: true,
	}
	assert.Equal(t, Result{
		Call: "VE3/W1AW", Name: "Hiram", Email: "w1aw@arrl.org",
		Service: ServiceQRZ, Limited: true,
	}, r.portable())
}

func TestNetworkError(t *testing.T) {
	inner := errors.New("connection refused")
	var err error = &NetworkError{Service: ServiceHamQTH, Err: inner}
	assert.Equal(t, "callbook: hamqth: connection refused", err.Error())
	assert.ErrorIs(t, fmt.Errorf("wrap: %w", err), inner)

	var ne *NetworkError
	assert.ErrorAs(t, fmt.Errorf("wrap: %w", err), &ne)
	assert.Equal(t, ServiceHamQTH, ne.Service)
}
