package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type fakeUpdater struct {
	info     UpdateInfo
	checkErr error
	applyErr error
	applied  int
}

func (f *fakeUpdater) Check(context.Context) (UpdateInfo, error) { return f.info, f.checkErr }
func (f *fakeUpdater) Apply(context.Context) error               { f.applied++; return f.applyErr }

func keyU() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}} }

// bootWithUpdate boots browse with an updater and delivers its check result.
func bootWithUpdate(t *testing.T, u *fakeUpdater) *BrowseModel {
	t.Helper()
	m, _ := bootBrowse(t)
	m.SetUpdater(u)
	drain(&m, m.checkUpdate())
	return m
}

func availableUpdate() *fakeUpdater {
	return &fakeUpdater{info: UpdateInfo{Latest: "v0.4.0", Method: "Homebrew", CanApply: true, Manual: "brew upgrade lazyleet"}}
}

func TestUpdateLineHiddenWhenCurrentOrCheckFails(t *testing.T) {
	for name, u := range map[string]*fakeUpdater{
		"up to date":   {},
		"check failed": {info: UpdateInfo{Latest: "v9.9.9"}, checkErr: errors.New("offline")},
	} {
		m := bootWithUpdate(t, u)
		if m.update != nil || strings.Contains(m.View(), "available") {
			t.Errorf("%s: no update should be shown\n%s", name, m.View())
		}
		step(&m, keyU())
		if u.applied != 0 || m.updateConfirm {
			t.Errorf("%s: U must be a no-op without an update", name)
		}
	}
}

func TestUpdateShownInStatusPane(t *testing.T) {
	m := bootWithUpdate(t, availableUpdate())
	if !strings.Contains(m.View(), "v0.4.0 available") {
		t.Fatalf("status pane should advertise the update\n%s", m.View())
	}
}

func TestUpdateNeedsConfirmationThenAppliesAndRestarts(t *testing.T) {
	u := availableUpdate()
	m := bootWithUpdate(t, u)

	step(&m, keyU())
	if !m.updateConfirm || u.applied != 0 {
		t.Fatalf("first U should only ask to confirm (confirm=%v applied=%d)", m.updateConfirm, u.applied)
	}
	if !strings.Contains(m.statusMsg, "press U again") {
		t.Errorf("statusMsg = %q, want a confirmation prompt", m.statusMsg)
	}

	updated, cmd := m.Update(keyU())
	m = updated.(*BrowseModel)
	if !m.updating {
		t.Fatal("second U should start the update")
	}
	// Run the batch by hand: the apply result must end in tea.Quit.
	var quit bool
	for _, c := range cmd().(tea.BatchMsg) {
		msg := c()
		if res, ok := msg.(updateAppliedMsg); ok {
			_, next := m.Update(res)
			_, quit = next().(tea.QuitMsg)
		}
	}
	if u.applied != 1 {
		t.Fatalf("Apply calls = %d, want 1", u.applied)
	}
	if !quit || !m.RestartRequested() {
		t.Fatalf("a successful update should quit for restart (quit=%v restart=%v)", quit, m.RestartRequested())
	}
}

func TestUpdateConfirmCancelledByEsc(t *testing.T) {
	u := availableUpdate()
	m := bootWithUpdate(t, u)
	step(&m, keyU())
	step(&m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.updateConfirm || m.statusMsg != "" {
		t.Fatalf("esc should cancel the pending update (confirm=%v msg=%q)", m.updateConfirm, m.statusMsg)
	}
	step(&m, keyU())
	if u.applied != 0 || !m.updateConfirm {
		t.Fatal("U after a cancel should ask again, not apply")
	}
}

func TestUpdateFailureIsReportedWithManualCommand(t *testing.T) {
	u := availableUpdate()
	u.applyErr = errors.New("brew upgrade sven97/tap/lazyleet: exit status 1:\nError: permission denied")
	m := bootWithUpdate(t, u)
	step(&m, keyU())
	updated, cmd := m.Update(keyU())
	m = updated.(*BrowseModel)
	drain(&m, cmd)
	if m.updating || m.RestartRequested() {
		t.Fatal("a failed update must not restart")
	}
	if !strings.Contains(m.statusMsg, "update failed") || !strings.Contains(m.statusMsg, "brew upgrade lazyleet") {
		t.Errorf("statusMsg = %q", m.statusMsg)
	}
	if strings.Contains(m.statusMsg, "\n") {
		t.Errorf("status bar message must be one line: %q", m.statusMsg)
	}
}

func TestUpdateNotAutoApplicableShowsManualCommand(t *testing.T) {
	u := &fakeUpdater{info: UpdateInfo{Latest: "v0.4.0", Method: "go install", Manual: "go install example@latest"}}
	m := bootWithUpdate(t, u)
	step(&m, keyU())
	step(&m, keyU())
	if u.applied != 0 {
		t.Fatal("must not apply when CanApply is false")
	}
	if !strings.Contains(m.statusMsg, "go install example@latest") {
		t.Errorf("statusMsg = %q, want the manual command", m.statusMsg)
	}
}

func TestClickUpdateLineConfirmsThenApplies(t *testing.T) {
	u := availableUpdate()
	m := bootWithUpdate(t, u)
	x, y := m.layout.Status.X+2, m.updateLineY()
	if !strings.Contains(ansi.Strip(strings.Split(m.View(), "\n")[y]), "available") {
		t.Fatalf("row %d is not the update line:\n%s", y, m.View())
	}
	step(&m, press(x, y))
	if !m.updateConfirm {
		t.Fatal("first click should ask to confirm")
	}
	updated, _ := m.Update(press(x, y))
	m = updated.(*BrowseModel)
	if !m.updating {
		t.Fatal("second click should start the update")
	}
}

func TestUpdateMessagesReachBrowseWhileWorkspaceOpen(t *testing.T) {
	if !isBrowseBackgroundMsg(updateCheckedMsg{}) || !isBrowseBackgroundMsg(updateAppliedMsg{}) {
		t.Fatal("update results must route to browse even with a workspace open")
	}
}
