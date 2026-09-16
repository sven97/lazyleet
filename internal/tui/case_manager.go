package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sven97/lazyleet/internal/testcase"
	"github.com/sven97/lazyleet/internal/workspace"
)

type caseManager struct {
	snapshot  workspace.CaseSnapshot
	cursor    int
	editing   bool
	editIndex int
	deleting  bool
	focus     int
	inputs    textarea.Model
	expected  textarea.Model
	message   string
}

func (m *WorkspaceModel) openCaseManager() (tea.Model, tea.Cmd) {
	snapshot, err := m.ws.ReadCaseSnapshot()
	if err != nil {
		m.statusMsg = "cannot open test cases: " + err.Error()
		return m, nil
	}
	m.cases = &caseManager{snapshot: snapshot}
	return m, nil
}

func (m *WorkspaceModel) editCase(index int) tea.Cmd {
	c := m.cases
	c.editing, c.deleting, c.editIndex, c.focus = true, false, index, 0
	c.message = ""
	c.inputs = textarea.New()
	c.expected = textarea.New()
	c.inputs.CharLimit, c.expected.CharLimit = 0, 0
	c.inputs.MaxHeight, c.expected.MaxHeight = 0, 0
	c.inputs.MaxWidth, c.expected.MaxWidth = 0, 0
	c.inputs.ShowLineNumbers, c.expected.ShowLineNumbers = false, false
	c.inputs.Placeholder = "One JSON value per input, in parameter order"
	c.expected.Placeholder = "Optional JSON value (blank = unknown)"
	if index >= 0 {
		selected := c.snapshot.Cases[index]
		c.inputs.SetValue(strings.Join(selected.In, "\n"))
		c.expected.SetValue(selected.Out)
	}
	m.sizeCaseForm()
	return c.inputs.Focus()
}

func (m *WorkspaceModel) sizeCaseForm() {
	if m.cases == nil || !m.cases.editing {
		return
	}
	w := max(1, m.width-4)
	h := max(1, (m.height-10)/2)
	m.cases.inputs.SetWidth(w)
	m.cases.expected.SetWidth(w)
	m.cases.inputs.SetHeight(h)
	m.cases.expected.SetHeight(h)
}

func (m *WorkspaceModel) handleCaseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := m.cases
	k := msg.String()
	if c.deleting {
		switch k {
		case "y", "enter":
			if err := m.ws.EditCase(c.snapshot, c.cursor, nil); err != nil {
				c.message = err.Error()
				c.deleting = false
				return m, nil
			}
			c.deleting = false
			m.reloadCaseList("case deleted · press esc then r to run", true)
		case "n", "esc", "q":
			c.deleting = false
		}
		return m, nil
	}
	if c.editing {
		switch k {
		case "esc":
			c.editing = false
			return m, nil
		case "ctrl+s":
			return m.saveCase()
		case "tab", "shift+tab":
			c.focus = 1 - c.focus
			c.inputs.Blur()
			c.expected.Blur()
			if c.focus == 0 {
				return m, c.inputs.Focus()
			}
			return m, c.expected.Focus()
		}
		var cmd tea.Cmd
		if c.focus == 0 {
			c.inputs, cmd = c.inputs.Update(msg)
		} else {
			c.expected, cmd = c.expected.Update(msg)
		}
		return m, cmd
	}
	switch k {
	case "esc", "q", "t", "b":
		m.cases = nil
		return m, nil
	case "up", "k":
		c.cursor = max(0, c.cursor-1)
	case "down", "j":
		c.cursor = min(max(0, len(c.snapshot.Cases)-1), c.cursor+1)
	case "a":
		return m, m.editCase(-1)
	case "e", "enter":
		if len(c.snapshot.Cases) > 0 {
			return m, m.editCase(c.cursor)
		}
	case "d":
		if len(c.snapshot.Cases) > 0 {
			c.deleting = true
		}
	case "r":
		m.reloadCaseList("reloaded testcases.jsonl", true)
	case "i":
		model, cmd := m.importFailingCase()
		m.reloadCaseList(m.statusMsg, false)
		return model, cmd
	}
	return m, nil
}

