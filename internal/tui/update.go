package tui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/kenyounot123/prq/internal/app"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshDoneMsg:
		m.statusMessage = ""
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
			m.cacheWarning = msg.result.CacheWarning
		} else {
			m.loading = false
			m.refreshErr = msg.err
			m.stale = !m.fetchedAt.IsZero()
		}
		m.refreshing = false
	case actionDoneMsg:
		if msg.err == nil {
			m.statusMessage = msg.success
		} else {
			m.statusMessage = fmt.Sprintf("%s failed: %v", msg.action, msg.err)
		}
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "r":
			if m.refreshing {
				return m, nil
			}
			m.refreshing = true
			m.statusMessage = ""
			return m, m.refreshCmd()
		case "C":
			if len(m.pullRequests) == 0 {
				m.statusMessage = "No pull request selected."
				return m, nil
			}
			text := app.FormatSlackList(m.pullRequests)
			success := fmt.Sprintf("Copied %d pull requests.", len(m.pullRequests))
			return m, m.copyCmd(text, success)
		case "c":
			pullRequest, ok := m.selectedPullRequest()
			if !ok {
				m.statusMessage = "No pull request selected."
				return m, nil
			}
			return m, m.copyCmd(pullRequest.URL, "Copied pull request URL.")
		case "o":
			pullRequest, ok := m.selectedPullRequest()
			if !ok {
				m.statusMessage = "No pull request selected."
				return m, nil
			}
			return m, m.openCmd(pullRequest.URL)
		}

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

func (m Model) selectedPullRequest() (app.PullRequest, bool) {
	if m.selected < 0 || m.selected >= len(m.pullRequests) {
		return app.PullRequest{}, false
	}
	return m.pullRequests[m.selected], true
}

func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		if m.deps.Refresh == nil {
			return refreshDoneMsg{err: fmt.Errorf("refresh dependency is unavailable")}
		}
		result, err := m.deps.Refresh(m.ctx, m.selection)
		return refreshDoneMsg{result: result, err: err}
	}
}

func (m Model) copyCmd(text, success string) tea.Cmd {
	return func() tea.Msg {
		if m.deps.Copy == nil {
			return actionDoneMsg{
				action: "Copy",
				err:    fmt.Errorf("clipboard dependency is unavailable"),
			}
		}
		err := m.deps.Copy(m.ctx, text)
		return actionDoneMsg{action: "Copy", success: success, err: err}
	}
}

func (m Model) openCmd(rawURL string) tea.Cmd {
	return func() tea.Msg {
		if m.deps.Open == nil {
			return actionDoneMsg{
				action: "Open",
				err:    fmt.Errorf("browser dependency is unavailable"),
			}
		}
		err := m.deps.Open(m.ctx, rawURL)
		return actionDoneMsg{action: "Open", success: "Opened pull request.", err: err}
	}
}
