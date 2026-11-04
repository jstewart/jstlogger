package indexfmt

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"jstlogger/internal/logbook"
)

// Default reproduces the built-in index columns: mark, date, time, call,
// mode, RSTs, band, a flexible country column and the station callsign.
const Default = "%Z %-10{%Y-%m-%d} %-5{%H:%M} %-10c %-6m %-5s %-5r %-5b %-*C %-10S"

// ErrSyntax is wrapped by every error Parse returns for a malformed format.
var ErrSyntax = errors.New("syntax error")

// FieldTime is the Item.Field of a %{strftime} expando, whose layout is in
// Item.Time.
const FieldTime byte = '{'

// Unit is a distance unit for the %D expando. Its values match the config
// distance_unit strings, so a caller can convert with Unit(s).
type Unit string

const (
	// Km formats distances in kilometers.
	Km Unit = "km"
	// Mi formats distances in international miles.
	Mi Unit = "mi"
)

// Format is a parsed index format: a sequence of items, each producing one
// header title and one cell value per QSO.
type Format struct {
	// Items are the format's literals and expandos, in order.
	Items []Item
}

// Item is one part of a Format: either literal text or an expando. An Item
// is a literal when Field is 0 (see IsLiteral); then only Literal is set.
// Otherwise Field is the expando letter, or FieldTime for %{...}.
type Item struct {
	// Literal is the text of a literal item, with %% already unescaped.
	Literal string
	// Field is the expando letter, FieldTime, or 0 for a literal.
	Field byte
	// Time is the strftime layout of a FieldTime item.
	Time string
	// Width is the fixed column width in cells, or 0 for natural width (or
	// for a flexible column). %Z with no width is given its intrinsic width
	// of 1.
	Width int
	// Left left-aligns the value in its width; otherwise it is right-aligned.
	Left bool
	// Flex marks the column that takes the remaining width. At most one
	// item in a Format is flexible.
	Flex bool
}

// IsLiteral reports whether it is literal text rather than an expando.
func (it Item) IsLiteral() bool { return it.Field == 0 }

// expando describes one expando letter: its header title and how to get
// its value from a QSO.
type expando struct {
	title string
	value func(q logbook.QSO, unit Unit) string
}

// expandos are the supported expando letters other than Z and {, which
// need the mark state and the strftime layout respectively.
var expandos = map[byte]expando{
	'c': {"Call", func(q logbook.QSO, _ Unit) string { return q.Call }},
	'm': {"Mode", func(q logbook.QSO, _ Unit) string {
		if q.Submode != "" {
			return q.Submode
		}
		return q.Mode
	}},
	's': {"RST S", func(q logbook.QSO, _ Unit) string { return q.RSTSent }},
	'r': {"RST R", func(q logbook.QSO, _ Unit) string { return q.RSTRcvd }},
	'b': {"Band", func(q logbook.QSO, _ Unit) string { return q.Band }},
	'f': {"Freq", func(q logbook.QSO, _ Unit) string {
		if !q.FreqHz.Valid {
			return ""
		}
		return formatFreqMHz(q.FreqHz.V)
	}},
	'C': {"Country", func(q logbook.QSO, _ Unit) string { return q.Country }},
	'S': {"Station", func(q logbook.QSO, _ Unit) string { return q.StationCallsign }},
	'o': {"Op", func(q logbook.QSO, _ Unit) string { return q.Operator }},
	'n': {"Name", func(q logbook.QSO, _ Unit) string { return q.Name }},
	'g': {"Grid", func(q logbook.QSO, _ Unit) string { return q.Gridsquare }},
	'D': {"Dist", func(q logbook.QSO, unit Unit) string {
		if !q.DistanceKm.Valid {
			return ""
		}
		d := q.DistanceKm.V
		if unit == Mi {
			d = logbook.KmToMiles(d)
		}
		return strconv.FormatFloat(math.Round(d), 'f', 0, 64)
	}},
	'k': {"Cont", func(q logbook.QSO, _ Unit) string { return q.Cont }},
	'x': {"DXCC", func(q logbook.QSO, _ Unit) string {
		if !q.DXCC.Valid {
			return ""
		}
		return strconv.Itoa(q.DXCC.V)
	}},
}

// errorf builds an error wrapping ErrSyntax that reports the 1-based
// column of the rune at offset pos.
func errorf(pos int, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("indexfmt: %s at column %d: %w", msg, pos+1, ErrSyntax)
}

