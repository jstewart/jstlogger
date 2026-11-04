package indexfmt

import (
	"fmt"
	"strings"
	"time"
)

// strftimeConversions are the conversion letters the strftime subset
// supports, besides %%.
const strftimeConversions = "YymdejHMSbBaA"

// timeConversions are the conversion letters that format a time of day
// rather than a date.
const timeConversions = "HMS"

// checkStrftime reports an error wrapping ErrSyntax if layout uses a
// conversion outside the supported subset or ends in a lone '%'. base is
// the 0-based rune offset of layout within the whole format, so the error
// names the column of the offending '%' in the format.
func checkStrftime(layout string, base int) error {
	rs := []rune(layout)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '%' {
			continue
		}
		if i+1 >= len(rs) {
			return errorf(base+i, "lone %%")
		}
		i++
		if rs[i] != '%' && !strings.ContainsRune(strftimeConversions, rs[i]) {
			return errorf(base+i-1, "unknown strftime conversion %q", "%"+string(rs[i]))
		}
	}
	return nil
}

// timeOnly reports whether layout has at least one time-of-day conversion
// (%H, %M or %S) and no other conversions besides %%, so its column is a
// time rather than a date. layout must have passed checkStrftime.
func timeOnly(layout string) bool {
	hasTime := false
	rs := []rune(layout)
	for i := 0; i+1 < len(rs); i++ {
		if rs[i] != '%' {
			continue
		}
		i++
		switch {
		case rs[i] == '%':
		case strings.ContainsRune(timeConversions, rs[i]):
			hasTime = true
		default:
			return false
		}
	}
	return hasTime
}

// strftime formats t with layout, a strftime subset: %Y %y %m %d %e %j %H
// %M %S %b %B %a %A and %%. Other text is copied verbatim. layout must have
// passed checkStrftime; an unsupported conversion is copied verbatim.
func strftime(t time.Time, layout string) string {
	var b strings.Builder
	rs := []rune(layout)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '%' || i+1 >= len(rs) {
			b.WriteRune(rs[i])
			continue
		}
		i++
		switch rs[i] {
		case 'Y':
			fmt.Fprintf(&b, "%04d", t.Year())
		case 'y':
			fmt.Fprintf(&b, "%02d", t.Year()%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'e':
			fmt.Fprintf(&b, "%2d", t.Day())
		case 'j':
			fmt.Fprintf(&b, "%03d", t.YearDay())
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'M':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 'S':
			fmt.Fprintf(&b, "%02d", t.Second())
		case 'b':
			b.WriteString(t.Month().String()[:3])
		case 'B':
			b.WriteString(t.Month().String())
		case 'a':
			b.WriteString(t.Weekday().String()[:3])
		case 'A':
			b.WriteString(t.Weekday().String())
		case '%':
			b.WriteByte('%')
		default:
			b.WriteRune('%')
			b.WriteRune(rs[i])
		}
	}
	return b.String()
}
