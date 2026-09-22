package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Updater is browse's hook into self-update (cmd/lazyleet wires it to
// internal/selfupdate). A nil Updater disables the feature entirely.
type Updater interface {
	// Check reports the newest release. It must be cheap to call on every
	// launch (cached) and return a zero UpdateInfo when disabled.
	Check(ctx context.Context) (UpdateInfo, error)
	// Apply installs the release the last Check found.
	Apply(ctx context.Context) error
}

// UpdateInfo describes an available update. Latest is "" when the running
// version is current.
type UpdateInfo struct {
	Latest   string // e.g. "v0.4.0"
	Method   string // how lazyleet was installed, e.g. "Homebrew"
	CanApply bool   // false: show Manual instead of offering one-key update
	Manual   string // command to run by hand (or why auto-update is off)
}

type updateCheckedMsg struct {
	info UpdateInfo
	err  error
}

type updateAppliedMsg struct{ err error }

// updateCheckTimeout bounds the launch-time check; it runs in the background
// and fails silently, so it can never slow startup or clutter the UI.
const updateCheckTimeout = 10 * time.Second

// updateApplyTimeout bounds brew/go install/download, which can be slow.
const updateApplyTimeout = 10 * time.Minute

// SetUpdater enables the launch-time update check and the U-to-update flow.
func (m *BrowseModel) SetUpdater(u Updater) { m.updater = u }

// RestartRequested reports whether the program quit so the freshly
// installed version can be started in its place.
func (m *BrowseModel) RestartRequested() bool { return m.restartAfterQuit }

func (m *BrowseModel) checkUpdate() tea.Cmd {
	if m.updater == nil {
		return nil
	}
	u := m.updater
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		info, err := u.Check(ctx)
		return updateCheckedMsg{info: info, err: err}
	}
}

func (m *BrowseModel) handleUpdateChecked(msg updateCheckedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || msg.info.Latest == "" {
		return m, nil // offline, rate-limited, or current: nothing to show
	}
	info := msg.info
	m.update = &info
	return m, m.refreshDetail()
}

// requestUpdate is U / a click on the Status pane's update line. The first
// press asks for confirmation (it runs brew/go and restarts the app, not
// something to trigger by a stray click); the second starts the update.
func (m *BrowseModel) requestUpdate() (tea.Model, tea.Cmd) {
	switch {
	case m.update == nil || m.updating:
		return m, nil
	case !m.update.CanApply:
		m.statusMsg = m.update.Latest + " available — update with: " + m.update.Manual
		return m, nil
	case !m.updateConfirm:
		m.updateConfirm = true
		m.statusMsg = "update to " + m.update.Latest + " via " + m.update.Method +
			" and restart? click or press U again (esc cancels)"
		return m, nil
	}
	m.updateConfirm = false
	m.updating = true
	m.statusMsg = ""
	u := m.updater
	return m, tea.Batch(m.spin.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateApplyTimeout)
		defer cancel()
		return updateAppliedMsg{err: u.Apply(ctx)}
	})
}

func (m *BrowseModel) cancelUpdateConfirm() {
	if m.updateConfirm {
		m.updateConfirm = false
		m.statusMsg = ""
	}
}

func (m *BrowseModel) handleUpdateApplied(msg updateAppliedMsg) (tea.Model, tea.Cmd) {
	m.updating = false
	if msg.err != nil {
		m.statusMsg = "update failed: " + firstLine(msg.err.Error()) + " — try: " + m.update.Manual
		return m, nil
	}
	m.savePosition()
	m.restartAfterQuit = true
	return m, tea.Quit
}

// updateLineY is the terminal row of the Status pane's update line (its
// fourth body row, below auth · cache · daily).
func (m *BrowseModel) updateLineY() int { return m.layout.Status.Y + 1 + 3 }

// statusUpdateLine is the Status pane's fourth row: empty unless a newer
// release exists.
func (m *BrowseModel) statusUpdateLine(w int) string {
	switch {
	case m.update == nil:
		return ""
	case m.updating:
		return m.th.Spinner.Render(m.spin.View()) + " " + truncate("updating to "+m.update.Latest+"…", w-2)
	case m.updateConfirm:
		return m.th.StatusKey.Render(truncate("↑ click or press U to confirm", w))
	case !m.update.CanApply:
		return m.th.Muted.Render(truncate("↑ "+m.update.Latest+" available", w))
	default:
		return m.th.Pass.Render(truncate("↑ "+m.update.Latest+" available · U update", w))
	}
}

// updateDetailLine is the About panel's line under the version.
func (m *BrowseModel) updateDetailLine(w int) string {
	if m.update == nil {
		return ""
	}
	txt := "update: " + m.update.Latest + " available (" + m.update.Method + ")"
	if m.update.CanApply {
		return m.th.Pass.Render(truncate(txt+" — press U", w)) + "\n"
	}
	return m.th.Pass.Render(truncate(txt, w)) + "\n" +
		m.th.Muted.Render(truncate("  "+m.update.Manual, w)) + "\n"
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}
