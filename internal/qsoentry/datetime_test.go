package qsoentry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// day returns midnight UTC on the given date.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestParseDate(t *testing.T) {
	mid := time.Date(2026, 11, 12, 18, 30, 0, 0, time.UTC)
	newYear := time.Date(2027, 1, 1, 0, 5, 0, 0, time.UTC)
	marchFirst := time.Date(2028, 3, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		s    string
		now  time.Time
		want time.Time
	}{
		{"full", "2026-11-03", mid, day(2026, 11, 3)},
		{"compact", "20261103", mid, day(2026, 11, 3)},
		{"spaces", "  2026-11-03 ", mid, day(2026, 11, 3)},
		{"future full date stays", "2026-12-25", mid, day(2026, 12, 25)},
		{"today", "t", mid, day(2026, 11, 12)},
		{"today upper", "T", mid, day(2026, 11, 12)},
		{"yesterday", "y", mid, day(2026, 11, 11)},
		{"yesterday over a year end", "y", newYear, day(2026, 12, 31)},
		{"yesterday over a leap day", "y", marchFirst, day(2028, 2, 29)},
		{"days ago", "-3", mid, day(2026, 11, 9)},
		{"zero days ago", "-0", mid, day(2026, 11, 12)},
		{"days ago over a month end", "-12", mid, day(2026, 10, 31)},
		{"days ago over a year end", "-2", newYear, day(2026, 12, 30)},
		{"month and day", "1103", mid, day(2026, 11, 3)},
		{"month and day dashed", "11-03", mid, day(2026, 11, 3)},
		{"month and day today", "1112", mid, day(2026, 11, 12)},
		{"later month and day is last year", "1225", mid, day(2025, 12, 25)},
		{"tomorrow is last year", "11-13", mid, day(2025, 11, 13)},
		{"month and day over a year end", "1231", newYear, day(2026, 12, 31)},
		{"month and day on new year's day", "0101", newYear, day(2027, 1, 1)},
		{"leap day this year", "0229", marchFirst, day(2028, 2, 29)},
		{"leap day after a leap year", "0229", time.Date(2029, 3, 1, 0, 0, 0, 0, time.UTC), day(2028, 2, 29)},
		{"leap day early in the year after", "0229", time.Date(2029, 1, 5, 0, 0, 0, 0, time.UTC), day(2028, 2, 29)},
		{"no leap day this year or last", "0229", time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC), time.Time{}},
		{"surrounding spaces", " t ", mid, day(2026, 11, 12)},
		{"local now is read in UTC", "t", time.Date(2026, 11, 12, 20, 0, 0, 0, time.FixedZone("EST", -5*3600)), day(2026, 11, 13)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDate(tt.s, tt.now)
			if tt.want.IsZero() {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseDateInvalid(t *testing.T) {
	now := time.Date(2026, 11, 12, 18, 30, 0, 0, time.UTC)
	for _, s := range []string{
		"", "x", "tt", "yy", "-", "--3", "-x", "+3", "1332", "0230", "11-31", "1-3",
		"113", "11033", "2026-13-01", "2026-11-31", "26-11-03", "today",
	} {
		t.Run(s, func(t *testing.T) {
			_, err := ParseDate(s, now)
			assert.Error(t, err)
		})
	}
}

func TestParseTime(t *testing.T) {
	now := time.Date(2026, 11, 12, 18, 30, 27, 500, time.UTC)
	hm := func(h, m, s int) time.Duration {
		return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
	}
	tests := []struct {
		s    string
		want time.Duration
	}{
		{"14:30", hm(14, 30, 0)},
		{"14:30:15", hm(14, 30, 15)},
		{"1430", hm(14, 30, 0)},
		{"143015", hm(14, 30, 15)},
		{" 1430 ", hm(14, 30, 0)},
		{"n", hm(18, 30, 27)},
		{"N", hm(18, 30, 27)},
		{"14", hm(14, 0, 0)},
		{"9", hm(9, 0, 0)},
		{"09", hm(9, 0, 0)},
		{"0", 0},
		{"00", 0},
		{"23", hm(23, 0, 0)},
		{"0000", 0},
		{"2359", hm(23, 59, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got, err := ParseTime(tt.s, now)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("now is read in UTC", func(t *testing.T) {
		got, err := ParseTime("n", time.Date(2026, 11, 12, 20, 15, 0, 0, time.FixedZone("EST", -5*3600)))
		require.NoError(t, err)
		assert.Equal(t, hm(1, 15, 0), got)
	})

	for _, s := range []string{"", "x", "nn", "now", "24", "-1", "123", "2400", "1460", "12:60", "12345", "1430151"} {
		t.Run("invalid "+s, func(t *testing.T) {
			_, err := ParseTime(s, now)
			assert.Error(t, err)
		})
	}
}

func TestCanonical(t *testing.T) {
	now := time.Date(2026, 11, 12, 18, 30, 27, 0, time.UTC)
	tests := []struct {
		name string
		k    Field
		s    string
		want string
	}{
		{"date short form", Date, "t", "2026-11-12"},
		{"date off short form", DateOff, "y", "2026-11-11"},
		{"compact date", Date, "20261103", "2026-11-03"},
		{"canonical date", Date, "2026-11-03", "2026-11-03"},
		{"date with spaces", Date, " 1103 ", "2026-11-03"},
		{"time short form", TimeOn, "1430", "14:30"},
		{"time off short form", TimeOff, "14", "14:00"},
		{"time with seconds", TimeOn, "143015", "14:30:15"},
		{"time now", TimeOn, "n", "18:30:27"},
		{"canonical time", TimeOn, "14:30", "14:30"},
		{"invalid date kept", Date, "2026-13-01", "2026-13-01"},
		{"invalid time kept", TimeOn, "25", "25"},
		{"empty kept", Date, "", ""},
		{"blank kept", TimeOn, "  ", "  "},
		{"other field kept", Call, "t", "t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Canonical(tt.k, tt.s, now))
		})
	}
}

func TestOffDefaults(t *testing.T) {
	now := time.Date(2026, 11, 12, 18, 30, 0, 0, time.UTC)
	tests := []struct {
		name                     string
		date, on, off            string
		wantDateOff, wantTimeOff string
	}{
		{"copies the on fields", "2026-11-10", "14:30", "", "2026-11-10", "14:30"},
		{"copies short forms canonically", "y", "1430", "", "2026-11-11", "14:30"},
		{"time off typed later", "2026-11-10", "14:30", "1445", "2026-11-10", "14:30"},
		{"time off typed earlier crosses midnight", "2026-11-10", "23:50", "0010", "2026-11-11", "23:50"},
		{"crosses a month end", "2026-10-31", "2350", "0005", "2026-11-01", "23:50"},
		{"time off equal stays", "2026-11-10", "14:30", "14:30", "2026-11-10", "14:30"},
		{"no time on", "2026-11-10", "", "", "2026-11-10", ""},
		{"invalid time on", "2026-11-10", "25", "", "2026-11-10", ""},
		{"invalid time off doesn't cross", "2026-11-10", "14:30", "99", "2026-11-10", "14:30"},
		{"no date", "", "14:30", "", "", "14:30"},
		{"invalid date", "2026-13-01", "14:30", "", "", "14:30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f Form
			f[Date], f[TimeOn], f[TimeOff] = tt.date, tt.on, tt.off
			dateOff, timeOff := f.OffDefaults(now)
			assert.Equal(t, tt.wantDateOff, dateOff, "date off")
			assert.Equal(t, tt.wantTimeOff, timeOff, "time off")
		})
	}
}

func TestBuildShortForms(t *testing.T) {
	now := time.Date(2026, 11, 12, 18, 30, 0, 0, time.UTC)
	f := validForm()
	f[Date], f[TimeOn], f[DateOff], f[TimeOff] = "y", "23", "t", "0010"
	q, err := Build(f, now)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 11, 11, 23, 0, 0, 0, time.UTC), q.StartUTC)
	assert.Equal(t, time.Date(2026, 11, 12, 0, 10, 0, 0, time.UTC), q.EndUTC.V)

	f[Date], f[TimeOn] = "1113", "n"
	_, err = Build(f, now)
	require.NoError(t, err, "a month and day after today is last year's")
}
