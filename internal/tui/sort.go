package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/logbook"
)

// sortChoice is a field the sort prompt offers: the key that picks it and
// its name with that key in parentheses.
type sortChoice struct {
	key   key.Binding
	label string
	field logbook.SortField
}

// sortChoices are the fields the sort prompt offers, in the order it lists
// them. Their keys mirror the pattern letters that limit by the same
// fields.
var sortChoices = []sortChoice{
	{key.NewBinding(key.WithKeys("d")), "(d)ate", logbook.SortDate},
	{key.NewBinding(key.WithKeys("c")), "(c)all", logbook.SortCall},
	{key.NewBinding(key.WithKeys("b")), "(b)and", logbook.SortBand},
	{key.NewBinding(key.WithKeys("m")), "(m)ode", logbook.SortMode},
	{key.NewBinding(key.WithKeys("C")), "(C)ountry", logbook.SortCountry},
	{key.NewBinding(key.WithKeys("s")), "(s)tate", logbook.SortState},
	{key.NewBinding(key.WithKeys("o")), "c(o)ntinent", logbook.SortContinent},
}

// sortQuestion returns the sort prompt's text, listing the fields with the
// key that picks each, for a reverse sort when reverse is true.
func sortQuestion(reverse bool) string {
	labels := make([]string, len(sortChoices))
	for i, c := range sortChoices {
		labels[i] = c.label
	}
	q := "Sort by: "
	if reverse {
		q = "Reverse sort by: "
	}
	return q + strings.Join(labels, " ")
}

// askSort opens the sort prompt, which a single key press answers, for a
// reverse sort when reverse is true.
func (m Model) askSort(reverse bool) Model {
	m = m.closePrompt()
	m.prompt = promptSort
	if reverse {
		m.prompt = promptSortReverse
	}
	return m
}

// answerSort closes the sort prompt and sorts the index by the field msg
// picks. Any other key, esc included, cancels silently.
func (m Model) answerSort(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	reverse := m.prompt == promptSortReverse
	m = m.closePrompt()
	for _, c := range sortChoices {
		if key.Matches(msg, c.key) {
			return m.sortIndex(logbook.Sort{Field: c.field, Reverse: reverse})
		}
	}
	return m, nil
}

// sortIndex sorts the index by s and returns a command that reloads it,
// keeping the cursor on the same QSO. The rows cached in the old order are
// dropped, so none is shown while the new order loads. Sorting by the
// current sort does nothing.
func (m Model) sortIndex(s logbook.Sort) (Model, tea.Cmd) {
	x := &m.idx
	if s == x.sort {
		return m, nil
	}
	x.sort = s
	id := x.restart()
	x.settling = true
	x.blocks = map[int][]logbook.QSO{}
	return m, loadIndexCmd(m.logbook, x.gen, x.limit.expr, x.sort, id, x.cursor, x.wantsTags())
}

// WithIndexSort sorts the index by s, rather than by date, newest first,
// until it is sorted otherwise.
func WithIndexSort(s logbook.Sort) Option {
	return func(m *Model) {
		m.idx.sort = s
	}
}
