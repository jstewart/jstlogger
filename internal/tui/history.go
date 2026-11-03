package tui

// patternHistorySize caps the pattern history; the oldest patterns drop off.
const patternHistorySize = 100

// history is the input entered at a prompt, oldest first, which the prompt
// browses with the history keys.
type history struct {
	entries []string
	// pos is the entry in the prompt while browsing, or len(entries) when
	// the prompt shows the input typed before browsing began.
	pos int
	// draft is the input typed before browsing began.
	draft string
}

// add records s, unless it is empty or the same as the newest entry.
func (h *history) add(s string) {
	n := len(h.entries)
	if s == "" || n > 0 && h.entries[n-1] == s {
		return
	}
	// Capping the capacity makes append copy, so a Model copied before the
	// add keeps its own entries.
	h.entries = append(h.entries[max(n-patternHistorySize+1, 0):n:n], s)
}

// reset starts browsing again from the newest entry.
func (h *history) reset() {
	h.pos = len(h.entries)
	h.draft = ""
}

// prev returns the entry older than the one in the prompt, reporting false
// at the oldest. cur is the prompt's input, kept as the draft when browsing
// begins.
func (h *history) prev(cur string) (string, bool) {
	if h.pos == 0 {
		return "", false
	}
	if h.pos == len(h.entries) {
		h.draft = cur
	}
	h.pos--
	return h.entries[h.pos], true
}

// next returns the entry newer than the one in the prompt, or the draft past
// the newest, reporting false when the prompt already shows the draft.
func (h *history) next() (string, bool) {
	if h.pos >= len(h.entries) {
		return "", false
	}
	h.pos++
	if h.pos == len(h.entries) {
		return h.draft, true
	}
	return h.entries[h.pos], true
}
