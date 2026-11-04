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
		m.generation++
		m.statusMessage = ""
		if msg.err == nil {
			selectedID := ""
			if m.selected >= 0 && m.selected < len(m.pullRequests) {
				selectedID = m.pullRequests[m.selected].ID
			}
			checked := make(map[string]bool)
			for _, pullRequest := range msg.result.Snapshot.PullRequests {
				if m.checked[pullRequest.ID] {
					checked[pullRequest.ID] = true
				}
			}
			m.checked = checked
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
		if msg.generation != m.generation {
			return m, nil
		}
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
		case "space":
			if pullRequest, ok := m.selectedPullRequest(); ok {
				if m.checked == nil {
					m.checked = make(map[string]bool)
				}
				if m.checked[pullRequest.ID] {
					delete(m.checked, pullRequest.ID)
				} else {
					m.checked[pullRequest.ID] = true
				}
				m.statusMessage = ""
			}
			return m, nil
		case "a":
			m.checked = make(map[string]bool, len(m.pullRequests))
			for _, pullRequest := range m.pullRequests {
				m.checked[pullRequest.ID] = true
			}
			m.statusMessage = ""
			return m, nil
		case "esc":
			m.checked = make(map[string]bool)
			m.statusMessage = ""
			return m, nil
		case "c":
			if len(m.checked) > 0 {
				var checked []app.PullRequest
				for _, pullRequest := range m.pullRequests {
					if m.checked[pullRequest.ID] {
						checked = append(checked, pullRequest)
					}
				}
				success := fmt.Sprintf("Copied %d PRs.", len(checked))
				if len(checked) == 1 {
					success = "Copied 1 PR."
				}
				return m, m.copyCmd(app.FormatSlackList(checked), success)
			}
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
		case tea.KeyDown:
			if m.selected < len(m.pullRequests)-1 {
				m.selected++
			}
		case tea.KeyUp:
			if m.selected > 0 {
				m.selected--
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
				generation: m.generation,
				action:     "Copy",
				err:        fmt.Errorf("clipboard dependency is unavailable"),
			}
		}
		err := m.deps.Copy(m.ctx, text)
		return actionDoneMsg{
			generation: m.generation,
			action:     "Copy",
			success:    success,
			err:        err,
		}
	}
}

func (m Model) openCmd(rawURL string) tea.Cmd {
	return func() tea.Msg {
		if m.deps.Open == nil {
			return actionDoneMsg{
				generation: m.generation,
				action:     "Open",
				err:        fmt.Errorf("browser dependency is unavailable"),
			}
		}
		err := m.deps.Open(m.ctx, rawURL)
		return actionDoneMsg{
			generation: m.generation,
			action:     "Open",
			success:    "Opened pull request.",
			err:        err,
		}
	}
}
