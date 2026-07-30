package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshDoneMsg:
		if msg.err == nil {
			selectedID := ""
			if m.selected >= 0 && m.selected < len(m.pullRequests) {
				selectedID = m.pullRequests[m.selected].ID
			}
			m.pullRequests = slices.Clone(msg.result.Snapshot.PullRequests)
			m.fetchedAt = msg.result.Snapshot.FetchedAt
			for index := range m.pullRequests {
				if m.pullRequests[index].ID == selectedID {
					m.selected = index
					break
				}
			}
			if len(m.pullRequests) == 0 {
				m.selected = 0
			} else if m.selected >= len(m.pullRequests) {
				m.selected = len(m.pullRequests) - 1
			} else if m.selected < 0 {
				m.selected = 0
			}
			m.syncDetailViewport()
			m.loading = false
			m.stale = false
			m.refreshErr = nil
		} else {
			m.loading = false
			m.refreshErr = msg.err
		}
		m.refreshing = false
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case tea.KeyPressMsg:
		switch msg.Key().Code {
		case tea.KeyTab:
			if m.focus == listFocus {
				m.focus = detailFocus
			} else {
				m.focus = listFocus
			}
		case tea.KeyDown:
			if m.focus == detailFocus {
				m.detailViewport.ScrollDown(1)
			} else if m.selected < len(m.pullRequests)-1 {
				m.selected++
				m.syncDetailViewport()
			}
		case tea.KeyUp:
			if m.focus == detailFocus {
				m.detailViewport.ScrollUp(1)
			} else if m.selected > 0 {
				m.selected--
				m.syncDetailViewport()
			}
		}
	}
	return m, nil
}
