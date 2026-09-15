package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sven97/lazyleet/internal/termimg"
)

// OpenProblemMsg is emitted by BrowseModel when the user opens a problem and
// the browse UI is embedded in an AppModel (so we must not tea.Quit).
type OpenProblemMsg struct{ Slug string }

// BackToBrowseMsg is emitted by WorkspaceModel when the user presses back/quit
// and the workspace should return to browse instead of exiting the process.
type BackToBrowseMsg struct{}

// WorkspaceFactory scaffolds and builds a WorkspaceModel for a problem slug.
type WorkspaceFactory func(slug string) (*WorkspaceModel, error)

// AppModel is the top-level Bubble Tea model that switches between browse and
// workspace while preserving browse filters and cursor.
type AppModel struct {
	browse    *BrowseModel
	workspace *WorkspaceModel
	factory   WorkspaceFactory

	width, height int

	imgProto  termimg.Protocol
	imgDir    string
	imgWriter *ImageWriter

	statusErr string // factory / open failure, shown via browse status
}

// NewAppModel wraps browse with a factory used when the user opens a problem.
func NewAppModel(browse *BrowseModel, factory WorkspaceFactory) *AppModel {
	browse.embedded = true
	return &AppModel{browse: browse, factory: factory}
}

// EnableImages configures inline statement images for both modes.
func (m *AppModel) EnableImages(proto termimg.Protocol, cacheDir string, iw *ImageWriter) {
	m.imgProto = proto
	m.imgDir = cacheDir
	m.imgWriter = iw
	m.browse.EnableImages(proto, cacheDir, iw)
}

func (m *AppModel) Init() tea.Cmd {
	return m.browse.Init()
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case OpenProblemMsg:
		return m.openWorkspace(msg.Slug)
	case BackToBrowseMsg:
		return m.closeWorkspace()
	}

	if m.workspace != nil {
		updated, cmd := m.workspace.Update(msg)
		if ws, ok := updated.(*WorkspaceModel); ok {
			m.workspace = ws
		}
		return m, cmd
	}

	updated, cmd := m.browse.Update(msg)
	if bm, ok := updated.(*BrowseModel); ok {
		m.browse = bm
		// Non-embedded open still uses Chosen+Quit; ignore if embedded.
		if !bm.embedded && bm.Chosen != "" {
			return m, tea.Quit
		}
	}
	return m, cmd
}

func (m *AppModel) View() string {
	if m.workspace != nil {
		return m.workspace.View()
	}
	return m.browse.View()
}

func (m *AppModel) openWorkspace(slug string) (tea.Model, tea.Cmd) {
	if m.factory == nil {
		m.browse.statusMsg = "cannot open workspace: no factory configured"
		return m, nil
	}
	ws, err := m.factory(slug)
	if err != nil {
		m.browse.statusMsg = fmt.Sprintf("workspace: %v", err)
		m.statusErr = err.Error()
		return m, nil
	}
	ws.returnToBrowse = true
	if m.imgWriter != nil {
		ws.EnableImages(m.imgProto, m.imgDir, m.imgWriter)
	}
	if m.width > 0 {
		ws.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	}
	m.workspace = ws
	m.browse.Chosen = ""
	return m, ws.Init()
}

func (m *AppModel) closeWorkspace() (tea.Model, tea.Cmd) {
	if m.workspace != nil {
		m.workspace.Close()
		m.workspace = nil
	}
	var cmd tea.Cmd
	if m.width > 0 {
		_, cmd = m.browse.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	}
	return m, cmd
}

// InWorkspace reports whether the workspace child is active (tests / debug).
func (m *AppModel) InWorkspace() bool { return m.workspace != nil }

// Browse returns the underlying browse model (tests).
func (m *AppModel) Browse() *BrowseModel { return m.browse }

// wheelAtEdge delegates to the active child so WheelEdgeFilter keeps working.
func (m *AppModel) wheelAtEdge(msg tea.MouseMsg) bool {
	if m.workspace != nil {
		return m.workspace.wheelAtEdge(msg)
	}
	if m.browse != nil {
		return m.browse.wheelAtEdge(msg)
	}
	return true
}
