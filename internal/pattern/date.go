package pattern

import (
	"fmt"
	"time"

	"jstlogger/internal/logbook"
)

const (
	// dateLayout is the layout of a ~d date.
	dateLayout = "2006-01-02"
	// dateLen is the length of a date in dateLayout. Dates contain '-', so
	// ranges are split by position rather than by searching for it.
	dateLen = len(dateLayout)
)

// parseDateRange parses the value of a ~d term into a Filter on StartUTC.
// Dates are UTC days and every bound is inclusive of whole days: "D" is
// that day, "D-" is D onwards, "-D" is through the end of D and "D1-D2" is
// D1 through the end of D2. Errors wrap ErrSyntax.
func parseDateRange(s string) (logbook.Filter, error) {
	var fromStr, toStr string
	switch {
	case len(s) == dateLen:
		fromStr, toStr = s, s
	case len(s) == dateLen+1 && s[dateLen] == '-':
		fromStr = s[:dateLen]
	case len(s) == dateLen+1 && s[0] == '-':
		toStr = s[1:]
	case len(s) == 2*dateLen+1 && s[dateLen] == '-':
		fromStr, toStr = s[:dateLen], s[dateLen+1:]
	default:
		return logbook.Filter{}, fmt.Errorf("invalid date range %q (want D, D-, -D or D1-D2 with dates as YYYY-MM-DD): %w", s, ErrSyntax)
	}

	var f logbook.Filter
	if fromStr != "" {
		from, err := parseDate(fromStr)
		if err != nil {
			return logbook.Filter{}, err
		}
		f.From = from
	}
	if toStr != "" {
		to, err := parseDate(toStr)
		if err != nil {
			return logbook.Filter{}, err
		}
		if !f.From.IsZero() && to.Before(f.From) {
			return logbook.Filter{}, fmt.Errorf("invalid date range %q: end %s is before start %s: %w", s, toStr, fromStr, ErrSyntax)
		}
		// Filter.To is exclusive, so include all of the last day.
		f.To = to.AddDate(0, 0, 1)
	}
	return f, nil
}

// parseDate parses one YYYY-MM-DD date as midnight UTC.
func parseDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation(dateLayout, s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: %w: %w", s, err, ErrSyntax)
	}
	return t, nil
}
