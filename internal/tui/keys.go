package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// digitKey reports the bare digit 1-9 a keypress represents, if any — used
// by both modes' F2 "jump straight to pane N" dispatch (see the digit block
// near the end of handleKey in both browse_model.go and workspace_model.go).
// Neither KeyMap nor BrowseKeyMap binds a bare digit to anything else, so
// there's no collision to worry about; digit-jump keys are deliberately kept
// out of both maps (and the shortcut bar) since they're documented in the
// `?` help overlay instead, not the always-visible hints.
func digitKey(msg tea.KeyMsg) (int, bool) {
	s := msg.String()
	if len(s) != 1 || s[0] < '1' || s[0] > '9' {
		return 0, false
	}
	return int(s[0] - '0'), true
}

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
	Hints    key.Binding
	History  key.Binding
	Zoom     key.Binding
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding
	// Copy (F1) copies the focused pane's current text selection, or its
	// whole visible content if there is no selection — a keyboard fallback
	// for terminals without OSC 52 support. Deliberately left out of
	// shortcutHints (F3 keeps that bar to 3 items); documented in `?` help.
	Copy key.Binding
	Help key.Binding
	Back key.Binding
	Quit key.Binding
}

// DefaultKeyMap returns the built-in bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		NextPane: key.NewBinding(key.WithKeys("tab", "l", "right"), key.WithHelp("tab", "next pane")),
		PrevPane: key.NewBinding(key.WithKeys("shift+tab", "h", "left"), key.WithHelp("⇧tab", "prev pane")),
		Edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Run:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run local")),
		RunLC:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "run @LC")),
		Submit:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "submit")),
		Import:   key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "import failing case")),
		Hints:    key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "hints")),
		History:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "history")),
		Tests:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tests")),
		Zoom:     key.NewBinding(key.WithKeys("z", "+"), key.WithHelp("z", "zoom")),
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		Copy:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy selection (or whole pane)")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Back:     key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "back")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "back")),
	}
}

// hint is one entry in the shortcut bar.
type hint struct {
	key  string
	desc string
}

// shortcutHints returns the bindings to show in the status bar for the
// currently focused pane: at most 3 context-specific hints (the ones judged
// most-used for that pane, cf. lazygit's context-dependent bottom bar), plus
// a trailing, always-present `?` help hint so full reference is never more
// than one keypress away. The `?` help overlay documents every binding in
// full — this bar deliberately shows only the hottest few.
func (k KeyMap) shortcutHints(focus Pane) []hint {
	var hints []hint
	switch focus {
	case PaneStatement:
		hints = []hint{
			{k.Edit.Help().Key, k.Edit.Help().Desc},
			{k.Hints.Help().Key, k.Hints.Help().Desc},
			{k.Zoom.Help().Key, k.Zoom.Help().Desc},
		}
	case PaneCode:
		hints = []hint{
			{k.Edit.Help().Key, k.Edit.Help().Desc},
			{k.Run.Help().Key, k.Run.Help().Desc},
			{k.RunLC.Help().Key, k.RunLC.Help().Desc},
		}
	case PaneResults:
		hints = []hint{
			{k.Submit.Help().Key, k.Submit.Help().Desc},
			{k.Import.Help().Key, "import"}, // full desc ("import failing case") is in `?` help
			{k.History.Help().Key, k.History.Help().Desc},
		}
	}
	return append(hints, hint{k.Help.Help().Key, k.Help.Help().Desc})
}
