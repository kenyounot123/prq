package tui

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/kenyounot123/prq/internal/app"
)

const detailLabelWidth = 10

type valuePresentation struct {
	glyph string
	text  string
	color color.Color
}

var reviewPresentations = map[app.ReviewState]valuePresentation{
	app.ReviewApproved:         {glyph: "✓", text: "approved", color: greenColor},
	app.ReviewChangesRequested: {glyph: "✗", text: "changes requested", color: errorColor},
	app.ReviewRequired:         {glyph: "○", text: "required", color: amberColor},
	app.ReviewNone:             {text: "none", color: mutedColor},
	app.ReviewUnknown:          {text: "unknown", color: mutedColor},
}

var checkPresentations = map[app.CheckState]valuePresentation{
	app.ChecksPassing: {glyph: "✓", text: "passing", color: greenColor},
	app.ChecksFailing: {glyph: "✗", text: "failing", color: errorColor},
	app.ChecksPending: {glyph: "◌", text: "pending", color: amberColor},
	app.ChecksNone:    {text: "none", color: mutedColor},
	app.ChecksUnknown: {text: "unknown", color: mutedColor},
}

func (p valuePresentation) render() string {
	text := p.text
	if p.glyph != "" {
		text = p.glyph + " " + text
	}
	return fg(p.color).Render(text)
}

func reviewPresentationOf(state app.ReviewState) valuePresentation {
	if presentation, ok := reviewPresentations[state]; ok {
		return presentation
	}
	return valuePresentation{text: oneLine(sanitizeTerminalText(string(state), false)), color: mutedColor}
}

func checkPresentationOf(state app.CheckState) valuePresentation {
	if presentation, ok := checkPresentations[state]; ok {
		return presentation
	}
	return valuePresentation{text: oneLine(sanitizeTerminalText(string(state), false)), color: mutedColor}
}

func (m Model) renderDetailPane(width, height int) []string {
	pullRequest, ok := m.selectedPullRequest()
	if !ok {
		var message []string
		if len(m.pullRequests) > 0 {
			message = []string{fg(mutedColor).Render("No pull request selected.")}
		}
		return fitLines(message, width, height)
	}
	return fitLines(m.detailLines(pullRequest, width), width, height)
}

func (m Model) detailLines(pullRequest app.PullRequest, width int) []string {
	repository := oneLine(sanitizeTerminalText(pullRequest.Repository, false))
	lines := []string{
		fg(secondaryColor).Render(truncateDisplayLine(fmt.Sprintf("%s #%d", repository, pullRequest.Number), width)),
	}
	title := oneLine(sanitizeTerminalText(pullRequest.Title, false))
	for _, line := range wrapText(title, width) {
		lines = append(lines, bold(titleColor).Render(line))
	}
	lines = append(lines, "")

	labelled := func(label, value string) string {
		return fg(mutedColor).Render(padTo(label, detailLabelWidth)) + truncateDisplayLine(value, max(0, width-detailLabelWidth))
	}
	lines = append(lines,
		labelled("review", reviewPresentationOf(pullRequest.Review).render()),
		labelled("checks", checkPresentationOf(pullRequest.Checks).render()),
	)
	if branch := oneLine(sanitizeTerminalText(pullRequest.Branch, false)); branch != "" {
		lines = append(lines, labelled("branch", fg(bodyColor).Render(branch)))
	}
	age := fmt.Sprintf(
		"opened %s · updated %s",
		compactAge(m.now(), pullRequest.CreatedAt),
		compactAge(m.now(), pullRequest.UpdatedAt),
	)
	lines = append(lines,
		labelled("age", fg(bodyColor).Render(age)),
		"",
		fg(ruleColor).Render(strings.Repeat("─", width)),
		"",
		fg(mutedColor).Render("DESCRIPTION"),
	)

	body := strings.TrimRight(sanitizeTerminalText(pullRequest.Body, true), " \n")
	body = strings.TrimLeft(body, "\n")
	if strings.TrimSpace(body) == "" {
		return append(lines, fg(mutedColor).Render("No description"))
	}
	for _, line := range wrapText(body, width) {
		lines = append(lines, fg(bodyColor).Render(line))
	}
	return lines
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return nil
	}
	var wrapped []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " ")
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		for _, piece := range strings.Split(ansi.Wrap(line, width, ""), "\n") {
			wrapped = append(wrapped, strings.TrimRight(piece, " "))
		}
	}
	return wrapped
}
