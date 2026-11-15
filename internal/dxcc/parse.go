package dxcc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// continents are the two-letter continent codes cty.csv uses.
var continents = []string{"AF", "AN", "AS", "EU", "NA", "OC", "SA"}

// overrideOpen are the characters that open a token's override suffixes:
// (CQ zone), [ITU zone], <lat/lon>, {continent} and ~UTC offset~.
const overrideOpen = "([<{~"

// overrideClose maps each override's opening character to its closing one.
var overrideClose = map[byte]byte{'(': ')', '[': ']', '<': '>', '{': '}', '~': '~'}

// line is one parsed cty.csv entity line.
type line struct {
	num    int
	entity Entity
	wae    bool // the primary prefix starts with "*": a WAE or CQ-only entity
	tokens string
}

// Parse reads a cty.csv country file. Each line holds one entity: its
// primary prefix, name, DXCC number, continent, CQ and ITU zones, latitude,
// longitude, UTC offset and its space-separated prefixes and exact calls.
//
// An entity whose primary prefix starts with "*" is on the WAE or CQ lists
// but is not a DXCC entity; its calls keep their own zones and location but
// take the name of the DXCC entity with the same number.
func Parse(r io.Reader) (*Table, error) {
	var lines []line
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 1<<20)
	for num := 1; sc.Scan(); num++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		l, err := parseLine(text)
		if err != nil {
			return nil, fmt.Errorf("cty.csv line %d: %w", num, err)
		}
		l.num = num
		lines = append(lines, l)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading cty.csv: %w", err)
	}

	names := make(map[int]string)
	for _, l := range lines {
		if !l.wae {
			names[l.entity.DXCC] = l.entity.Name
		}
	}

	t := &Table{exact: make(map[string]Entity), prefixes: make(map[string]Entity)}
	for _, l := range lines {
		if l.wae {
			name, ok := names[l.entity.DXCC]
			if !ok {
				return nil, fmt.Errorf("cty.csv line %d: no DXCC entity %d for %s", l.num, l.entity.DXCC, l.entity.Name)
			}
			l.entity.Name = name
		}
		for tok := range strings.FieldsSeq(l.tokens) {
			if err := t.add(tok, l.entity); err != nil {
				return nil, fmt.Errorf("cty.csv line %d: %w", l.num, err)
			}
		}
	}
	return t, nil
}

// parseLine parses the ten comma-separated fields of an entity line.
func parseLine(text string) (line, error) {
	fields := strings.Split(text, ",")
	if len(fields) != 10 {
		return line{}, fmt.Errorf("want 10 fields, got %d", len(fields))
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}

	var (
		l   line
		err error
		e   = &l.entity
	)
	l.wae = strings.HasPrefix(fields[0], "*")
	e.Name = fields[1]
	if e.DXCC, err = strconv.Atoi(fields[2]); err != nil {
		return line{}, fmt.Errorf("DXCC number: %w", err)
	}
	if e.Continent, err = parseContinent(fields[3]); err != nil {
		return line{}, err
	}
	if e.CQZone, err = strconv.Atoi(fields[4]); err != nil {
		return line{}, fmt.Errorf("CQ zone: %w", err)
	}
	if e.ITUZone, err = strconv.Atoi(fields[5]); err != nil {
		return line{}, fmt.Errorf("ITU zone: %w", err)
	}
	if e.Lat, err = strconv.ParseFloat(fields[6], 64); err != nil {
		return line{}, fmt.Errorf("latitude: %w", err)
	}
	if e.Lon, err = parseLon(fields[7]); err != nil {
		return line{}, fmt.Errorf("longitude: %w", err)
	}
	if _, err = strconv.ParseFloat(fields[8], 64); err != nil {
		return line{}, fmt.Errorf("UTC offset: %w", err)
	}
	l.tokens = strings.TrimSuffix(fields[9], ";")
	return l, nil
}

// add parses one prefix or "=CALL" token, applies its overrides to e and
// stores it in the table.
func (t *Table) add(tok string, e Entity) error {
	key, rest := tok, ""
	if i := strings.IndexAny(tok, overrideOpen); i >= 0 {
		key, rest = tok[:i], tok[i:]
	}
	exact := strings.HasPrefix(key, "=")
	key = strings.TrimPrefix(key, "=")
	if key == "" {
		return fmt.Errorf("token %q: no prefix or call", tok)
	}
	if err := applyOverrides(rest, &e); err != nil {
		return fmt.Errorf("token %q: %w", tok, err)
	}
	if exact {
		t.exact[key] = e
		return nil
	}
	t.prefixes[key] = e
	t.maxPrefixLen = max(t.maxPrefixLen, len(key))
	return nil
}

// applyOverrides applies a token's override suffixes, such as "(5)[8]",
// to e.
func applyOverrides(s string, e *Entity) error {
	for s != "" {
		closer := overrideClose[s[0]]
		if closer == 0 {
			return fmt.Errorf("unexpected %q", s)
		}
		end := strings.IndexByte(s[1:], closer)
		if end < 0 {
			return fmt.Errorf("unterminated %q", s)
		}
		val := s[1 : end+1]
		var err error
		switch s[0] {
		case '(':
			e.CQZone, err = strconv.Atoi(val)
		case '[':
			e.ITUZone, err = strconv.Atoi(val)
		case '<':
			err = parseLatLon(val, e)
		case '{':
			e.Continent, err = parseContinent(val)
		case '~':
			_, err = strconv.ParseFloat(val, 64)
		}
		if err != nil {
			return fmt.Errorf("override %q: %w", s[:end+2], err)
		}
		s = s[end+2:]
	}
	return nil
}

// parseLatLon parses a "lat/lon" override, longitude positive west.
func parseLatLon(s string, e *Entity) error {
	latStr, lonStr, ok := strings.Cut(s, "/")
	if !ok {
		return errors.New("want lat/lon")
	}
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return fmt.Errorf("latitude: %w", err)
	}
	lon, err := parseLon(lonStr)
	if err != nil {
		return fmt.Errorf("longitude: %w", err)
	}
	e.Lat, e.Lon = lat, lon
	return nil
}

// parseLon parses a cty.csv longitude, which is positive west, and returns
// it positive east.
func parseLon(s string) (float64, error) {
	lon, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return -lon, nil
}

// parseContinent checks that s is a known continent code.
func parseContinent(s string) (string, error) {
	if !slices.Contains(continents, s) {
		return "", fmt.Errorf("continent %q: unknown", s)
	}
	return s, nil
}
