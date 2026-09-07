package tui

import "github.com/charmbracelet/bubbles/key"

// BrowseKeyMap is the browse-mode keymap.
type BrowseKeyMap struct {
	Up         key.Binding
	Down       key.Binding
	Top        key.Binding
	Bottom     key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	NextPane   key.Binding
	PrevPane   key.Binding
	Open       key.Binding
	Filter     key.Binding
	ClearFilt  key.Binding
	Sync       key.Binding
	Zoom       key.Binding
	PreviewTab key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func DefaultBrowseKeyMap() BrowseKeyMap {
	return BrowseKeyMap{
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:        key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:     key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		PageUp:     key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
		PageDown:   key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
		NextPane:   key.NewBinding(key.WithKeys("tab", "l", "right"), key.WithHelp("tab", "next pane")),
		PrevPane:   key.NewBinding(key.WithKeys("shift+tab", "h", "left"), key.WithHelp("⇧tab", "prev pane")),
		Open:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "open workspace")),
		Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		ClearFilt:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
		Sync:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		Zoom:       key.NewBinding(key.WithKeys("z", "+"), key.WithHelp("z", "zoom")),
		PreviewTab: key.NewBinding(key.WithKeys("]", "["), key.WithHelp("]", "preview tab")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k BrowseKeyMap) shortcutHints(filtering bool) []hint {
	if filtering {
		return []hint{
			{"↵", "apply"}, {"esc", "cancel"}, {"type", "to filter"},
		}
	}
	return []hint{
		{k.Open.Help().Key, k.Open.Help().Desc},
		{k.Filter.Help().Key, k.Filter.Help().Desc},
		{k.NextPane.Help().Key, k.NextPane.Help().Desc},
		{k.Sync.Help().Key, k.Sync.Help().Desc},
		{k.Zoom.Help().Key, k.Zoom.Help().Desc},
		{k.Help.Help().Key, k.Help.Help().Desc},
		{k.Quit.Help().Key, k.Quit.Help().Desc},
	}
}
