package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// helpSection is a titled group of keybindings the help modal lists.
type helpSection struct {
	title string
	keys  []key.Binding
}

// helpKeyMap is a help.KeyMap whose bindings the help modal lists in
// titled sections.
type helpKeyMap interface {
	help.KeyMap
	sections() []helpSection
}

// sectionKeys returns the bindings of each section, as FullHelp does.
func sectionKeys(sections []helpSection) [][]key.Binding {
	out := make([][]key.Binding, 0, len(sections))
	for _, s := range sections {
		out = append(out, s.keys)
	}
	return out
}

// keyMap holds the global keybindings. It implements help.KeyMap so the
// footer and the help modal stay in sync with the actual bindings.
type keyMap struct {
	Freq key.Binding
	Mode key.Binding
	Help key.Binding
	Quit key.Binding
}

// defaultKeys returns the global keybindings. The rig keys are disabled, and
// so hidden from help, unless rigEnabled is true.
func defaultKeys(rigEnabled bool) keyMap {
	k := keyMap{
		Freq: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "freq"),
		),
		Mode: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "mode"),
		),
		Help: key.NewBinding(
			key.WithKeys("?", "f1"),
			key.WithHelp("?/f1", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
	k.Freq.SetEnabled(rigEnabled)
	k.Mode.SetEnabled(rigEnabled)
	return k
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Freq, k.Mode, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return sectionKeys(k.sections())
}

func (k keyMap) sections() []helpSection {
	return []helpSection{
		{"Rig", []key.Binding{k.Freq, k.Mode}},
		{"General", []key.Binding{k.Help, k.Quit}},
	}
}

// promptKeyMap holds the keybindings handled while an input prompt is open;
// every other key goes to the prompt.
type promptKeyMap struct {
	Submit key.Binding
	Cancel key.Binding
	Quit   key.Binding
	// Help opens the help modal; ? is typed into the prompt.
	Help key.Binding
	// HistoryPrev and HistoryNext browse the pattern history. They are
	// enabled only while a pattern prompt is open.
	HistoryPrev key.Binding
	HistoryNext key.Binding
}

func defaultPromptKeys() promptKeyMap {
	return promptKeyMap{
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Help: helpKey(),
		HistoryPrev: key.NewBinding(
			key.WithKeys("up", "ctrl+p"),
			key.WithHelp("↑/ctrl+p", "prev pattern"),
			key.WithDisabled(),
		),
		HistoryNext: key.NewBinding(
			key.WithKeys("down", "ctrl+n"),
			key.WithHelp("↓/ctrl+n", "next pattern"),
			key.WithDisabled(),
		),
	}
}

func (k promptKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Cancel, k.HistoryPrev, k.HistoryNext, k.Help}
}

func (k promptKeyMap) FullHelp() [][]key.Binding {
	return sectionKeys(k.sections())
}

func (k promptKeyMap) sections() []helpSection {
	return []helpSection{
		{"Prompt", []key.Binding{k.Submit, k.Cancel, k.Help, k.Quit}},
		{"History", []key.Binding{k.HistoryPrev, k.HistoryNext}},
	}
}

// helpKey returns the binding that opens the help modal where ? is typed
// as text, as in a prompt or the New QSO screen.
func helpKey() key.Binding {
	return key.NewBinding(
		key.WithKeys("f1"),
		key.WithHelp("f1", "help"),
	)
}

// indexKeyMap holds the QSO index keybindings, modeled on NeoMutt's generic
// menu.
type indexKeyMap struct {
	Next       key.Binding
	Prev       key.Binding
	NextPage   key.Binding
	PrevPage   key.Binding
	HalfDown   key.Binding
	HalfUp     key.Binding
	NextLine   key.Binding
	PrevLine   key.Binding
	TopPage    key.Binding
	MiddlePage key.Binding
	BottomPage key.Binding
	First      key.Binding
	Last       key.Binding
	Jump       key.Binding
	Open       key.Binding
	NewQSO     key.Binding
	// Edit opens the QSO under the cursor in the QSO form.
	Edit           key.Binding
	Mark           key.Binding
	MarkPattern    key.Binding
	UnmarkPattern  key.Binding
	Limit          key.Binding
	Sort           key.Binding
	SortReverse    key.Binding
	Reload         key.Binding
	Search         key.Binding
	SearchReverse  key.Binding
	SearchNext     key.Binding
	SearchOpposite key.Binding
	Delete         key.Binding
	MarkPrefix     key.Binding
	// PrefixCancel cancels the mark-prefix. It has no help, so help does not
	// list it.
	PrefixCancel key.Binding
	// The bulk operations run only after the mark-prefix, so their help
	// shows the prefix with the key.
	BulkDelete key.Binding
	BulkSave   key.Binding
	BulkTag    key.Binding
	BulkUntag  key.Binding
}

