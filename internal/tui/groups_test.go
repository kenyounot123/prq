package tui

import (
	"context"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kenyounot123/prq/internal/app"
)

func groupedTestModel(pullRequests ...app.PullRequest) Model {
	return NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		snapshotPointer(testSnapshot(pullRequests...)),
		nil,
		testDependencies(),
	)
}

func blockedPullRequest(id string, number int) app.PullRequest {
	pullRequest := testPullRequest(id, "acme/api", number)
	pullRequest.Checks = app.ChecksFailing
	return pullRequest
}

func readyPullRequest(id string, number int) app.PullRequest {
	pullRequest := testPullRequest(id, "acme/api", number)
	pullRequest.Review, pullRequest.Checks = app.ReviewApproved, app.ChecksPassing
	return pullRequest
}

func draftPullRequest(id string, number int) app.PullRequest {
	pullRequest := testPullRequest(id, "acme/api", number)
	pullRequest.Draft = true
	return pullRequest
}

// cursorAt names what the cursor rests on: a pull request ID, or the label of
// a collapsed group header.
func cursorAt(model Model) string {
	rows := model.rows()
	if model.selected < 0 || model.selected >= len(rows) {
		return "<none>"
	}
	row := rows[model.selected]
	if row.isHeader() {
		return "header:" + presentationOf(row.group).shortLabel
	}
	return model.pullRequests[row.pullRequest].ID
}

func press(t *testing.T, model Model, key tea.Key) Model {
	t.Helper()
	return updateModel(t, model, tea.KeyPressMsg(key))
}

var (
	keyDown  = tea.Key{Code: tea.KeyDown}
	keyUp    = tea.Key{Code: tea.KeyUp}
	keyTab   = tea.Key{Code: tea.KeyTab}
	keyEnter = tea.Key{Code: tea.KeyEnter}
)

func TestRowsListGroupsInDisplayOrderAndHideEmptyOnes(t *testing.T) {
	model := groupedTestModel(
		testPullRequest("PR_wait_1", "acme/api", 1),
		draftPullRequest("PR_draft", 2),
		blockedPullRequest("PR_block", 3),
		testPullRequest("PR_wait_2", "acme/api", 4),
	)

	var got []string
	for _, row := range model.rows() {
		if row.isHeader() {
			got = append(got, "header:"+presentationOf(row.group).shortLabel)
		} else {
			got = append(got, model.pullRequests[row.pullRequest].ID)
		}
	}
	want := []string{
		"header:blocked", "PR_block",
		"header:review", "PR_wait_1", "PR_wait_2",
		"header:draft",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v (no ready header, draft collapsed, service order kept)", got, want)
	}
}

func TestCursorStartsOnTheFirstPullRequestAndSkipsExpandedHeaders(t *testing.T) {
	model := groupedTestModel(
		blockedPullRequest("PR_block", 1),
		readyPullRequest("PR_ready", 2),
		draftPullRequest("PR_draft", 3),
	)

	steps := []struct {
		key  tea.Key
		want string
	}{
		{keyDown, "PR_ready"},
		{keyDown, "header:draft"},
		{keyDown, "header:draft"},
		{keyUp, "PR_ready"},
		{keyUp, "PR_block"},
		{keyUp, "PR_block"},
	}
	if got := cursorAt(model); got != "PR_block" {
		t.Fatalf("initial cursor = %q, want PR_block", got)
	}
	for _, step := range steps {
		model = press(t, model, step.key)
		if got := cursorAt(model); got != step.want {
			t.Fatalf("cursor = %q, want %q", got, step.want)
		}
	}
}

func TestTabJumpsToTheNextGroupAndWraps(t *testing.T) {
	model := groupedTestModel(
		blockedPullRequest("PR_block_1", 1),
		blockedPullRequest("PR_block_2", 2),
		testPullRequest("PR_wait", "acme/api", 3),
		draftPullRequest("PR_draft", 4),
	)

	for _, want := range []string{"PR_wait", "header:draft", "PR_block_1", "PR_wait"} {
		model = press(t, model, keyTab)
		if got := cursorAt(model); got != want {
			t.Fatalf("cursor after Tab = %q, want %q", got, want)
		}
	}
}

func TestTabInASingleGroupReturnsToItsFirstPullRequest(t *testing.T) {
	model := groupedTestModel(
		testPullRequest("PR_one", "acme/api", 1),
		testPullRequest("PR_two", "acme/api", 2),
	)
	model = press(t, model, keyDown)
	model = press(t, model, keyTab)
	if got := cursorAt(model); got != "PR_one" {
		t.Fatalf("cursor = %q, want PR_one", got)
	}
}

