package app

import "testing"

func TestEscapeSlack(t *testing.T) {
	if got, want := EscapeSlack("Fish & <chips>"), "Fish &amp; &lt;chips&gt;"; got != want {
		t.Fatalf("EscapeSlack() = %q, want %q", got, want)
	}
}

func TestFormatSlackList(t *testing.T) {
	prs := []PullRequest{
		{
			URL:   "https://github.com/acme/api/pull/184",
			Title: "Fix <sessions> & cookies",
			Body:  "Refreshes expired sessions.",
		},
		{
			URL:   "https://github.com/acme/web/pull/92",
			Title: "Invoice filters",
			Body:  "",
		},
	}
	want := "• <https://github.com/acme/api/pull/184|Fix &lt;sessions&gt; &amp; cookies> — Refreshes expired sessions.\n" +
		"• <https://github.com/acme/web/pull/92|Invoice filters> — No description"
	if got := FormatSlackList(prs); got != want {
		t.Fatalf("FormatSlackList() = %q, want %q", got, want)
	}
}
