package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap is the workspace keymap. Bindings map to semantic actions; the same
// definitions feed dispatch and the generated shortcut bar (cf. lazygit).
type KeyMap struct {
	NextPane key.Binding
	PrevPane key.Binding
	Edit     key.Binding
	Run      key.Binding
	RunLC    key.Binding
	Submit   key.Binding
	Import   key.Binding
	Tests    key.Binding
	Zoom     key.Binding
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Help     key.Binding
	Back     key.Binding
	Quit     key.Binding
}

// DefaultKeyMap returns the built-in bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		NextPane: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next pane")),
		PrevPane: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧tab", "prev pane")),
		Edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Run:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run local")),
		RunLC:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "run @LC")),
		Submit:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "submit")),
		Import:   key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "import failing case")),
		Tests:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tests")),
		Zoom:     key.NewBinding(key.WithKeys("z", "+"), key.WithHelp("z", "zoom")),
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Back:     key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "back")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// hint is one entry in the shortcut bar.
type hint struct {
	key  string
	desc string
}

// shortcutHints returns the bindings to show in the status bar for the current
// context, in order. RunLC/Submit are shown but marked pending (Phase 6).
func (k KeyMap) shortcutHints() []hint {
	return []hint{
		{k.Edit.Help().Key, k.Edit.Help().Desc},
		{k.Run.Help().Key, k.Run.Help().Desc},
		{k.RunLC.Help().Key, k.RunLC.Help().Desc},
		{k.Submit.Help().Key, k.Submit.Help().Desc},
		{k.Import.Help().Key, "import"}, // full desc ("import failing case") is in `?` help
		{k.NextPane.Help().Key, k.NextPane.Help().Desc},
		{k.Zoom.Help().Key, k.Zoom.Help().Desc},
		{k.Help.Help().Key, k.Help.Help().Desc},
		{k.Back.Help().Key, k.Back.Help().Desc},
		{k.Quit.Help().Key, k.Quit.Help().Desc},
	}
}
