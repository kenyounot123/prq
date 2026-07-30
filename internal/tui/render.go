package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kenyounot123/prq/internal/app"
)

var (
	accentColor = lipgloss.Color("#7D56F4")
	mutedColor  = lipgloss.Color("#6C7086")
	errorColor  = lipgloss.Color("#F38BA8")

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor)
	mutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor)
	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor)
)

func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	view.WindowTitle = "prq"
	return view
}

func (m Model) render() string {
	listWidth, detailWidth, listHeight, detailHeight := paneDimensions(
		m.width,
		m.height,
		m.chromeRows(),
	)
	listPane := m.renderListPane(listWidth, listHeight)
	detailPane := m.renderDetailPane(detailWidth, detailHeight)

	var panes string
	if m.width >= wideLayoutMin {
		panes = lipgloss.JoinHorizontal(lipgloss.Top, listPane, " ", detailPane)
	} else {
		panes = lipgloss.JoinVertical(lipgloss.Left, listPane, detailPane)
	}

	return strings.Join([]string{
		m.renderHeader(),
		panes,
		m.renderStatus(),
		m.renderFooter(),
	}, "\n")
}

func (m Model) renderHeader() string {
	countLabel := "pull requests"
	if len(m.pullRequests) == 1 {
		countLabel = "pull request"
	}
	state := "ready"
	switch {
	case m.refreshing:
		state = "refreshing"
	case m.stale:
		state = "stale"
	case m.loading:
		state = "loading"
	case m.refreshErr != nil:
		state = "error"
	}
	lastUpdated := "never"
	if !m.fetchedAt.IsZero() {
		lastUpdated = relativeAge(m.now(), m.fetchedAt)
	}
	header := fmt.Sprintf(
		"PRQ  •  %d %s  •  %s  •  last updated %s",
		len(m.pullRequests),
		countLabel,
		state,
		lastUpdated,
	)
	return headerStyle.Width(max(0, m.width)).Render(truncateLine(header, m.width))
}

func (m Model) renderListPane(width, height int) string {
	innerWidth := max(0, width-paneFrameWidth)
	innerHeight := max(0, height-paneBorderHeight)
	lines := []string{selectedStyle.Render("PULL REQUESTS")}

	switch {
	case m.loading && len(m.pullRequests) == 0:
		lines = append(lines, "", "Loading pull requests…")
	case m.refreshErr != nil && !m.stale:
		lines = append(
			lines,
			"",
			errorStyle.Render(truncateLine("Refresh failed: "+m.refreshErr.Error(), innerWidth)),
			"Press r to retry or q to quit.",
		)
	case len(m.pullRequests) == 0:
		lines = append(lines, "", "No open pull requests.")
	default:
		const rowHeight = 5
		visibleRows := max(1, (innerHeight-1)/rowHeight)
		start := 0
		if m.selected >= visibleRows {
			start = m.selected - visibleRows + 1
		}
		end := min(len(m.pullRequests), start+visibleRows)
		for index := start; index < end; index++ {
			lines = append(lines, m.renderPullRequestRow(m.pullRequests[index], index == m.selected, innerWidth)...)
		}
	}

	return renderPane(strings.Join(lines, "\n"), width, height, m.focus == listFocus)
}

func (m Model) renderPullRequestRow(pullRequest app.PullRequest, selected bool, width int) []string {
	marker := "  "
	if selected {
		marker = "› "
	}
	state := ""
	if pullRequest.Draft {
		state = "  DRAFT"
	}
	repository := truncateLine(
		fmt.Sprintf("%s%s #%d%s", marker, pullRequest.Repository, pullRequest.Number, state),
		width,
	)
	title := truncateLine("  "+oneLine(pullRequest.Title), width)
	description := truncateLine("  "+app.CompactDescription(pullRequest.Body), width)
	review := truncateLine("  review: "+string(pullRequest.Review), width)
	checks := truncateLine(
		fmt.Sprintf("  checks: %s  •  %s", pullRequest.Checks, relativeAge(m.now(), pullRequest.UpdatedAt)),
		width,
	)
	if selected {
		repository = selectedStyle.Render(repository)
		title = selectedStyle.Render(title)
	}
	return []string{repository, title, description, review, checks}
}

