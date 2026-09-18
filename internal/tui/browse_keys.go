package tui

import "github.com/charmbracelet/bubbles/key"

// BrowseKeyMap is the browse-mode keymap.
type BrowseKeyMap struct {
	Up              key.Binding
	Down            key.Binding
	Top             key.Binding
	Bottom          key.Binding
	PageUp          key.Binding
	PageDown        key.Binding
	NextPane        key.Binding
	PrevPane        key.Binding
	Open            key.Binding
	Filter          key.Binding
	ClearFilt       key.Binding
	FilterDiff      key.Binding
	FilterStatus    key.Binding
	FilterPaid      key.Binding
	FilterTags      key.Binding
	Sort            key.Binding
	ClearListFilter key.Binding
	Sync            key.Binding
	Zoom            key.Binding
	Help            key.Binding
	Quit            key.Binding
}

func DefaultBrowseKeyMap() BrowseKeyMap {
	return BrowseKeyMap{
		Up:              key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:            key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:             key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:          key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		PageUp:          key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
		PageDown:        key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
		NextPane:        key.NewBinding(key.WithKeys("tab", "l", "right"), key.WithHelp("tab", "next pane")),
		PrevPane:        key.NewBinding(key.WithKeys("shift+tab", "h", "left"), key.WithHelp("⇧tab", "prev pane")),
		Open:            key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "open workspace")),
		Filter:          key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "fuzzy filter")),
		ClearFilt:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear fuzzy filter")),
		FilterDiff:      key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "difficulty filter")),
		FilterStatus:    key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "status filter")),
		FilterTags:      key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "topics")),
		FilterPaid:      key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "toggle paid-only")),
		Sort:            key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "cycle sort")),
		ClearListFilter: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear filters/sort")),
		Sync:            key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		Zoom:            key.NewBinding(key.WithKeys("z", "+"), key.WithHelp("z", "zoom")),
		Help:            key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:            key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// shortcutHints returns the bindings to show in the status bar: at most 3
// hints for the currently focused region (the ones judged most-used there,
// cf. lazygit's context-dependent bottom bar), plus a trailing, always-present
// `?` help hint so full reference is never more than one keypress away. The
// `?` help overlay documents every binding in full — this bar deliberately
// shows only the hottest few.
//
// filtering overrides everything else: while the fuzzy filter textinput is
// focused, only its own bindings are meaningful, so that special-case list
// wins regardless of region (unchanged from before this became region-aware).
func (k BrowseKeyMap) shortcutHints(filtering bool, focus Region) []hint {
	if filtering {
		return []hint{
			{"↵", "apply"}, {"esc", "cancel"}, {"type", "to filter"},
		}
	}
	var hints []hint
	switch focus {
	case RegionStatus:
		hints = []hint{
			{k.Sync.Help().Key, k.Sync.Help().Desc},
			{k.NextPane.Help().Key, k.NextPane.Help().Desc},
			{k.Zoom.Help().Key, k.Zoom.Help().Desc},
		}
	case RegionSources:
		hints = []hint{
			{k.Up.Help().Key + " " + k.Down.Help().Key, "select"},
			{k.Open.Help().Key, k.Open.Help().Desc},
			{k.NextPane.Help().Key, k.NextPane.Help().Desc},
		}
	case RegionList:
		hints = []hint{
			{k.Open.Help().Key, k.Open.Help().Desc},
			{k.Filter.Help().Key, k.Filter.Help().Desc},
			{k.Sort.Help().Key, k.Sort.Help().Desc},
		}
	case RegionDetail:
		hints = []hint{
			{k.Up.Help().Key + " " + k.Down.Help().Key, "scroll"},
			{k.NextPane.Help().Key, k.NextPane.Help().Desc},
			{k.Zoom.Help().Key, k.Zoom.Help().Desc},
		}
	}
	return append(hints, hint{k.Help.Help().Key, k.Help.Help().Desc})
}
