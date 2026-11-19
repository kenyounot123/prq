package tui

import (
	"fmt"
	"maps"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/kenyounot123/prq/internal/app"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	next.scrollTop = next.scrolledTop()
	return next, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshDoneMsg:
		m.generation++
		m.status = statusLine{}
		if msg.err == nil {
			target := m.cursorTarget()
			checked := make(map[string]bool)
			for _, pullRequest := range msg.result.Snapshot.PullRequests {
				if m.checked[pullRequest.ID] {
					checked[pullRequest.ID] = true
				}
			}
			m.checked = checked
			oldSelected := m.selected
			m.pullRequests = slices.Clone(msg.result.Snapshot.PullRequests)
			m.fetchedAt = msg.result.Snapshot.FetchedAt
			rows := m.rows()
			if index, ok := m.locate(rows, target); ok {
				m.selected = index
			} else {
				m.selected = m.nearestNavigable(rows, oldSelected)
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
			m.status = statusLine{text: msg.success, kind: statusSuccess}
		} else {
			m.status = statusLine{text: fmt.Sprintf("%s failed: %v", msg.action, msg.err), kind: statusError}
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
			m.status = statusLine{}
			return m, m.refreshCmd()
		case "C":
			if len(m.pullRequests) == 0 {
				m.status = noPullRequestSelected
				return m, nil
			}
			ordered := m.groupedPullRequests()
			text := app.FormatSlackList(ordered)
			success := fmt.Sprintf("Copied %d pull requests.", len(ordered))
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
				m.status = statusLine{}
			}
			return m, nil
		case "a":
			m.checked = make(map[string]bool, len(m.pullRequests))
			for _, pullRequest := range m.groupedPullRequests() {
				m.checked[pullRequest.ID] = true
			}
			m.status = statusLine{}
			return m, nil
		case "esc":
			m.checked = make(map[string]bool)
			m.status = statusLine{}
			return m, nil
		case "c":
			if len(m.checked) > 0 {
				var checked []app.PullRequest
				for _, pullRequest := range m.groupedPullRequests() {
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
				m.status = noPullRequestSelected
				return m, nil
			}
			return m, m.copyCmd(pullRequest.URL, "Copied pull request URL.")
		case "o":
			pullRequest, ok := m.selectedPullRequest()
			if !ok {
				m.status = noPullRequestSelected
				return m, nil
			}
			return m, m.openCmd(pullRequest.URL)
		}

		switch msg.String() {
		case "down":
			m.selected = m.stepCursor(m.rows(), 1)
		case "up":
			m.selected = m.stepCursor(m.rows(), -1)
		case "tab":
			m.selected = m.nextGroupCursor(m.rows())
		case "enter":
			m = m.toggleGroupAtCursor()
		}
	}
	return m, nil
}

var noPullRequestSelected = statusLine{text: "No pull request selected.", kind: statusWarning}

// toggleGroupAtCursor expands a collapsed group from its header, or collapses
// the group of the highlighted pull request and rests on its header.
func (m Model) toggleGroupAtCursor() Model {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return m
	}
	group := rows[m.selected].group
	m.collapsed = maps.Clone(m.collapsed)
	if m.collapsed == nil {
		m.collapsed = make(map[app.StatusGroup]bool)
	}
	expanding := m.collapsed[group]
	m.collapsed[group] = !expanding
	for index, row := range m.rows() {
		if row.group != group {
			continue
		}
		if expanding && !row.isHeader() {
			m.selected = index
			break
		}
		if !expanding && row.isHeader() {
			m.selected = index
			break
		}
	}
	return m
}

func (m Model) selectedPullRequest() (app.PullRequest, bool) {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) || rows[m.selected].isHeader() {
		return app.PullRequest{}, false
	}
	return m.pullRequests[rows[m.selected].pullRequest], true
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
