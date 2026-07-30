package app

import "testing"

func TestCompactDescription(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"blank":        {" \n\t", "No description"},
		"heading":      {"\n## Refresh sessions\nMore detail", "Refresh sessions"},
		"bullet":       {"- Avoid repeated requests", "Avoid repeated requests"},
		"numbered":     {"1. Add cache\n2. Add tests", "Add cache"},
		"collapse":     {"Keeps   the\tcontext  visible", "Keeps the context visible"},
		"CRLF heading": {"\r\n### Refresh sessions\r\nMore detail", "Refresh sessions"},
		"empty marker": {"-\nActual description", "Actual description"},
		"hashtag":      {"#hashtag", "#hashtag"},
		"attached bullet": {
			"-not a list item",
			"-not a list item",
		},
		"attached numbered prefix": {
			"1.version note",
			"1.version note",
		},
		"seven hashes": {"####### text", "####### text"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := CompactDescription(tt.body); got != tt.want {
				t.Fatalf("CompactDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}
