# Detail Pane Visual Hierarchy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the detail pane's flat metadata list with a title-first grouped summary that is easier to scan while preserving every existing pull-request field and the plain-text description viewport.

**Architecture:** Keep the change inside `internal/tui`. A focused `renderPullRequestDetailSummary` helper will sanitize, truncate, group, and style the seven summary lines; `renderDetailPane` will compose the pane heading, summary, and existing viewport. Reducing `detailMetadataRows` from nine to eight gives the unchanged viewport one additional row.

**Tech Stack:** Go 1.26, Bubble Tea v2, Bubbles viewport v2, Lip Gloss v2, Go `testing`, ANSI-normalized golden files.

## Global Constraints

- Do not add Markdown rendering or any dependency.
- Change only detail-pane presentation and its viewport height calculation.
- Keep the `DETAIL` heading, plain-text description, scrolling, sanitization, truncation, responsive layout, and domain data unchanged.
- Use the existing accent, muted, and error styles; introduce no new colors.
- Style `changes requested` and `failing` as errors; keep every other review/check state neutral.
- Do not change GitHub queries, setup, caching, sorting, list rows, navigation, copying, or URL opening.

---

### Task 1: Render the grouped detail summary

**Files:**
- Modify: `internal/tui/render_test.go`
- Modify: `internal/tui/render.go:149-181`
- Modify: `internal/tui/model.go:13-17`
- Modify: `internal/tui/model_test.go:397-428`
- Modify: `internal/tui/testdata/wide.golden`
- Modify: `internal/tui/testdata/narrow.golden`
- Modify: `internal/tui/testdata/help.golden`
- Modify: `internal/tui/testdata/help-narrow.golden`
- Modify: `internal/tui/testdata/stale.golden`

**Interfaces:**
- Consumes: `app.PullRequest`, `selectedStyle`, `mutedStyle`, `errorStyle`, `sanitizeTerminalText`, `truncateLine`, and `truncateDisplayLine`.
- Produces: `renderPullRequestDetailSummary(pullRequest app.PullRequest, width int) []string`, returning exactly seven lines: title, context, blank separator, status, activity, URL, blank separator.
- Preserves: `Model.renderDetailPane(width, height int) string` and the existing `viewport.Model` behavior.

- [ ] **Step 1: Write the failing focused hierarchy test**

Add this test near the other focused renderer tests in `internal/tui/render_test.go`:

```go
func TestRenderPullRequestDetailSummaryUsesGroupedVisualHierarchy(t *testing.T) {
	pullRequest := app.PullRequest{
		Repository: "acme/api",
		Number:     184,
		Title:      "Fix session refresh",
		URL:        "https://github.com/acme/api/pull/184",
		CreatedAt:  time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		Review:     app.ReviewChangesRequested,
		Checks:     app.ChecksFailing,
	}

	got := renderPullRequestDetailSummary(pullRequest, 120)
	want := []string{
		selectedStyle.Render("Fix session refresh"),
		mutedStyle.Render("acme/api #184  •  OPEN"),
		"",
		errorStyle.Render("Review changes requested") + "  •  " + errorStyle.Render("Checks failing"),
		mutedStyle.Render("Created 2026-07-20 12:00 UTC  •  Updated 2026-07-29 12:00 UTC"),
		mutedStyle.Render("https://github.com/acme/api/pull/184"),
		"",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("renderPullRequestDetailSummary() = %#v, want %#v", got, want)
	}

	pullRequest.Review = app.ReviewApproved
	pullRequest.Checks = app.ChecksPassing
	got = renderPullRequestDetailSummary(pullRequest, 120)
	if got[3] != "Review approved  •  Checks passing" {
		t.Fatalf("neutral status line = %q, want unstyled neutral statuses", got[3])
	}
}
```

