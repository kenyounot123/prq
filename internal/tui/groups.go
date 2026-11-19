package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/kenyounot123/prq/internal/app"
)

var (
	accentColor    = lipgloss.Color("#A48BFF")
	greenColor     = lipgloss.Color("#7FD88F")
	amberColor     = lipgloss.Color("#F2B45A")
	errorColor     = lipgloss.Color("#F38BA8")
	mutedColor     = lipgloss.Color("#6B7086")
	secondaryColor = lipgloss.Color("#8A8FA3")
	textColor      = lipgloss.Color("#E6E6EA")
	bodyColor      = lipgloss.Color("#B9BDCC")
	ruleColor      = lipgloss.Color("#33374A")
	titleColor     = lipgloss.Color("#FFFFFF")
	onAccentColor  = lipgloss.Color("#1E2029")
	trayDimColor   = lipgloss.Color("#2B2250")
)

type groupPresentation struct {
	glyph      string
	label      string
	shortLabel string
	color      color.Color
}

var groupPresentations = [...]groupPresentation{
	app.StatusBlocked: {glyph: "✗", label: "BLOCKED", shortLabel: "blocked", color: errorColor},
	app.StatusReady:   {glyph: "●", label: "READY TO MERGE", shortLabel: "ready", color: greenColor},
	app.StatusWaiting: {glyph: "○", label: "WAITING ON REVIEW", shortLabel: "review", color: amberColor},
	app.StatusDraft:   {glyph: "◇", label: "DRAFT", shortLabel: "draft", color: secondaryColor},
}

var summaryOrder = [...]app.StatusGroup{
	app.StatusReady, app.StatusWaiting, app.StatusBlocked, app.StatusDraft,
}

func presentationOf(group app.StatusGroup) groupPresentation {
	return groupPresentations[group]
}

const headerRow = -1

type listRow struct {
	group            app.StatusGroup
	pullRequestIndex int
}

func (r listRow) isHeader() bool { return r.pullRequestIndex == headerRow }

func (m Model) groupIndexes() [][]int {
	groups := make([][]int, len(groupPresentations))
	for index, pullRequest := range m.pullRequests {
		group := app.StatusGroupOf(pullRequest)
		groups[group] = append(groups[group], index)
	}
	return groups
}

func (m Model) rows() []listRow {
	groups := m.groupIndexes()
	var rows []listRow
	for _, group := range app.StatusGroups() {
		if len(groups[group]) == 0 {
			continue
		}
		rows = append(rows, listRow{group: group, pullRequestIndex: headerRow})
		if m.collapsed[group] {
			continue
		}
		for _, index := range groups[group] {
			rows = append(rows, listRow{group: group, pullRequestIndex: index})
		}
	}
	return rows
}

func (m Model) groupedPullRequests() []app.PullRequest {
	groups := m.groupIndexes()
	ordered := make([]app.PullRequest, 0, len(m.pullRequests))
	for _, group := range app.StatusGroups() {
		for _, index := range groups[group] {
			ordered = append(ordered, m.pullRequests[index])
		}
	}
	return ordered
}

func (m Model) isNavigable(row listRow) bool {
	return !row.isHeader() || m.collapsed[row.group]
}

func (m Model) firstNavigable(rows []listRow) int {
	for index, row := range rows {
		if m.isNavigable(row) {
			return index
		}
	}
	return 0
}

func (m Model) stepCursor(rows []listRow, delta int) int {
	for index := m.selected + delta; index >= 0 && index < len(rows); index += delta {
		if m.isNavigable(rows[index]) {
			return index
		}
	}
	return m.selected
}

func (m Model) nextGroupCursor(rows []listRow) int {
	if m.selected < 0 || m.selected >= len(rows) {
		return m.firstNavigable(rows)
	}
	current := rows[m.selected].group
	for index := m.selected + 1; index < len(rows); index++ {
		if rows[index].group != current && m.isNavigable(rows[index]) {
			return index
		}
	}
	return m.firstNavigable(rows)
}

func (m Model) nearestNavigable(rows []listRow, index int) int {
	if len(rows) == 0 {
		return 0
	}
	index = min(max(index, 0), len(rows)-1)
	for candidate := index; candidate >= 0; candidate-- {
		if m.isNavigable(rows[candidate]) {
			return candidate
		}
	}
	for candidate := index + 1; candidate < len(rows); candidate++ {
		if m.isNavigable(rows[candidate]) {
			return candidate
		}
	}
	return 0
}

type cursorTarget struct {
	pullRequestID string
	group         app.StatusGroup
	onHeader      bool
	valid         bool
}

func (m Model) cursorTarget() cursorTarget {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return cursorTarget{}
	}
	row := rows[m.selected]
	if row.isHeader() {
		return cursorTarget{group: row.group, onHeader: true, valid: true}
	}
	return cursorTarget{pullRequestID: m.pullRequests[row.pullRequestIndex].ID, valid: true}
}

func (m Model) locate(rows []listRow, target cursorTarget) (int, bool) {
	if !target.valid {
		return 0, false
	}
	for index, row := range rows {
		switch {
		case target.onHeader && row.isHeader() && row.group == target.group:
			return index, true
		case !target.onHeader && !row.isHeader() && m.pullRequests[row.pullRequestIndex].ID == target.pullRequestID:
			return index, true
		}
	}
	if target.onHeader {
		return 0, false
	}
	for _, pullRequest := range m.pullRequests {
		if pullRequest.ID == target.pullRequestID {
			return headerRowOf(rows, app.StatusGroupOf(pullRequest))
		}
	}
	return 0, false
}

func headerRowOf(rows []listRow, group app.StatusGroup) (int, bool) {
	for index, row := range rows {
		if row.isHeader() && row.group == group {
			return index, true
		}
	}
	return 0, false
}
