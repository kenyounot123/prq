package app

import (
	"regexp"
	"strings"
)

var markdownLineMarker = regexp.MustCompile(`^(?:#{1,6}\s*|[-*+]\s*|\d+[.)]\s*)`)

func CompactDescription(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = markdownLineMarker.ReplaceAllString(line, "")
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			return line
		}
	}

	return "No description"
}