func (m *WorkspaceModel) saveCase() (tea.Model, tea.Cmd) {
	c := m.cases
	var in []string
	for _, line := range strings.Split(strings.ReplaceAll(c.inputs.Value(), "\r\n", "\n"), "\n") {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}
		if !json.Valid([]byte(value)) {
			c.message = fmt.Sprintf("input %d must be valid JSON", len(in)+1)
			return m, nil
		}
		in = append(in, value)
	}
	if len(in) == 0 {
		c.message = "enter at least one input"
		return m, nil
	}
	if n := m.q.Meta.Arity(); n > 0 && len(in) != n {
		c.message = fmt.Sprintf("expected %d inputs, got %d", n, len(in))
		return m, nil
	}
	out := strings.TrimSpace(c.expected.Value())
	if out != "" && !json.Valid([]byte(out)) {
		c.message = "expected result must be valid JSON or blank"
		return m, nil
	}
	if err := m.ws.EditCase(c.snapshot, c.editIndex, &testcase.Case{In: in, Out: out}); err != nil {
		c.message = err.Error()
		return m, nil
	}
	c.editing = false
	if c.editIndex < 0 {
		c.cursor = len(c.snapshot.Cases)
	}
	m.reloadCaseList("case saved · press esc then r to run", true)
	return m, nil
}

func (m *WorkspaceModel) reloadCaseList(message string, invalidate bool) {
	snapshot, err := m.ws.ReadCaseSnapshot()
	if err != nil {
		m.cases.message = err.Error()
		return
	}
	m.cases.snapshot = snapshot
	m.cases.cursor = min(m.cases.cursor, max(0, len(snapshot.Cases)-1))
	m.cases.message = message
	if invalidate {
		m.casesRevision++
		m.casesUnrun = true
		m.lastRun = nil
		m.lastErr = nil
		m.refreshResults()
	}
}

func (m *WorkspaceModel) caseManagerView() string {
	c := m.cases
	if m.height < 12 || m.width < 30 {
		return truncate("Enlarge terminal to edit tests; esc returns", m.width)
	}
	var b strings.Builder
	title := fmt.Sprintf("Test cases · %s · %d cases", m.q.Title, len(c.snapshot.Cases))
	b.WriteString(m.th.TitleFocused.Render(truncate(title, m.width)) + "\n")
	if c.editing {
		name := "Add case"
		if c.editIndex >= 0 {
			name = fmt.Sprintf("Edit case %d", c.editIndex+1)
		}
		b.WriteString(name + "\n")
		label := "Inputs (one JSON value per line)"
		if len(m.q.Meta.Params) > 0 {
			var names []string
			for _, p := range m.q.Meta.Params {
				names = append(names, p.Name)
			}
			label += " · " + strings.Join(names, ", ")
		}
		b.WriteString(truncate(label, m.width) + "\n" + c.inputs.View() + "\n")
		b.WriteString("Expected result (optional)\n" + c.expected.View() + "\n")
		b.WriteString(truncate(c.message, m.width) + "\n")
		b.WriteString(truncate("tab switch field · ctrl+s save · esc cancel", m.width))
	} else {
		b.WriteString("\n")
		rows := max(1, m.height-6)
		start := max(0, c.cursor-rows+1)
		if len(c.snapshot.Cases) == 0 {
			b.WriteString("No cases yet. Press a to add one.\n")
		}
		for i := start; i < min(len(c.snapshot.Cases), start+rows); i++ {
			cs := c.snapshot.Cases[i]
			out := cs.Out
			if out == "" {
				out = "unknown"
			}
			prefix := "  "
			if i == c.cursor {
				prefix = "› "
			}
			line := fmt.Sprintf("%s%d. %s → %s", prefix, i+1, strings.Join(cs.In, " | "), out)
			line = strings.ReplaceAll(strings.ReplaceAll(line, "\n", " "), "\r", " ")
			line = truncate(line, m.width)
			if i == c.cursor {
				line = m.th.TitleFocused.Render(line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + truncate(c.message, m.width) + "\n")
		if c.deleting {
			b.WriteString(fmt.Sprintf("Delete case %d? y/enter confirm · esc cancel", c.cursor+1))
		} else {
			b.WriteString("a add · e edit · d delete · i import · r reload · esc back")
		}
	}
	lines := strings.Split(b.String(), "\n")
	for i, line := range lines {
		lines[i] = truncate(line, m.width)
	}
	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(strings.Join(lines, "\n"))
}
