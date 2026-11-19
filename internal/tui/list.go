package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kenyounot123/prq/internal/app"
)

const (
	maxLabelWidth  = 18
	labelGap       = 2
	rightColumnMin = 9
)

const blankLine = -1

func (m Model) listLines(rows []listRow) []int {
	lines := make([]int, 0, len(rows)+len(groupPresentations))
	for index, row := range rows {
		if row.isHeader() && index > 0 {
			lines = append(lines, blankLine)
		}
		lines = append(lines, index)
	}
	return lines
}

func (m Model) listHeight() int {
	return layoutBody(m.contentWidth(), m.bodyHeight()).listHeight
}

func (m Model) scrolledTop() int {
	rows := m.rows()
	lines := m.listLines(rows)
	height := m.listHeight()
	if len(rows) == 0 || height <= 0 {
		return 0
	}
	bottom := slicesIndex(lines, m.selected)
	if bottom < 0 {
		return 0
	}
	top := bottom
	if !rows[m.selected].isHeader() && m.selected > 0 && rows[m.selected-1].isHeader() {
		top = bottom - 1
	}
	offset := m.scrollTop
	if top < offset {
		offset = top
	}
	if bottom >= offset+height {
		offset = bottom - height + 1
	}
	return min(max(offset, 0), max(0, len(lines)-height))
}

func slicesIndex(values []int, want int) int {
	for index, value := range values {
		if value == want {
			return index
		}
	}
	return -1
}

func (m Model) renderListPane(width, height int) []string {
	var lines []string
	switch {
	case m.loading && len(m.pullRequests) == 0:
		lines = []string{fg(textColor).Render("Loading pull requests…")}
	case m.refreshErr != nil && !m.stale:
		refreshError := sanitizeTerminalText(m.refreshErr.Error(), false)
		lines = []string{
			fg(errorColor).Render(truncateLine("Refresh failed: "+refreshError, width)),
			fg(textColor).Render("Press r to retry or q to quit."),
		}
	case len(m.pullRequests) == 0:
		lines = []string{fg(textColor).Render("No open pull requests.")}
	default:
		lines = m.renderListRows(width, height)
	}
	return fitLines(lines, width, height)
}

func (m Model) renderListRows(width, height int) []string {
	rows := m.rows()
	labelWidth := m.labelColumnWidth(rows)
	visual := m.listLines(rows)
	top := m.scrolledTop()
	end := min(len(visual), top+height)

	lines := make([]string, 0, height)
	for _, rowIndex := range visual[top:end] {
		if rowIndex == blankLine {
			lines = append(lines, "")
			continue
		}
		row := rows[rowIndex]
		cursor := rowIndex == m.selected
		if row.isHeader() {
			lines = append(lines, m.renderGroupHeader(row.group, cursor, width))
			continue
		}
		lines = append(lines, m.renderPullRequestRow(m.pullRequests[row.pullRequestIndex], cursor, labelWidth, width))
	}
	return lines
}

func (m Model) renderGroupHeader(group app.StatusGroup, cursor bool, width int) string {
	presentation := presentationOf(group)
	count := len(m.groupIndexes()[group])
	prefix := ""
	if m.collapsed[group] {
		prefix = "▸ "
	}
	title := fmt.Sprintf("%s%s %s", prefix, presentation.glyph, presentation.label)
	if cursor {
		return cursorStyle().Render(padTo(truncateDisplayLine(fmt.Sprintf("%s %d", title, count), width), width))
	}
	var line string
	if prefix != "" {
		line = fg(mutedColor).Render(prefix) + bold(presentation.color).Render(title[len(prefix):])
	} else {
		line = bold(presentation.color).Render(title)
	}
	line += " " + fg(mutedColor).Render(fmt.Sprint(count))
	return padTo(ansi.Truncate(line, width, "…"), width)
}

func cursorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(onAccentColor).Background(accentColor)
}

func pullRequestLabel(pullRequest app.PullRequest) string {
	repository := sanitizeTerminalText(pullRequest.Repository, false)
	if _, name, ok := strings.Cut(repository, "/"); ok {
		repository = name
	}
	return fmt.Sprintf("%s#%d", oneLine(repository), pullRequest.Number)
}

func (m Model) labelColumnWidth(rows []listRow) int {
	longest := 0
	for _, row := range rows {
		if row.isHeader() {
			continue
		}
		longest = max(longest, ansi.StringWidth(pullRequestLabel(m.pullRequests[row.pullRequestIndex])))
	}
	return min(longest, maxLabelWidth)
}

func (m Model) rightColumnFor(pullRequest app.PullRequest) (string, lipgloss.Style) {
	blocked := app.StatusGroupOf(pullRequest) == app.StatusBlocked
	switch {
	case blocked && pullRequest.Checks == app.ChecksFailing:
		return "checks", fg(errorColor)
	case blocked:
		return "changes", fg(errorColor)
	case pullRequest.Checks == app.ChecksPending:
		return "◌ ci", fg(amberColor)
	}
	return compactAge(m.now(), pullRequest.CreatedAt), fg(secondaryColor)
}

func (m Model) renderPullRequestRow(pullRequest app.PullRequest, cursor bool, labelWidth, width int) string {
	checkbox := "□"
	if m.checked[pullRequest.ID] {
		checkbox = "■"
	}
	rightWidth := min(rightColumnMin, max(0, width-3))
	available := max(0, width-2-rightWidth)
	labelColumn := min(labelWidth+labelGap, available/2)

	label := padTo(truncateDisplayLine(pullRequestLabel(pullRequest), max(0, labelColumn-labelGap)), labelColumn)
	title := truncateDisplayLine(oneLine(sanitizeTerminalText(pullRequest.Title, false)), available-labelColumn)
	title = padTo(title, available-labelColumn)
	rightText, rightStyle := m.rightColumnFor(pullRequest)
	right := strings.Repeat(" ", max(0, rightWidth-ansi.StringWidth(rightText))) + truncateDisplayLine(rightText, rightWidth)

	if cursor {
		return cursorStyle().Render(padTo(checkbox+" "+label+title+right, width))
	}
	checkboxStyle := fg(mutedColor)
	if m.checked[pullRequest.ID] {
		checkboxStyle = fg(accentColor)
	}
	line := checkboxStyle.Render(checkbox) + " " +
		fg(secondaryColor).Render(label) +
		fg(textColor).Render(title) +
		rightStyle.Render(right)
	return padTo(ansi.Truncate(line, width, ""), width)
}

func fitLines(lines []string, width, height int) []string {
	fitted := make([]string, height)
	for index := range fitted {
		line := ""
		if index < len(lines) {
			line = ansi.Truncate(lines[index], width, "…")
		}
		fitted[index] = padTo(line, width)
	}
	return fitted
}
