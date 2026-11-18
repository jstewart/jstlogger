package qsoentry

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DateLayout and TimeLayout are the canonical layouts of the form's dates
// and times. A time with seconds is written with TimeSecondsLayout.
const (
	DateLayout        = "2006-01-02"
	TimeLayout        = "15:04"
	TimeSecondsLayout = "15:04:05"
)

// dateLayouts are the full layouts a date can be typed in.
var dateLayouts = []string{DateLayout, "20060102"}

// clockLayouts are the full layouts a time can be typed in.
var clockLayouts = []string{TimeLayout, TimeSecondsLayout, "1504", "150405"}

// oneDay is the length of a day in UTC, which has no daylight saving.
const oneDay = 24 * time.Hour

// ParseDate parses a date typed into the form, as midnight UTC. Besides
// YYYY-MM-DD and YYYYMMDD, it reads short forms counting from now's UTC
// date: t is today, y yesterday and -N N days ago. MMDD and MM-DD are this
// year's date, or last year's when this year's is after today or doesn't
// exist, since a QSO can't be in the future.
func ParseDate(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	today := now.UTC().Truncate(oneDay)
	switch {
	case s == "t":
		return today, nil
	case s == "y":
		return today.Add(-oneDay), nil
	case strings.HasPrefix(s, "-") && allDigits(s[1:]):
		n, err := strconv.Atoi(s[1:])
		if err != nil {
			return time.Time{}, fmt.Errorf("parse date %q: %w", s, err)
		}
		return today.AddDate(0, 0, -n), nil
	case len(s) == 4 && allDigits(s):
		return monthDay(s[:2], s[2:], today)
	case len(s) == 5 && s[2] == '-' && allDigits(s[:2]) && allDigits(s[3:]):
		return monthDay(s[:2], s[3:], today)
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse date %q: not YYYY-MM-DD, t, y, -N or MMDD", s)
}

// monthDay returns the date in month mm on day dd of today's year, or of
// the year before when that is after today or doesn't exist.
func monthDay(mm, dd string, today time.Time) (time.Time, error) {
	for _, year := range []int{today.Year(), today.Year() - 1} {
		t, err := time.Parse(DateLayout, fmt.Sprintf("%04d-%s-%s", year, mm, dd))
		if err == nil && !t.After(today) {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse date %s-%s: no such month and day this year or last", mm, dd)
}

// ParseTime parses a time of day typed into the form, UTC, as the time
// since midnight. Besides HH:MM, HH:MM:SS, HHMM and HHMMSS, it reads n as
// now's time of day, to the second, and an hour of one or two digits as
// that hour on the hour.
func ParseTime(s string, now time.Time) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "n":
		return now.UTC().Sub(now.UTC().Truncate(oneDay)).Truncate(time.Second), nil
	case len(s) >= 1 && len(s) <= 2 && allDigits(s):
		h, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("parse time %q: %w", s, err)
		}
		if h > 23 {
			return 0, fmt.Errorf("parse time %q: hour out of range", s)
		}
		return time.Duration(h) * time.Hour, nil
	}
	for _, layout := range clockLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			h, m, sec := t.Clock()
			return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second, nil
		}
	}
	return 0, fmt.Errorf("parse time %q: not HH:MM, HH:MM:SS, n or HH", s)
}

// FormatTime writes the time of day d, since midnight, in the canonical
// layout: HH:MM, or HH:MM:SS when it has seconds.
func FormatTime(d time.Duration) string {
	t := time.Time{}.Add(d)
	if t.Second() != 0 {
		return t.Format(TimeSecondsLayout)
	}
	return t.Format(TimeLayout)
}

// Canonical returns s, typed into field k, rewritten in the canonical
// layout of a date or time, so a short form shows what it was read as. It
// returns s as it is when k is not a date or time field, or s doesn't
// parse.
func Canonical(k Field, s string, now time.Time) string {
	switch k {
	case Date, DateOff:
		if t, err := ParseDate(s, now); err == nil {
			return t.Format(DateLayout)
		}
	case TimeOn, TimeOff:
		if d, err := ParseTime(s, now); err == nil {
			return FormatTime(d)
		}
	}
	return s
}

// OffDefaults returns the date off and time off that f's off fields copy
// from its on fields while they are blank: the time on, and the date, or
// the day after when the time off is earlier than the time on, crossing
// midnight. Each is "" when what it copies doesn't parse.
func (f Form) OffDefaults(now time.Time) (dateOff, timeOff string) {
	on, onErr := ParseTime(f[TimeOn], now)
	if onErr == nil {
		timeOff = FormatTime(on)
	}
	date, err := ParseDate(f[Date], now)
	if err != nil {
		return "", timeOff
	}
	if strings.TrimSpace(f[TimeOff]) != "" {
		if off, err := ParseTime(f[TimeOff], now); err == nil && onErr == nil && off < on {
			date = date.Add(oneDay)
		}
	}
	return date.Format(DateLayout), timeOff
}

// allDigits reports whether s is one or more ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
