package app

import "strings"

func EscapeSlack(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	return strings.ReplaceAll(text, ">", "&gt;")
}

func FormatSlackList(prs []PullRequest) string {
	lines := make([]string, 0, len(prs))
	for _, pr := range prs {
		lines = append(lines, "• <"+pr.URL+"|"+EscapeSlack(pr.Title)+"> — "+EscapeSlack(CompactDescription(pr.Body)))
	}
	return strings.Join(lines, "\n")
}
