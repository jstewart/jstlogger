package tui

// patternHistorySize caps the pattern history; the oldest patterns drop off.
const patternHistorySize = 100

// history is the input entered at a prompt, oldest first, which the prompt
// browses with the history keys.
type history struct {
	entries []string
	// pos is the entry in the prompt while browsing, or end() when the
	// prompt shows the input typed before browsing began.
	pos int
	// draft is the input typed before browsing began.
	draft string
}

// end is the position of the draft: past the newest entry, or on it when the
// draft is the same, so browsing never shows the same input twice running.
// A prompt opened with the last pattern to edit starts this way.
func (h *history) end() int {
	n := len(h.entries)
	if n > 0 && h.entries[n-1] == h.draft {
		return n - 1
	}
	return n
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
// at the oldest. cur is the prompt's input, kept as the draft when the prompt
// shows the draft.
func (h *history) prev(cur string) (string, bool) {
	if h.pos >= h.end() {
		h.draft = cur
		h.pos = h.end()
	}
	if h.pos == 0 {
		return "", false
	}
	h.pos--
	return h.entries[h.pos], true
}

// next returns the entry newer than the one in the prompt, or the draft past
// the newest, reporting false when the prompt already shows the draft.
func (h *history) next() (string, bool) {
	if h.pos >= h.end() {
		return "", false
	}
	h.pos++
	if h.pos == h.end() {
		return h.draft, true
	}
	return h.entries[h.pos], true
}