This test catches a flat or repository-first layout, missing grouping whitespace, incorrect status coloring, accidental coloring of healthy states, and removal of any summary field.

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
go test ./internal/tui -run TestRenderPullRequestDetailSummaryUsesGroupedVisualHierarchy -count=1
```

Expected: FAIL to compile because `renderPullRequestDetailSummary` does not exist yet.

- [ ] **Step 3: Implement the minimal grouped summary**

Add the helper below `renderDetailPane` in `internal/tui/render.go`, then replace the selected-pull-request branch in `renderDetailPane` with `lines = append(lines, renderPullRequestDetailSummary(m.pullRequests[m.selected], innerWidth)...)`:

```go
func renderPullRequestDetailSummary(pullRequest app.PullRequest, width int) []string {
	repositoryText := sanitizeTerminalText(pullRequest.Repository, false)
	titleText := sanitizeTerminalText(pullRequest.Title, false)
	urlText := sanitizeTerminalText(pullRequest.URL, false)
	reviewText := sanitizeTerminalText(string(pullRequest.Review), false)
	checksText := sanitizeTerminalText(string(pullRequest.Checks), false)
	state := "OPEN"
	if pullRequest.Draft {
		state = "DRAFT"
	}

	review := "Review " + reviewText
	if pullRequest.Review == app.ReviewChangesRequested {
		review = errorStyle.Render(review)
	}
	checks := "Checks " + checksText
	if pullRequest.Checks == app.ChecksFailing {
		checks = errorStyle.Render(checks)
	}

	return []string{
		selectedStyle.Render(truncateLine(titleText, width)),
		mutedStyle.Render(truncateDisplayLine(
			fmt.Sprintf("%s #%d  •  %s", repositoryText, pullRequest.Number, state),
			width,
		)),
		"",
		truncateDisplayLine(review+"  •  "+checks, width),
		mutedStyle.Render(truncateDisplayLine(
			fmt.Sprintf(
				"Created %s  •  Updated %s",
				pullRequest.CreatedAt.UTC().Format("2006-01-02 15:04 MST"),
				pullRequest.UpdatedAt.UTC().Format("2006-01-02 15:04 MST"),
			),
			width,
		)),
		mutedStyle.Render(truncateLine(urlText, width)),
		"",
	}
}
```

Do not alter `m.detailViewport.View()` or the no-selection message.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
go test ./internal/tui -run TestRenderPullRequestDetailSummaryUsesGroupedVisualHierarchy -count=1
```

Expected: PASS.

- [ ] **Step 5: Write the failing viewport-height expectations**

In `TestWindowResizeUpdatesDimensionsAndDetailViewport` in `internal/tui/model_test.go`, change the expected wide height from `18` to `19` and the expected narrow height from `4` to `5` without changing production constants.

- [ ] **Step 6: Run the viewport test and verify RED**

Run:

```bash
go test ./internal/tui -run TestWindowResizeUpdatesDimensionsAndDetailViewport -count=1
```

Expected: FAIL with actual heights `18` and `4`, proving the summary has not yet returned its saved row to the viewport.

- [ ] **Step 7: Increase the detail viewport by one row**

In `internal/tui/model.go`, change:

```go
detailMetadataRows = 9
```

to:

```go
detailMetadataRows = 8
```

- [ ] **Step 8: Run the viewport test and verify GREEN**

Run:

```bash
go test ./internal/tui -run TestWindowResizeUpdatesDimensionsAndDetailViewport -count=1
```

Expected: PASS with wide height `19` and narrow height `5`.

- [ ] **Step 9: Update sanitization assertions and regenerate golden views**

In `TestRenderSanitizesUntrustedPullRequestTextWithoutMutatingDomainData`, update the required status fragments from `Review: approved�now` and `Checks: passing�now` to `Review approved�now` and `Checks passing�now`. Keep every forbidden-control-sequence assertion unchanged.

Regenerate the existing TUI golden files:

```bash
UPDATE_GOLDEN=1 go test ./internal/tui -run 'TestRenderWideAndNarrowLayouts|TestRenderRefreshAndHelpStates' -count=1
```

Expected: PASS with title-first grouped detail summaries in wide, narrow, help, and stale views. The description remains plain text and visible.

- [ ] **Step 10: Run complete verification**

Run:

```bash
gofmt -w internal/tui/render.go internal/tui/render_test.go internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -count=1
go test ./...
go vet ./...
test ! -e /tmp/prq-detail-pane-verify-20260802
go build -o /tmp/prq-detail-pane-verify-20260802 ./cmd/prq
rm /tmp/prq-detail-pane-verify-20260802
git diff --check
```

Expected: every command exits successfully with no warnings, build artifacts, or formatting errors.

- [ ] **Step 11: Commit the implementation**

```bash
git add internal/tui/render.go internal/tui/render_test.go internal/tui/model.go internal/tui/model_test.go internal/tui/testdata/*.golden
git commit -m "feat: improve detail pane hierarchy"
```
