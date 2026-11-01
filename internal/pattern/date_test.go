package pattern

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

func TestParseDateRange(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	tests := []struct {
		name string
		in   string
		want logbook.Filter
	}{
		{"single day", "2024-03-05", logbook.Filter{From: day(2024, 3, 5), To: day(2024, 3, 6)}},
		{"end of month", "2024-02-29", logbook.Filter{From: day(2024, 2, 29), To: day(2024, 3, 1)}},
		{"end of year", "2023-12-31", logbook.Filter{From: day(2023, 12, 31), To: day(2024, 1, 1)}},
		{"open end", "2024-03-05-", logbook.Filter{From: day(2024, 3, 5)}},
		{"open start", "-2024-03-05", logbook.Filter{To: day(2024, 3, 6)}},
		{"range", "2024-03-01-2024-03-05", logbook.Filter{From: day(2024, 3, 1), To: day(2024, 3, 6)}},
		{"one-day range", "2024-03-05-2024-03-05", logbook.Filter{From: day(2024, 3, 5), To: day(2024, 3, 6)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDateRange(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseDateRangeErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bad month", "2024-13-01", "2024-13-01"},
		{"bad day", "2023-02-29", "2023-02-29"},
		{"not a date", "yesterday", "yesterday"},
		{"short", "2024-3-5", "2024-3-5"},
		{"lone dash", "-", `"-"`},
		{"bad end", "2024-03-01-2024-03-xx", "2024-03-xx"},
		{"bad separator", "2024-03-01+2024-03-05", "2024-03-01+2024-03-05"},
		{"reversed", "2024-03-05-2024-03-01", "before"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseDateRange(tt.in)
			require.ErrorIs(t, err, ErrSyntax)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
