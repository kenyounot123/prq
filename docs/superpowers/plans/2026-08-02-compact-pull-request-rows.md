# Compact Pull Request Rows Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render every pull request as a title-first two-line entry with muted repository metadata and a blank separator, while keeping description and status information in the detail pane.

**Architecture:** Keep the change inside `internal/tui`: `renderPullRequestRow` owns the content order and styles, while `renderListPane` owns the three-line entry footprint used for visibility calculations. A focused row-rendering test establishes the hierarchy before production changes, and the existing full-screen golden tests cover responsive layout and unchanged detail content.

**Tech Stack:** Go 1.26, Bubble Tea v2, Lip Gloss v2, Go `testing`, ANSI-normalized golden files.

## Global Constraints

- Do not add Markdown rendering or any dependency.
- Change only the pull-request list pane; leave the detail pane and domain data unchanged.
- Preserve terminal-text sanitization and width truncation on the title and repository metadata.
- Preserve the `DRAFT` marker on the metadata line.
- Keep navigation, sorting, copying, refresh, cache, and selection behavior unchanged.

---

### Task 1: Render compact title-first pull-request rows

**Files:**
- Modify: `internal/tui/render_test.go`
- Modify: `internal/tui/render.go:92-156`
- Modify: `internal/tui/testdata/wide.golden`
- Modify: `internal/tui/testdata/narrow.golden`
- Modify: `internal/tui/testdata/help.golden`
- Modify: `internal/tui/testdata/help-narrow.golden`
- Modify: `internal/tui/testdata/stale.golden`

**Interfaces:**
- Consumes: `Model.renderPullRequestRow(pullRequest app.PullRequest, selected bool, width int) []string`, `selectedStyle`, `mutedStyle`, `sanitizeTerminalText`, and `truncateLine`.
- Produces: the same `renderPullRequestRow` signature, returning exactly three strings per entry: title, metadata, and blank separator.

- [ ] **Step 1: Write the failing focused renderer test**

Add this test near the other renderer tests in `internal/tui/render_test.go`:

```go
func TestRenderPullRequestRowUsesTitleFirstVisualHierarchy(t *testing.T) {
	pullRequest := app.PullRequest{
		Repository: "acme/api",
		Number:     184,
		Title:      "Fix session refresh",
		Body:       "Description must not appear in the list.",
		Draft:      true,
		Review:     app.ReviewApproved,
		Checks:     app.ChecksPassing,
		UpdatedAt:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
	}

	got := (Model{}).renderPullRequestRow(pullRequest, true, 80)
	want := []string{
		selectedStyle.Render("› Fix session refresh"),
		mutedStyle.Render("  acme/api #184  DRAFT"),
		"",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("renderPullRequestRow() = %#v, want %#v", got, want)
	}
}
```

This exact assertion proves the title comes first, the selected accent does not overpower the metadata, the draft marker remains, and description/review/check/age lines are absent.

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
go test ./internal/tui -run TestRenderPullRequestRowUsesTitleFirstVisualHierarchy -count=1
```

Expected: FAIL because the existing renderer returns five lines in repository-first order and does not mute metadata.

- [ ] **Step 3: Implement the minimal compact renderer**

In `internal/tui/render.go`, change the list entry height and rewrite `renderPullRequestRow` to the following behavior:

```go
const rowHeight = 3
```

```go
func (m Model) renderPullRequestRow(pullRequest app.PullRequest, selected bool, width int) []string {
	repositoryText := sanitizeTerminalText(pullRequest.Repository, false)
	titleText := sanitizeTerminalText(pullRequest.Title, false)
	marker := "  "
	if selected {
		marker = "› "
	}
	state := ""
	if pullRequest.Draft {
		state = "  DRAFT"
	}
	title := truncateLine(marker+oneLine(titleText), width)
	metadata := truncateLine(
		fmt.Sprintf("  %s #%d%s", repositoryText, pullRequest.Number, state),
		width,
	)
	if selected {
		title = selectedStyle.Render(title)
	}
	metadata = mutedStyle.Render(metadata)
	return []string{title, metadata, ""}
}
```

Do not change `renderDetailPane` or `syncDetailViewport`.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
go test ./internal/tui -run TestRenderPullRequestRowUsesTitleFirstVisualHierarchy -count=1
```

Expected: PASS.

- [ ] **Step 5: Update full-screen assertions and regenerate goldens**

In `TestRenderWideAndNarrowLayouts`, remove the lowercase list-only requirements `review: approved`, `checks: passing`, `review: changes requested`, and `checks: failing`. Retain the body-text and capitalized detail metadata requirements so the test continues to prove the detail pane is unchanged.

Regenerate only the existing TUI golden files by running:

```bash
UPDATE_GOLDEN=1 go test ./internal/tui -run 'TestRenderWideAndNarrowLayouts|TestRenderRefreshAndHelpStates' -count=1
```

Expected: PASS and updates to the five list-bearing golden files; `refresh-error.golden` may be rewritten without a semantic change because it has no pull-request entries.

- [ ] **Step 6: Verify renderer and full project behavior**

Run:

```bash
gofmt -w internal/tui/render.go internal/tui/render_test.go
go test ./internal/tui -count=1
go test ./...
go vet ./...
go build ./cmd/prq
git diff --check
```

Expected: every command exits successfully with no warnings or formatting errors.

- [ ] **Step 7: Commit the implementation**

```bash
git add internal/tui/render.go internal/tui/render_test.go internal/tui/testdata/*.golden
git commit -m "feat: simplify pull request list rows"
```