// Parse parses an index format. See the package documentation for the
// syntax. Adjacent literal text, including %%, becomes a single literal
// item. Errors wrap ErrSyntax and name the column of the offending
// character.
func Parse(s string) (Format, error) {
	rs := []rune(s)
	var f Format
	var lit strings.Builder
	flushLit := func() {
		if lit.Len() > 0 {
			f.Items = append(f.Items, Item{Literal: lit.String()})
			lit.Reset()
		}
	}
	flex := false
	for i := 0; i < len(rs); i++ {
		if rs[i] != '%' {
			lit.WriteRune(rs[i])
			continue
		}
		start := i
		i++
		if i < len(rs) && rs[i] == '%' {
			lit.WriteByte('%')
			continue
		}
		var it Item
		if i < len(rs) && rs[i] == '-' {
			it.Left = true
			i++
		}
		switch {
		case i < len(rs) && rs[i] == '*':
			it.Flex = true
			i++
		case i < len(rs) && rs[i] >= '0' && rs[i] <= '9':
			j := i
			for j < len(rs) && rs[j] >= '0' && rs[j] <= '9' {
				j++
			}
			w, err := strconv.Atoi(string(rs[i:j]))
			if err != nil {
				return Format{}, errorf(start, "bad width %q", string(rs[i:j]))
			}
			if w == 0 {
				return Format{}, errorf(start, "zero width")
			}
			it.Width = w
			i = j
		}
		if i >= len(rs) {
			return Format{}, errorf(start, "lone %%")
		}
		if it.Flex {
			if flex {
				return Format{}, errorf(start, "more than one flexible column")
			}
			flex = true
		}
		switch c := rs[i]; {
		case c == '{':
			end := strings.IndexRune(string(rs[i+1:]), '}')
			if end < 0 {
				return Format{}, errorf(i, "unterminated {")
			}
			layout := string(rs[i+1:])[:end]
			if err := checkStrftime(layout, i+1); err != nil {
				return Format{}, err
			}
			it.Field = FieldTime
			it.Time = layout
			i += len([]rune(layout)) + 1
		case c == 'Z':
			it.Field = 'Z'
			if it.Width == 0 && !it.Flex {
				it.Width = 1
			}
		case c < 0x80 && expandos[byte(c)].value != nil:
			it.Field = byte(c)
		default:
			return Format{}, errorf(start, "unknown expando %q", "%"+string(c))
		}
		flushLit()
		f.Items = append(f.Items, it)
	}
	flushLit()
	return f, nil
}

// Titles returns the header title of each item, in order. A literal's
// title is its text.
func (f Format) Titles() []string {
	out := make([]string, len(f.Items))
	for i, it := range f.Items {
		switch {
		case it.IsLiteral():
			out[i] = it.Literal
		case it.Field == 'Z':
			out[i] = ""
		case it.Field == FieldTime:
			if timeOnly(it.Time) {
				out[i] = "Time"
			} else {
				out[i] = "Date"
			}
		default:
			out[i] = expandos[it.Field].title
		}
	}
	return out
}

// Values returns the cell value of each item for q, in order. A literal's
// value is its text. marked sets the %Z mark, and unit is the unit of %D;
// any unit other than Mi formats kilometers. Times are formatted in UTC.
// Values are not padded or truncated to the item widths.
func (f Format) Values(q logbook.QSO, marked bool, unit Unit) []string {
	out := make([]string, len(f.Items))
	for i, it := range f.Items {
		switch {
		case it.IsLiteral():
			out[i] = it.Literal
		case it.Field == 'Z':
			out[i] = " "
			if marked {
				out[i] = "*"
			}
		case it.Field == FieldTime:
			out[i] = strftime(q.StartUTC.UTC(), it.Time)
		default:
			if e, ok := expandos[it.Field]; ok {
				out[i] = e.value(q, unit)
			}
		}
	}
	return out
}

// formatFreqMHz formats hz as decimal MHz with trailing zeros trimmed, e.g.
// 14075219 as "14.075219" and 7000000 as "7". It mirrors the logbook
// package's unexported ADIF formatter.
func formatFreqMHz(hz int64) string {
	whole := strconv.FormatInt(hz/1_000_000, 10)
	frac := strings.TrimRight(fmt.Sprintf("%06d", hz%1_000_000), "0")
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}