// defaultIndexKeys returns the index keybindings, disabled, and so hidden
// from help, unless enabled is true.
func defaultIndexKeys(enabled bool) indexKeyMap {
	k := indexKeyMap{
		Next: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "next"),
		),
		Prev: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "prev"),
		),
		NextPage: key.NewBinding(
			key.WithKeys("z", "pgdown", "right"),
			key.WithHelp("z/pgdn", "next page"),
		),
		PrevPage: key.NewBinding(
			key.WithKeys("Z", "pgup", "left"),
			key.WithHelp("Z/pgup", "prev page"),
		),
		HalfDown: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "half page down"),
		),
		HalfUp: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "half page up"),
		),
		NextLine: key.NewBinding(
			key.WithKeys(">"),
			key.WithHelp(">", "scroll down"),
		),
		PrevLine: key.NewBinding(
			key.WithKeys("<"),
			key.WithHelp("<", "scroll up"),
		),
		TopPage: key.NewBinding(
			key.WithKeys("H"),
			key.WithHelp("H", "top of page"),
		),
		MiddlePage: key.NewBinding(
			key.WithKeys("M"),
			key.WithHelp("M", "middle of page"),
		),
		BottomPage: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "bottom of page"),
		),
		First: key.NewBinding(
			key.WithKeys("=", "home"),
			key.WithHelp("=/home", "first"),
		),
		Last: key.NewBinding(
			key.WithKeys("*", "end"),
			key.WithHelp("*/end", "last"),
		),
		Jump: key.NewBinding(
			key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"),
			key.WithHelp("1-9", "jump to QSO"),
		),
		Open: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "show QSO"),
		),
		NewQSO: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "new QSO"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit"),
		),
		Mark: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "mark"),
		),
		MarkPattern: key.NewBinding(
			key.WithKeys("T"),
			key.WithHelp("T", "mark pattern"),
		),
		UnmarkPattern: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "unmark pattern"),
		),
		Limit: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "limit"),
		),
		Sort: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "sort"),
		),
		SortReverse: key.NewBinding(
			key.WithKeys("O"),
			key.WithHelp("O", "reverse sort"),
		),
		Reload: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "reload"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		// NeoMutt's Esc /. The terminal must send Esc and / together, as it
		// does for alt+/; Esc pressed on its own is a key of its own.
		SearchReverse: key.NewBinding(
			key.WithKeys("alt+/"),
			key.WithHelp("alt+/", "search back"),
		),
		SearchNext: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "search next"),
		),
		SearchOpposite: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "search opposite"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		MarkPrefix: key.NewBinding(
			key.WithKeys(";"),
			key.WithHelp(";", "mark-prefix"),
		),
		PrefixCancel: key.NewBinding(
			key.WithKeys("esc"),
		),
		BulkDelete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp(";d", "delete marked"),
		),
		BulkSave: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp(";s", "save marked"),
		),
		BulkTag: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp(";y", "tag marked"),
		),
		BulkUntag: key.NewBinding(
			key.WithKeys("Y"),
			key.WithHelp(";Y", "untag marked"),
		),
	}
	for _, b := range []*key.Binding{
		&k.Next, &k.Prev, &k.NextPage, &k.PrevPage, &k.HalfDown, &k.HalfUp,
		&k.NextLine, &k.PrevLine, &k.TopPage, &k.MiddlePage, &k.BottomPage,
		&k.First, &k.Last, &k.Jump, &k.Open, &k.NewQSO, &k.Edit, &k.Mark, &k.MarkPattern, &k.UnmarkPattern,
		&k.Limit, &k.Sort, &k.SortReverse, &k.Reload, &k.Search, &k.SearchReverse, &k.SearchNext,
		&k.SearchOpposite, &k.Delete, &k.MarkPrefix, &k.PrefixCancel,
		&k.BulkDelete, &k.BulkSave, &k.BulkTag, &k.BulkUntag,
	} {
		b.SetEnabled(enabled)
	}
	return k
}

func (k indexKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Next, k.Prev, k.Open, k.NewQSO, k.Edit, k.Mark, k.Jump}
}

func (k indexKeyMap) FullHelp() [][]key.Binding {
	return sectionKeys(k.sections())
}

func (k indexKeyMap) sections() []helpSection {
	return []helpSection{
		{"Navigation", []key.Binding{
			k.Next, k.Prev, k.NextLine, k.PrevLine, k.NextPage, k.PrevPage, k.HalfDown, k.HalfUp,
			k.TopPage, k.MiddlePage, k.BottomPage, k.First, k.Last, k.Jump,
		}},
		{"QSOs", []key.Binding{k.Open, k.NewQSO, k.Edit, k.Delete}},
		{"View", []key.Binding{k.Limit, k.Sort, k.SortReverse, k.Reload}},
		{"Search", []key.Binding{k.Search, k.SearchReverse, k.SearchNext, k.SearchOpposite}},
		{"Marks", []key.Binding{
			k.Mark, k.MarkPattern, k.UnmarkPattern, k.MarkPrefix,
			k.BulkDelete, k.BulkSave, k.BulkTag, k.BulkUntag,
		}},
	}
}

// joinedKeyMap shows several key maps' help as one, in order.
type joinedKeyMap []helpKeyMap

func (j joinedKeyMap) ShortHelp() []key.Binding {
	var out []key.Binding
	for _, k := range j {
		out = append(out, k.ShortHelp()...)
	}
	return out
}

func (j joinedKeyMap) FullHelp() [][]key.Binding {
	var out [][]key.Binding
	for _, k := range j {
		out = append(out, k.FullHelp()...)
	}
	return out
}

func (j joinedKeyMap) sections() []helpSection {
	var out []helpSection
	for _, k := range j {
		out = append(out, k.sections()...)
	}
	return out
}
