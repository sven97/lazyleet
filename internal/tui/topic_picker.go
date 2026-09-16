package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type topicChoice struct {
	tag   string
	count int
}
type topicPicker struct {
	search   textinput.Model
	choices  []topicChoice
	selected map[string]bool
	cursor   int
}

func (m *BrowseModel) openTopicPicker() (tea.Model, tea.Cmd) {
	search := textinput.New()
	search.Prompt = "Search topics: "
	search.Placeholder = "type a topic name"
	search.CharLimit = 100
	search.Width = max(1, m.width-18)
	p := &topicPicker{search: search, selected: map[string]bool{}}
	for _, tag := range m.fltTags {
		p.selected[tag] = true
	}
	m.topicPicker = p
	m.refreshTopicChoices()
	return m, p.search.Focus()
}

func (m *BrowseModel) refreshTopicChoices() {
	p := m.topicPicker
	if p == nil {
		return
	}
	counts := map[string]int{}
	for _, r := range m.allRows {
		seen := map[string]bool{}
		for _, tag := range r.Tags {
			if tag != "" && !seen[tag] {
				counts[tag]++
				seen[tag] = true
			}
		}
	}
	// Keep applied tags removable even if a later sync removes them.
	for tag, selected := range p.selected {
		if selected {
			if _, ok := counts[tag]; !ok {
				counts[tag] = 0
			}
		}
	}
	p.choices = nil
	for tag, count := range counts {
		p.choices = append(p.choices, topicChoice{tag, count})
	}
	sort.Slice(p.choices, func(i, j int) bool { return p.choices[i].tag < p.choices[j].tag })
	p.cursor = min(p.cursor, max(0, len(p.matches())-1))
}

func (p *topicPicker) matches() []topicChoice {
	q := strings.ToLower(strings.TrimSpace(p.search.Value()))
	var out []topicChoice
	for _, choice := range p.choices {
		if strings.Contains(strings.ToLower(choice.tag), q) {
			out = append(out, choice)
		}
	}
	return out
}

func (p *topicPicker) tags() []string {
	var tags []string
	for tag, selected := range p.selected {
		if selected {
			tags = append(tags, tag)
		}
	}
	sort.Strings(tags)
	return tags
}

func (m *BrowseModel) handleTopicKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.topicPicker
	switch msg.String() {
	case "esc":
		m.topicPicker = nil
		return m, nil
	case "ctrl+c":
		m.savePosition()
		return m, tea.Quit
	case "enter":
		m.fltTags = p.tags()
		m.topicPicker = nil
		return m.afterListChange()
	case "up", "ctrl+p":
		p.cursor = max(0, p.cursor-1)
		return m, nil
	case "down", "ctrl+n":
		matches := p.matches()
		p.cursor = min(max(0, len(matches)-1), p.cursor+1)
		return m, nil
	case "pgup":
		p.cursor = max(0, p.cursor-max(1, m.height-7))
		return m, nil
	case "pgdown":
		matches := p.matches()
		p.cursor = min(max(0, len(matches)-1), p.cursor+max(1, m.height-7))
		return m, nil
	case " ":
		matches := p.matches()
		if len(matches) > 0 {
			tag := matches[p.cursor].tag
			p.selected[tag] = !p.selected[tag]
		}
		return m, nil
	case "ctrl+r":
		p.selected = map[string]bool{}
		return m, nil
	}
	before := p.search.Value()
	var cmd tea.Cmd
	p.search, cmd = p.search.Update(msg)
	if before != p.search.Value() {
		p.cursor = 0
	}
	return m, cmd
}

func (m *BrowseModel) topicPickerView() string {
	p := m.topicPicker
	matches := p.matches()
	var b strings.Builder
	b.WriteString(m.th.TitleFocused.Render("Topic filters · match ALL selected topics") + "\n")
	b.WriteString(p.search.View() + "\n")
	tags := p.tags()
	selected := "none"
	if len(tags) > 0 {
		selected = strings.Join(tags, ", ")
	}
	b.WriteString(truncate("Selected: "+selected, m.width) + "\n\n")
	rows := max(1, m.height-7)
	start := max(0, p.cursor-rows+1)
	if len(matches) == 0 {
		if len(p.choices) == 0 {
			b.WriteString("No cached topics. Close and press s to sync.\n")
		} else {
			b.WriteString("No matching topics. Try another search.\n")
		}
	}
	for i := start; i < min(len(matches), start+rows); i++ {
		c := matches[i]
		mark := "[ ]"
		if p.selected[c.tag] {
			mark = "[x]"
		}
		prefix := "  "
		if i == p.cursor {
			prefix = "› "
		}
		line := truncate(fmt.Sprintf("%s%s %s (%d)", prefix, mark, c.tag, c.count), m.width)
		if i == p.cursor {
			line = m.th.TitleFocused.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n↑/↓ move · space toggle · enter apply · esc cancel\nctrl+r clear selection · counts: cached problems")
	lines := strings.Split(b.String(), "\n")
	for i, line := range lines {
		lines[i] = truncate(line, m.width)
	}
	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(strings.Join(lines, "\n"))
}
