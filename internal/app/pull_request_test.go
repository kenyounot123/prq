package app

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestNormalizeSelection(t *testing.T) {
	tests := map[string]struct {
		organization string
		repositories []string
		want         Selection
		wantErr      bool
	}{
		"trims organization, deduplicates repositories, and sorts case-insensitively": {
			organization: " Acme ",
			repositories: []string{" Acme/Web ", "acme/api", "ACME/web"},
			want: Selection{
				Organization: "Acme",
				Repositories: []string{"acme/api", "Acme/Web"},
			},
		},
		"rejects an empty repository list": {
			organization: "Acme",
			wantErr:      true,
		},
		"rejects an invalid repository name": {
			organization: "Acme",
			repositories: []string{"acme/api/extra"},
			wantErr:      true,
		},
		"rejects whitespace within a repository name": {
			organization: "Acme",
			repositories: []string{"acme/api name"},
			wantErr:      true,
		},
		"rejects a repository owned by another organization": {
			organization: "Acme",
			repositories: []string{"other/api"},
			wantErr:      true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := NormalizeSelection(tt.organization, tt.repositories)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NormalizeSelection() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NormalizeSelection() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSortPullRequests(t *testing.T) {
	t0 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		prs  []PullRequest
		want []int
	}{
		"groups drafts after non-drafts and sorts stale pull requests first": {
			prs: []PullRequest{
				{Repository: "acme/zeta", Number: 8, Draft: true, UpdatedAt: t0},
				{Repository: "acme/api", Number: 2, UpdatedAt: t0.Add(2 * time.Hour)},
				{Repository: "acme/api", Number: 1, UpdatedAt: t0},
				{Repository: "acme/web", Number: 3, Draft: true, UpdatedAt: t0.Add(-time.Hour)},
			},
			want: []int{1, 2, 3, 8},
		},
		"breaks update ties by repository then pull request number": {
			prs: []PullRequest{
				{Repository: "acme/web", Number: 2, UpdatedAt: t0},
				{Repository: "acme/api", Number: 3, UpdatedAt: t0},
				{Repository: "acme/api", Number: 1, UpdatedAt: t0},
			},
			want: []int{1, 3, 2},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			SortPullRequests(tt.prs)

			got := make([]int, len(tt.prs))
			for i, pr := range tt.prs {
				got[i] = pr.Number
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("numbers = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReviewStateFromGitHub(t *testing.T) {
	tests := map[string]struct {
		decision string
		want     ReviewState
	}{
		"approved":          {"APPROVED", ReviewApproved},
		"changes requested": {"CHANGES_REQUESTED", ReviewChangesRequested},
		"review required":   {"REVIEW_REQUIRED", ReviewRequired},
		"empty":             {"", ReviewNone},
		"unknown":           {"DISMISSED", ReviewUnknown},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ReviewStateFromGitHub(tt.decision); got != tt.want {
				t.Fatalf("ReviewStateFromGitHub(%q) = %q, want %q", tt.decision, got, tt.want)
			}
		})
	}
}

func TestCheckStateFromGitHub(t *testing.T) {
	tests := map[string]struct {
		status string
		want   CheckState
	}{
		"success":  {"SUCCESS", ChecksPassing},
		"failure":  {"FAILURE", ChecksFailing},
		"error":    {"ERROR", ChecksFailing},
		"pending":  {"PENDING", ChecksPending},
		"expected": {"EXPECTED", ChecksPending},
		"empty":    {"", ChecksNone},
		"unknown":  {"CANCELLED", ChecksUnknown},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := CheckStateFromGitHub(tt.status); got != tt.want {
				t.Fatalf("CheckStateFromGitHub(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
