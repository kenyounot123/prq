package tui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	columnGap  = "  "
	columnRule = "│"
)

var columnSeparatorWidth = ansi.StringWidth(columnGap + columnRule + columnGap)

func fg(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

func bold(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(c)
}

func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	view.WindowTitle = "prq"
	return view
}

func (m Model) render() string {
	width := m.contentWidth()
	layout := layoutBody(width, m.bodyHeight())
	list := m.renderListPane(layout.listWidth, layout.listHeight)
	detail := m.renderDetailPane(layout.detailWidth, layout.detailHeight)

	lines := []string{m.renderHeader(width), strings.Repeat(" ", width)}
	if layout.wide {
		separator := columnGap + fg(ruleColor).Render(columnRule) + columnGap
		for index := range list {
			lines = append(lines, list[index]+separator+detail[index])
		}
	} else {
		lines = append(lines, list...)
		if layout.listHeight+layout.detailHeight < m.bodyHeight() {
			lines = append(lines, fg(ruleColor).Render(strings.Repeat("─", width)))
		}
		lines = append(lines, detail...)
	}
	if len(m.checked) > 0 {
		lines = append(lines, m.renderTray(width))
	}
	lines = append(lines, m.footerLines()...)
	if m.height > 0 && len(lines) > m.height {
		lines = lines[len(lines)-m.height:]
	}

	margin := strings.Repeat(" ", pageMargin)
	for index, line := range lines {
		lines[index] = margin + line + margin
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHeader(width int) string {
	counts := m.groupIndexes()
	left := bold(accentColor).Render("prq")
	for _, group := range summaryOrder {
		presentation := presentationOf(group)
		left += "  " + fg(presentation.color).Render(
			fmt.Sprintf("%s %d %s", presentation.glyph, len(counts[group]), presentation.shortLabel),
		)
	}

	right := fg(mutedColor).Render(m.syncText())
	if len(m.checked) > 0 {
		right = bold(accentColor).Render(fmt.Sprintf("%d selected", len(m.checked))) + "  " + right
	}
	return justify(left, right, width)
}

func (m Model) syncText() string {
	synced := "never synced"
	if !m.fetchedAt.IsZero() {
		age := compactAge(m.now(), m.fetchedAt)
		if age == "now" {
			synced = "synced now"
		} else {
			synced = "synced " + age + " ago"
		}
	}
	switch {
	case m.refreshing && m.fetchedAt.IsZero():
		return "syncing…"
	case m.refreshing:
		return "refreshing · " + synced
	case m.stale:
		return "stale · " + synced
	}
	return synced
}

func justify(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	rightWidth := ansi.StringWidth(right)
	if rightWidth >= width {
		return padTo(ansi.Truncate(right, width, "…"), width)
	}
	left = ansi.Truncate(left, max(0, width-rightWidth-2), "…")
	gap := width - ansi.StringWidth(left) - rightWidth
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) renderTray(width int) string {
	background := lipgloss.NewStyle().Background(accentColor)
	strong := background.Bold(true).Foreground(onAccentColor)
	dim := background.Foreground(trayDimColor)

	inner := max(0, width-2)
	count := strong.Render(fmt.Sprintf("%d selected", len(m.checked)))
	hints := strong.Render("c") + dim.Render(" copy  ") + strong.Render("esc") + dim.Render(" clear")

	var labels []string
	for _, pullRequest := range m.groupedPullRequests() {
		if m.checked[pullRequest.ID] {
			labels = append(labels, pullRequestLabel(pullRequest))
		}
	}
	room := inner - ansi.StringWidth(count) - ansi.StringWidth(hints) - 4
	list := ""
	if room > 0 {
		list = dim.Render(ansi.Truncate(strings.Join(labels, " "), room, "…"))
	}

	content := count
	if list != "" {
		content += dim.Render("  ") + list
	}
	if gap := inner - ansi.StringWidth(content) - ansi.StringWidth(hints); gap >= 1 {
		content += background.Render(strings.Repeat(" ", gap)) + hints
	}
	line := background.Render(" ") + content + background.Render(" ")
	line = ansi.Truncate(line, width, "")
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += background.Render(strings.Repeat(" ", pad))
	}
	return line
}

type binding struct {
	key  string
	desc string
}

func (b binding) width() int {
	return ansi.StringWidth(b.key) + 1 + ansi.StringWidth(b.desc)
}

func (b binding) render() string {
	return bold(textColor).Render(b.key) + " " + fg(secondaryColor).Render(b.desc)
}

func (m Model) copyBinding() binding {
	if len(m.checked) > 0 {
		return binding{"c", fmt.Sprintf("copy %d", len(m.checked))}
	}
	return binding{"c", "copy url"}
}

func (m Model) footerLines() []string {
	width := m.contentWidth()
	status, hasStatus := m.currentStatus()
	statusText := ""
	if hasStatus {
		statusText = status.render()
	}

	if !m.showHelp {
		bindings := []binding{
			{"↑↓", "move"},
			{"tab", "next group"},
			{"space", "select"},
			m.copyBinding(),
			{"o", "open"},
		}
		room := width
		if statusText != "" {
			room = max(0, width-ansi.StringWidth(statusText)-2)
		}
		return []string{justifyStatus(renderBindings(fitBindings(bindings, binding{"?", "more"}, room)), statusText, width)}
	}

	lines := wrapBindings([]binding{
		{"↑↓", "move"},
		{"tab", "next group"},
		{"enter", "expand"},
		{"space", "select"},
		{"a", "select all"},
		{"esc", "clear"},
		m.copyBinding(),
		{"C", "copy all as Slack"},
		{"o", "open"},
		{"r", "refresh"},
		{"?", "less"},
		{"q", "quit"},
	}, width)
	for index, line := range lines {
		lines[index] = padTo(ansi.Truncate(line, width, "…"), width)
	}
	if hasStatus {
		lines = append(lines, justifyStatus("", statusText, width))
	}
	return lines
}

func justifyStatus(left, status string, width int) string {
	if width <= 0 {
		return ""
	}
	if status == "" {
		return padTo(ansi.Truncate(left, width, "…"), width)
	}
	statusWidth := ansi.StringWidth(status)
	leftWidth := ansi.StringWidth(left)
	if leftWidth+2+statusWidth > width {
		minStatusWidth := width / 2
		status = ansi.Truncate(status, min(statusWidth, max(minStatusWidth, width-leftWidth-2)), "…")
		statusWidth = ansi.StringWidth(status)
		left = ansi.Truncate(left, max(0, width-statusWidth-2), "…")
	}
	gap := width - ansi.StringWidth(left) - statusWidth
	return left + strings.Repeat(" ", gap) + status
}

func fitBindings(bindings []binding, pinned binding, width int) []binding {
	total := func(list []binding) int {
		sum := 2 * max(0, len(list)-1)
		for _, b := range list {
			sum += b.width()
		}
		return sum
	}
	fitted := slices.Clone(bindings)
	for len(fitted) > 0 && total(append(fitted, pinned)) > width {
		fitted = fitted[:len(fitted)-1]
	}
	return append(fitted, pinned)
}

func renderBindings(bindings []binding) string {
	rendered := make([]string, len(bindings))
	for index, b := range bindings {
		rendered[index] = b.render()
	}
	return strings.Join(rendered, "  ")
}

func wrapBindings(bindings []binding, width int) []string {
	var lines []string
	var current []binding
	currentWidth := 0
	flush := func() {
		if len(current) > 0 {
			lines = append(lines, renderBindings(current))
		}
		current, currentWidth = nil, 0
	}
	for _, b := range bindings {
		if len(current) > 0 && currentWidth+2+b.width() > width {
			flush()
		}
		if len(current) > 0 {
			currentWidth += 2
		}
		current = append(current, b)
		currentWidth += b.width()
	}
	flush()
	return lines
}

func (s statusLine) render() string {
	text := sanitizeTerminalText(s.text, false)
	switch s.kind {
	case statusSuccess:
		return fg(greenColor).Render(oneLine("✓ " + text))
	case statusWarning:
		return fg(amberColor).Render(oneLine(text))
	default:
		return fg(errorColor).Render(oneLine(text))
	}
}

func (m Model) currentStatus() (statusLine, bool) {
	switch {
	case m.status.text != "":
		return m.status, true
	case m.refreshErr != nil && m.stale:
		return statusLine{text: "Refresh failed: " + m.refreshErr.Error(), kind: statusError}, true
	case m.cacheWarning != nil:
		return statusLine{text: "Warning: " + m.cacheWarning.Error(), kind: statusWarning}, true
	}
	return statusLine{}, false
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

func padTo(value string, width int) string {
	if gap := width - ansi.StringWidth(value); gap > 0 {
		return value + strings.Repeat(" ", gap)
	}
	return value
}

func truncateLine(value string, width int) string {
	return truncateDisplayLine(oneLine(value), width)
}

func truncateDisplayLine(value string, width int) string {
	if width <= 0 {
		return ""
	}
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

func compactAge(now, then time.Time) string {
	if now.IsZero() || then.IsZero() || !now.After(then) {
		return "now"
	}
	age := now.Sub(then)
	switch {
	case age < time.Minute:
		return "now"
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age/time.Minute))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(age/(24*time.Hour)))
	}
}
