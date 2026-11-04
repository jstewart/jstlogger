package indexfmt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrftime(t *testing.T) {
	ts := time.Date(2024, time.March, 5, 7, 8, 9, 0, time.UTC)
	tests := []struct {
		name   string
		layout string
		want   string
	}{
		{"date", "%Y-%m-%d", "2024-03-05"},
		{"short year", "%y", "24"},
		{"space padded day", "%e", " 5"},
		{"day of year", "%j", "065"},
		{"time", "%H:%M:%S", "07:08:09"},
		{"month names", "%b %B", "Mar March"},
		{"weekday names", "%a %A", "Tue Tuesday"},
		{"percent", "100%%", "100%"},
		{"literal only", "at", "at"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, checkStrftime(tt.layout, 0))
			assert.Equal(t, tt.want, strftime(ts, tt.layout))
		})
	}
}

func TestCheckStrftimeErrors(t *testing.T) {
	tests := []struct {
		name    string
		layout  string
		wantMsg string
	}{
		{"unknown conversion", "%Y-%Q", `unknown strftime conversion "%Q" at column 14`},
		{"lone percent", "%Y%", "lone % at column 13"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkStrftime(tt.layout, 10)
			require.ErrorIs(t, err, ErrSyntax)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestTimeOnly(t *testing.T) {
	tests := []struct {
		layout string
		want   bool
	}{
		{"%H:%M", true},
		{"%H:%M:%S %%", true},
		{"%Y-%m-%d", false},
		{"%d %H", false},
		{"text", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.layout, func(t *testing.T) {
			assert.Equal(t, tt.want, timeOnly(tt.layout))
		})
	}
}
