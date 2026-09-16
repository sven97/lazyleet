package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestHintsRevealOnlyOnRequest(t *testing.T) {
	m := newTestModel(t)
	m.q.HintsFetched = true
	m.q.Hints = []string{"FIRST_SECRET", "SECOND_SECRET"}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	if strings.Contains(ansi.Strip(m.View()), "SECRET") {
		t.Fatal("hints leaked into workspace")
	}
	pressRune(&m, 'h')
	if !m.showHints || strings.Contains(ansi.Strip(m.View()), "SECRET") {
		t.Fatal("opening hints revealed spoilers")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(ansi.Strip(m.View()), "FIRST_SECRET") || strings.Contains(ansi.Strip(m.hints.View()), "SECOND_SECRET") {
		t.Fatalf("first reveal mismatch: revealed=%d view=%q", m.hintsRevealed, ansi.Strip(m.View()))
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.hintsRevealed != 1 || strings.Contains(ansi.Strip(m.View()), "SECOND_SECRET") {
		t.Fatal("resize revealed a hint")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.showHints || strings.Contains(ansi.Strip(m.View()), "SECRET") {
		t.Fatal("closing hints leaked content")
	}
	pressRune(&m, 'h')
	pressRune(&m, 'n')
	if !strings.Contains(ansi.Strip(m.View()), "SECOND_SECRET") || m.hintsRevealed != 2 {
		t.Fatal("second hint missing")
	}
	pressRune(&m, 'n')
	if m.hintsRevealed != 2 {
		t.Fatal("reveal exceeded hint count")
	}
	pressRune(&m, 'H')
	if m.hintsRevealed != 0 || strings.Contains(ansi.Strip(m.View()), "SECRET") {
		t.Fatal("hide all failed")
	}
}

func TestHintsNotYetCachedVsGenuinelyEmpty(t *testing.T) {
	// HintsFetched defaults to false on a plain fixture: we don't yet know
	// whether this problem has hints, so the message must point at --refresh
	// rather than claim there are none.
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	pressRune(&m, 'h')
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	notCachedView := ansi.Strip(m.hints.View())
	if m.hintsRevealed != 0 || !strings.Contains(notCachedView, "--refresh") {
		t.Fatalf("stale/pre-hints cache not explained: %q", notCachedView)
	}
	if strings.Contains(notCachedView, "no hints") {
		t.Fatalf("not-yet-cached case wrongly claimed the problem has no hints: %q", notCachedView)
	}

	// Once hints have actually been fetched and there genuinely are none, the
	// message must be different: refreshing again would never help.
	m.q.HintsFetched = true
	m.q.Hints = nil
	m.refreshHints()
	emptyView := ansi.Strip(m.hints.View())
	if !strings.Contains(emptyView, "no hints") {
		t.Fatalf("genuinely-empty hints not explained: %q", emptyView)
	}
	if strings.Contains(emptyView, "--refresh") {
		t.Fatalf("genuinely-empty case wrongly suggested --refresh: %q", emptyView)
	}
}

func TestHintsEmptyLongAndSmallViews(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	pressRune(&m, 'h')
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.hintsRevealed != 0 || !strings.Contains(ansi.Strip(m.hints.View()), "--refresh") {
		t.Fatal("empty hints not explained")
	}
	m.q.HintsFetched = true
	m.q.Hints = []string{strings.Repeat("A useful hint.\n\n", 100), "HIDDEN_SECRET"}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.hints.YOffset == 0 {
		t.Fatal("long hint does not scroll")
	}
	for _, size := range [][2]int{{60, 16}, {35, 12}, {120, 35}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(m.View())
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("hints overflow %v", size)
		}
		if strings.Contains(view, "HIDDEN_SECRET") {
			t.Fatal("hidden hint leaked on resize")
		}
	}
}
