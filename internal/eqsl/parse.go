package eqsl

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// headerPrefix starts the list's header line, which names the time the
// list was generated.
const headerPrefix = "List of AG members"

// maxLineBytes is the longest line Parse accepts. Real lines are a
// callsign; the limit only has to be large enough that an unexpected body,
// such as a minified HTML page, is skipped line by line rather than failing.
const maxLineBytes = 1 << 20

// Parse reads an eQSL AG member list from r. It returns the upper-case
// callsigns in the order they first appear, each once, and the number of
// malformed lines it skipped: lines holding anything but ASCII letters,
// digits and '/'. Blank lines and the header line are ignored without being
// counted. It returns an error only when r cannot be read.
func Parse(r io.Reader) (calls []string, skipped int, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	seen := make(map[string]struct{})
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, headerPrefix) {
			continue
		}
		call, ok := parseCall(line)
		if !ok {
			skipped++
			continue
		}
		if _, dup := seen[call]; dup {
			continue
		}
		seen[call] = struct{}{}
		calls = append(calls, call)
	}
	if err := sc.Err(); err != nil {
		return nil, skipped, fmt.Errorf("eqsl: parse: %w", err)
	}
	return calls, skipped, nil
}

// parseCall upper-cases a trimmed, non-empty line, reporting false if it
// holds any byte other than an ASCII letter, digit or '/'.
func parseCall(line string) (string, bool) {
	b := []byte(line)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = c - ('a' - 'A')
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '/':
		default:
			return "", false
		}
	}
	return string(b), true
}