func TestEnterExpandsACollapsedGroupAndCollapsesAnExpandedOne(t *testing.T) {
	model := groupedTestModel(
		testPullRequest("PR_wait", "acme/api", 1),
		draftPullRequest("PR_draft_1", 2),
		draftPullRequest("PR_draft_2", 3),
	)
	model = press(t, model, keyDown)
	if got := cursorAt(model); got != "header:draft" {
		t.Fatalf("cursor = %q, want the collapsed draft header", got)
	}

	model = press(t, model, keyEnter)
	if got := cursorAt(model); got != "PR_draft_1" {
		t.Fatalf("cursor after expanding = %q, want the group's first pull request", got)
	}
	if got := len(model.rows()); got != 1+1+1+2 {
		t.Fatalf("rows after expanding = %d, want waiting and draft groups fully listed", got)
	}

	model = press(t, model, keyDown)
	model = press(t, model, keyEnter)
	if got := cursorAt(model); got != "header:draft" {
		t.Fatalf("cursor after collapsing = %q, want the group's header", got)
	}
	if got := len(model.rows()); got != 3 {
		t.Fatalf("rows after collapsing = %d, want draft pull requests hidden", got)
	}
}

func TestCollapsingDoesNotMutateEarlierModelValues(t *testing.T) {
	before := groupedTestModel(testPullRequest("PR_wait", "acme/api", 1))
	_ = press(t, before, keyEnter)
	if before.collapsed[app.StatusWaiting] {
		t.Fatal("Enter on a copy collapsed the group of the original model")
	}
}

func TestRefreshKeepsCursorOnPullRequestThatMovedGroup(t *testing.T) {
	model := groupedTestModel(
		blockedPullRequest("PR_block", 1),
		testPullRequest("PR_move", "acme/api", 2),
	)
	model = selectPullRequest(t, model, "PR_move")

	moved := readyPullRequest("PR_move", 2)
	model = updateModel(t, model, refreshDoneMsg{result: app.RefreshResult{
		Snapshot: testSnapshot(blockedPullRequest("PR_block", 1), moved),
	}})
	if got := cursorAt(model); got != "PR_move" {
		t.Fatalf("cursor after the pull request moved to ready = %q, want PR_move", got)
	}
}

func TestRefreshMovingTheCursorPullRequestIntoACollapsedGroupSelectsItsHeader(t *testing.T) {
	model := groupedTestModel(
		testPullRequest("PR_one", "acme/api", 1),
		testPullRequest("PR_move", "acme/api", 2),
	)
	model = selectPullRequest(t, model, "PR_move")

	model = updateModel(t, model, refreshDoneMsg{result: app.RefreshResult{
		Snapshot: testSnapshot(testPullRequest("PR_one", "acme/api", 1), draftPullRequest("PR_move", 2)),
	}})
	if got := cursorAt(model); got != "header:draft" {
		t.Fatalf("cursor = %q, want the collapsed draft header holding PR_move", got)
	}
}

func TestRefreshKeepsCursorOnACollapsedHeader(t *testing.T) {
	model := groupedTestModel(testPullRequest("PR_one", "acme/api", 1), draftPullRequest("PR_draft", 2))
	model = press(t, model, keyDown)
	model = updateModel(t, model, refreshDoneMsg{result: app.RefreshResult{
		Snapshot: testSnapshot(
			blockedPullRequest("PR_new", 3),
			testPullRequest("PR_one", "acme/api", 1),
			draftPullRequest("PR_draft", 2),
		),
	}})
	if got := cursorAt(model); got != "header:draft" {
		t.Fatalf("cursor = %q, want header:draft", got)
	}
}

func TestFirstRefreshPlacesCursorOnTheFirstPullRequest(t *testing.T) {
	model := NewModel(
		context.Background(),
		app.Selection{Organization: "acme", Repositories: []string{"acme/api"}},
		nil,
		nil,
		testDependencies(),
	)
	model = updateModel(t, model, refreshDoneMsg{result: app.RefreshResult{
		Snapshot: testSnapshot(readyPullRequest("PR_ready", 1), blockedPullRequest("PR_block", 2)),
	}})
	if got := cursorAt(model); got != "PR_block" {
		t.Fatalf("cursor = %q, want PR_block, the first pull request on screen", got)
	}
}
