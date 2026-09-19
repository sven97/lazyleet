package tui

import "github.com/charmbracelet/lipgloss"

// Theme holds the styles the workspace UI draws with. One theme for now
// ("default"); PLAN.md Phase 7 adds more and a light-terminal fallback.
type Theme struct {
	PaneBorder        lipgloss.Style
	PaneBorderFocused lipgloss.Style
	Title             lipgloss.Style
	TitleFocused      lipgloss.Style
	StatusBar         lipgloss.Style
	StatusKey         lipgloss.Style
	StatusDivider     lipgloss.Style

	DiffAdd   lipgloss.Style // expected
	DiffDel   lipgloss.Style // actual (wrong)
	Pass      lipgloss.Style
	Fail      lipgloss.Style
	Muted     lipgloss.Style
	Spinner   lipgloss.Style
	ErrorText lipgloss.Style
	// Selection highlights a click-drag text selection (F1) over a prose
	// pane's body. Reverse video, matching the app's existing convention for
	// "this is picked" (the list cursor row, the active source row).
	Selection lipgloss.Style
}

var (
	colFocus   = lipgloss.Color("39")  // bright blue
	colBorder  = lipgloss.Color("240") // grey
	colPass    = lipgloss.Color("42")  // green
	colFail    = lipgloss.Color("203") // red
	colMuted   = lipgloss.Color("245")
	colWarnFg  = lipgloss.Color("214")
	colStatBg  = lipgloss.Color("236")
	colStatKey = lipgloss.Color("39")
)

// DefaultTheme returns the built-in theme.
func DefaultTheme() Theme {
	return Theme{
		PaneBorder:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colBorder),
		PaneBorderFocused: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colFocus),
		Title:             lipgloss.NewStyle().Foreground(colMuted).Bold(true),
		TitleFocused:      lipgloss.NewStyle().Foreground(colFocus).Bold(true),
		StatusBar:         lipgloss.NewStyle().Background(colStatBg).Foreground(lipgloss.Color("252")),
		StatusKey:         lipgloss.NewStyle().Background(colStatBg).Foreground(colStatKey).Bold(true),
		StatusDivider:     lipgloss.NewStyle().Background(colStatBg).Foreground(colBorder),
		DiffAdd:           lipgloss.NewStyle().Foreground(colPass),
		DiffDel:           lipgloss.NewStyle().Foreground(colFail),
		Pass:              lipgloss.NewStyle().Foreground(colPass).Bold(true),
		Fail:              lipgloss.NewStyle().Foreground(colFail).Bold(true),
		Muted:             lipgloss.NewStyle().Foreground(colMuted),
		Spinner:           lipgloss.NewStyle().Foreground(colFocus),
		ErrorText:         lipgloss.NewStyle().Foreground(colWarnFg),
		Selection:         lipgloss.NewStyle().Reverse(true),
	}
}

// DifficultyStyle colors a difficulty badge.
func DifficultyStyle(difficulty string) lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true)
	switch difficulty {
	case "Easy":
		return base.Foreground(colPass)
	case "Medium":
		return base.Foreground(colWarnFg)
	case "Hard":
		return base.Foreground(colFail)
	default:
		return base.Foreground(colMuted)
	}
}
