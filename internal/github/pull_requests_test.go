package github

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kenyounot123/prq/internal/app"
)

func TestListOpenPullRequestsPaginatesFiltersAndNormalizes(t *testing.T) {
	runner := &fakeRunner{
		outputs: [][]byte{
			readFixture(t, "viewer.json"),
			readFixture(t, "pull-requests-page-1.json"),
			readFixture(t, "pull-requests-page-2.json"),
		},
		errs: []error{nil, nil, nil},
	}

	pullRequests, err := NewClient(runner).ListOpenPullRequests(
		context.Background(),
		" Acme ",
		[]string{" acme/web ", "ACME/API", "Acme/Web"},
	)
	if err != nil {
		t.Fatalf("ListOpenPullRequests() error = %v", err)
	}

	want := []app.PullRequest{
		{
			ID:         "PR_approved",
			Repository: "Acme/API",
			Number:     17,
			Title:      "Keep sessions alive",
			Body:       "Refreshes expired sessions.",
			URL:        "https://github.com/Acme/API/pull/17",
			Draft:      false,
			CreatedAt:  time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC),
			Review:     app.ReviewApproved,
			Checks:     app.ChecksPassing,
		},
		{
			ID:         "PR_draft",
			Repository: "acme/web",
			Number:     28,
			Title:      "Draft navigation",
			Body:       "",
			URL:        "https://github.com/acme/web/pull/28",
			Draft:      true,
			CreatedAt:  time.Date(2026, 7, 11, 8, 15, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2026, 7, 12, 9, 45, 0, 0, time.UTC),
			Review:     app.ReviewNone,
			Checks:     app.ChecksNone,
		},
		{
			ID:         "PR_changes",
			Repository: "ACME/api",
			Number:     31,
			Title:      "Tighten request validation",
			Body:       "Reject malformed requests before dispatch.",
			URL:        "https://github.com/ACME/api/pull/31",
			Draft:      false,
			CreatedAt:  time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2026, 7, 14, 14, 0, 0, 0, time.UTC),
			Review:     app.ReviewChangesRequested,
			Checks:     app.ChecksFailing,
		},
		{
			ID:         "PR_unknown",
			Repository: "Acme/Web",
			Number:     42,
			Title:      "Exercise future enum values",
			Body:       "Unknown aggregate states remain visible.",
			URL:        "https://github.com/Acme/Web/pull/42",
			Draft:      false,
			CreatedAt:  time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2026, 7, 16, 16, 0, 0, 0, time.UTC),
			Review:     app.ReviewUnknown,
			Checks:     app.ChecksUnknown,
		},
	}
	if !reflect.DeepEqual(pullRequests, want) {
		t.Fatalf("ListOpenPullRequests() = %#v, want %#v", pullRequests, want)
	}

	const searchQuery = "is:pr is:open author:OctoCat org:Acme"
	wantCalls := [][]string{
		{
			"api",
			"graphql",
			"--hostname",
			"github.com",
			"-f",
			"query=" + currentViewerQuery,
		},
		{
			"api",
			"graphql",
			"--hostname",
			"github.com",
			"-f",
			"query=" + openPullRequestsQuery,
			"-f",
			"search=" + searchQuery,
		},
		{
			"api",
			"graphql",
			"--hostname",
			"github.com",
			"-f",
			"query=" + openPullRequestsQuery,
			"-f",
			"search=" + searchQuery,
			"-f",
			"cursor=pull-requests-cursor-1",
		},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, wantCalls)
	}
	for _, call := range runner.calls[1:] {
		if call[5] != "query="+openPullRequestsQuery {
			t.Fatalf("GraphQL document argument = %q, want fixed query byte-for-byte", call[5])
		}
		for _, repository := range []string{"acme/web", "ACME/API", "Acme/Web"} {
			if strings.Contains(call[5], repository) {
				t.Fatalf("GraphQL document contains repository %q: %q", repository, call[5])
			}
		}
	}
}

func TestListOpenPullRequestsRejectsPartialGraphQLErrors(t *testing.T) {
	runner := &fakeRunner{
		outputs: [][]byte{
			readFixture(t, "viewer.json"),
			readFixture(t, "pull-requests-partial-error.json"),
		},
		errs: []error{nil, nil},
	}

	pullRequests, err := NewClient(runner).ListOpenPullRequests(
		context.Background(),
		"Acme",
		[]string{"Acme/API"},
	)
	if err == nil {
		t.Fatal("ListOpenPullRequests() error = nil, want GraphQL error")
	}
	if pullRequests != nil {
		t.Fatalf("ListOpenPullRequests() = %#v, want nil on partial GraphQL error", pullRequests)
	}
	if !strings.Contains(err.Error(), "list open pull requests") {
		t.Fatalf("ListOpenPullRequests() error = %q, want operation context", err)
	}
}

func TestListOpenPullRequestsValidatesSelectionBeforeGitHubCalls(t *testing.T) {
	runner := &fakeRunner{}

	pullRequests, err := NewClient(runner).ListOpenPullRequests(
		context.Background(),
		"Acme",
		[]string{"Other/API"},
	)
	if err == nil {
		t.Fatal("ListOpenPullRequests() error = nil, want invalid selection error")
	}
	if pullRequests != nil {
		t.Fatalf("ListOpenPullRequests() = %#v, want nil", pullRequests)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %#v, want no GitHub calls for invalid selection", runner.calls)
	}
}
