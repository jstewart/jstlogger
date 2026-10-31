package adif

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Diff compares two record sets semantically and returns human-readable
// differences, or nil when they are equivalent.
//
// Records are compared as a multiset, ignoring record order and field order.
// Field names are compared case-insensitively; values must match exactly.
// Fields with an empty value are treated as absent. Fields named in ignore
// (case-insensitive) are skipped on both sides. Each difference names the
// record by its canonical form, its NAME=VALUE pairs sorted by name, and the
// output is sorted so it is deterministic.
func Diff(a, b []Record, ignore ...string) []string {
	skip := make(map[string]bool, len(ignore))
	for _, name := range ignore {
		skip[strings.ToUpper(name)] = true
	}

	// counts holds, per canonical record, how many more times it appears in
	// a than in b.
	counts := make(map[string]int)
	for _, r := range a {
		counts[canonical(r, skip)]++
	}
	for _, r := range b {
		counts[canonical(r, skip)]--
	}

	var first, second []string
	for _, key := range slices.Sorted(maps.Keys(counts)) {
		switch n := counts[key]; {
		case n > 0:
			first = append(first, describe("only in first", n, key))
		case n < 0:
			second = append(second, describe("only in second", -n, key))
		}
	}
	return append(first, second...)
}

// canonical returns a record's sorted, space-separated NAME="VALUE" pairs,
// leaving out empty values and skipped names. Values are quoted so that
// spaces inside them cannot make two different records look the same.
func canonical(r Record, skip map[string]bool) string {
	pairs := make([]string, 0, len(r))
	for _, f := range r {
		name := strings.ToUpper(f.Name)
		if f.Value == "" || skip[name] {
			continue
		}
		pairs = append(pairs, fmt.Sprintf("%s=%q", name, f.Value))
	}
	slices.Sort(pairs)
	return strings.Join(pairs, " ")
}

// describe formats one difference line, noting the count when above one.
func describe(side string, n int, key string) string {
	if n > 1 {
		return fmt.Sprintf("%s (x%d): %s", side, n, key)
	}
	return fmt.Sprintf("%s: %s", side, key)
}