func (m Model) renderDetailPane(width, height int) string {
	innerWidth := max(0, width-paneFrameWidth)
	lines := []string{selectedStyle.Render("DETAIL")}
	if m.selected < 0 || m.selected >= len(m.pullRequests) {
		lines = append(lines, "No pull request selected.")
		for len(lines) < detailMetadataRows {
			lines = append(lines, "")
		}
	} else {
		pullRequest := m.pullRequests[m.selected]
		state := "open"
		if pullRequest.Draft {
			state = "draft"
		}
		lines = append(lines,
			truncateLine(fmt.Sprintf("%s #%d", pullRequest.Repository, pullRequest.Number), innerWidth),
			truncateLine(oneLine(pullRequest.Title), innerWidth),
			"State: "+state,
			"Created: "+pullRequest.CreatedAt.UTC().Format("2006-01-02 15:04 MST"),
			"Updated: "+pullRequest.UpdatedAt.UTC().Format("2006-01-02 15:04 MST"),
			"Review: "+string(pullRequest.Review),
			"Checks: "+string(pullRequest.Checks),
			truncateLine(pullRequest.URL, innerWidth),
		)
	}
	lines = append(lines, m.detailViewport.View())
	return renderPane(strings.Join(lines, "\n"), width, height, m.focus == detailFocus)
}

func (m Model) renderStatus() string {
	status := m.statusMessage
	switch {
	case status != "":
	case m.refreshErr != nil && m.stale:
		status = "Refresh failed: " + m.refreshErr.Error()
	case m.cacheWarning != nil:
		status = "Warning: " + m.cacheWarning.Error()
	}
	return lipgloss.NewStyle().
		Foreground(mutedColor).
		Width(max(0, m.width)).
		Render(truncateLine(status, m.width))
}

func (m Model) renderFooter() string {
	return mutedStyle.
		Width(max(0, m.width)).
		Render(ansi.Wordwrap(m.footerText(), m.width, ""))
}

func (m Model) footerText() string {
	help := "↑/↓ move Tab focus o open c copy C all r refresh ? help q quit"
	if m.showHelp {
		help = wrapBindings([]string{
			"↑/↓ move or scroll",
			"Tab switch focus",
			"o open",
			"c copy URL",
			"C copy all as Slack",
			"r refresh",
			"? compact help",
			"q quit",
		}, m.width)
	}
	return help
}

func wrapBindings(bindings []string, width int) string {
	if width <= 0 {
		return strings.Join(bindings, "  ")
	}

	var wrapped strings.Builder
	lineWidth := 0
	for _, binding := range bindings {
		bindingWidth := ansi.StringWidth(binding)
		if lineWidth > 0 && lineWidth+1+bindingWidth > width {
			wrapped.WriteByte('\n')
			lineWidth = 0
		}
		if lineWidth > 0 {
			wrapped.WriteByte(' ')
			lineWidth++
		}
		wrapped.WriteString(binding)
		lineWidth += bindingWidth
	}
	return wrapped.String()
}

func footerLineCount(text string, width int) int {
	if width <= 0 {
		return 1
	}
	return strings.Count(ansi.Wordwrap(text, width, ""), "\n") + 1
}

func (m Model) now() time.Time {
	if m.deps.Now != nil {
		return m.deps.Now()
	}
	if !m.fetchedAt.IsZero() {
		return m.fetchedAt
	}
	return time.Time{}
}

func renderPane(content string, width, height int, focused bool) string {
	borderColor := mutedColor
	if focused {
		borderColor = accentColor
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(max(0, width)).
		Height(max(0, height)).
		Render(content)
}

func truncateLine(value string, width int) string {
	if width <= 0 {
		return ""
	}
	value = oneLine(value)
	if ansi.StringWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return ansi.Truncate(value, width, "…")
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func relativeAge(now, then time.Time) string {
	if now.IsZero() || then.IsZero() || !now.After(then) {
		return "now"
	}
	age := now.Sub(then)
	switch {
	case age < time.Minute:
		return "now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age/time.Minute))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(age/(24*time.Hour)))
	}
}
