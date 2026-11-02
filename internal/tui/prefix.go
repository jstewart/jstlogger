package tui

import (
	"maps"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// markedFunc is an index function that accepts the mark-prefix: on ;
// followed by its key, it runs on every marked QSO. Its binding must be
// enabled, since key.Matches ignores disabled bindings. A function that
// opens a prompt should look at the marks again when the prompt is
// submitted, as they may have changed.
type markedFunc struct {
	keys key.Binding
	// run runs the function on the QSOs with the given IDs, in ascending
	// order.
	run func(m Model, ids []int64) (Model, tea.Cmd)
}

// armPrefix arms the mark-prefix, so the next key applies to the marked
// QSOs. As in mutt, it is not armed when no QSO is marked.
func (x *index) armPrefix() {
	if len(x.marks) == 0 {
		x.msg = msgNoMarks
		return
	}
	x.prefix = true
}

// notMarkedMsg is the status message for a key, such as "j", that does not
// accept the mark-prefix.
func notMarkedMsg(k string) string {
	return k + " does not apply to marked QSOs."
}

// updatePrefix handles the key press that follows the mark-prefix. Quit
// still quits and esc cancels the prefix silently. A key a marked function
// accepts runs that function on every marked QSO; any other key cancels the
// prefix without running its own function.
func (m Model) updatePrefix(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	m.idx.prefix = false
	if key.Matches(msg, m.indexKeys.PrefixCancel) {
		return m, nil
	}
	for _, f := range m.markedFuncs {
		if !key.Matches(msg, f.keys) {
			continue
		}
		// An unmark pattern that finished after ; may have left nothing
		// marked.
		if len(m.idx.marks) == 0 {
			m.idx.msg = msgNoMarks
			return m, nil
		}
		return f.run(m, slices.Sorted(maps.Keys(m.idx.marks)))
	}
	m.idx.msg = notMarkedMsg(msg.String())
	return m, nil
}
