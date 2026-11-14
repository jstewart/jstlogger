package logbook

import (
	"database/sql/driver"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLCallBase(t *testing.T) {
	tests := []struct {
		name string
		arg  driver.Value
		want driver.Value
	}{
		{"text", "ve3/w1aw/p", "W1AW"},
		{"blob", []byte("W1AW/7"), "W1AW"},
		{"null", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sqlCallBase(nil, []driver.Value{tt.arg})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := sqlCallBase(nil, []driver.Value{int64(1)})
	assert.Error(t, err)
}
