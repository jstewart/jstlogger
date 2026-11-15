package dxcc

import (
	"bytes"
	_ "embed"
	"fmt"
	"slices"
	"strings"
	"sync"

	"jstlogger/internal/callsign"
)

// ctyCSV is the snapshot of AD1C's Big CTY cty.csv, refreshed by `just cty`.
//
//go:embed cty.csv
var ctyCSV []byte

// Entity is a DXCC entity, with the zones, continent and location for a
// call in it.
type Entity struct {
	DXCC      int    // ADIF DXCC entity number
	Name      string // entity name, as in cty.csv
	Continent string // two letters: AF AN AS EU NA OC SA
	CQZone    int
	ITUZone   int
	Lat, Lon  float64 // degrees, north and east positive
}

// Table resolves callsigns to DXCC entities. Build one with Parse, or use
// the embedded snapshot from Default.
type Table struct {
	exact        map[string]Entity // calls listed as "=CALL"
	prefixes     map[string]Entity
	maxPrefixLen int
}

// Default returns the table parsed from the embedded cty.csv snapshot. It
// parses the file once, on first use, and panics if the file is invalid.
func Default() *Table { return defaultTable() }

var defaultTable = sync.OnceValue(func() *Table {
	t, err := Parse(bytes.NewReader(ctyCSV))
	if err != nil {
		panic(fmt.Sprintf("dxcc: embedded cty.csv: %v", err))
	}
	return t
})

// Lookup returns the DXCC entity for call. It reports false when call is
// empty, has no known prefix, or is maritime or aeronautical mobile (/MM or
// /AM), which is in no entity.
//
// A call listed exactly in the country file, such as KC4/W1WCN, resolves to
// that listing. Otherwise a listed country prefix such as VE3/W1AW or
// W1AW/VE3 decides the entity, a call area digit as in UA9ABC/1 replaces the base
// call's first digit, and modifiers such as /P and /QRP are ignored. The
// base call's longest listed prefix then names the entity.
func (t *Table) Lookup(call string) (Entity, bool) {
	call = strings.ToUpper(strings.TrimSpace(call))
	if call == "" {
		return Entity{}, false
	}
	parts := strings.Split(call, "/")
	if slices.Contains(parts, "MM") || slices.Contains(parts, "AM") {
		return Entity{}, false
	}
	if e, ok := t.exact[call]; ok {
		return e, true
	}

	base := callsign.Base(call)
	if base == "" {
		return Entity{}, false
	}
	var prefix, area string
	seenBase := false
	for _, part := range parts {
		switch {
		case len(part) == 1 && part[0] >= '0' && part[0] <= '9':
			area = part
		case callsign.Base(part) == "": // empty or a modifier
		case part == base && !seenBase:
			seenBase = true
		case prefix == "":
			prefix = part
		}
	}

	if prefix != "" {
		// A part that is no listed prefix, such as /QRO or /T, is taken
		// for a suffix the call keeps its own entity under.
		if e, ok := t.longestPrefix(prefix, false); ok {
			return e, true
		}
	}
	if area != "" {
		if i := strings.IndexAny(base, "0123456789"); i >= 0 {
			base = base[:i] + area + base[i+1:]
		}
		return t.longestPrefix(base, true)
	}
	if e, ok := t.exact[base]; ok {
		return e, true
	}
	return t.longestPrefix(base, true)
}

// longestPrefix returns the entity of the longest listed prefix of s. When
// s is a full call, it applies AD1C's rule that KG4 is Guantanamo Bay only
// for KG4 followed by exactly two letters.
func (t *Table) longestPrefix(s string, fullCall bool) (Entity, bool) {
	for n := min(len(s), t.maxPrefixLen); n > 0; n-- {
		e, ok := t.prefixes[s[:n]]
		if !ok {
			continue
		}
		if fullCall && s[:n] == "KG4" && !isGuantanamo(s) {
			continue
		}
		return e, true
	}
	return Entity{}, false
}

// isGuantanamo reports whether call is KG4 followed by exactly two letters.
func isGuantanamo(call string) bool {
	return len(call) == 5 && strings.HasPrefix(call, "KG4") &&
		isLetter(call[3]) && isLetter(call[4])
}

func isLetter(c byte) bool { return c >= 'A' && c <= 'Z' }
