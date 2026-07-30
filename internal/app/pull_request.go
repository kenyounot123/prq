package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

type ReviewState string

const (
	ReviewApproved         ReviewState = "approved"
	ReviewChangesRequested ReviewState = "changes requested"
	ReviewRequired         ReviewState = "review required"
	ReviewNone             ReviewState = "none"
	ReviewUnknown          ReviewState = "unknown"
)

type CheckState string

const (
	ChecksPassing CheckState = "passing"
	ChecksFailing CheckState = "failing"
	ChecksPending CheckState = "pending"
	ChecksNone    CheckState = "none"
	ChecksUnknown CheckState = "unknown"
)

type Selection struct {
	Organization string   `json:"organization"`
	Repositories []string `json:"repositories"`
}

type PullRequest struct {
	ID         string      `json:"id"`
	Repository string      `json:"repository"`
	Number     int         `json:"number"`
	Title      string      `json:"title"`
	Body       string      `json:"body"`
	URL        string      `json:"url"`
	Draft      bool        `json:"draft"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	Review     ReviewState `json:"review"`
	Checks     CheckState  `json:"checks"`
}

type Snapshot struct {
	PullRequests []PullRequest `json:"pull_requests"`
	FetchedAt    time.Time     `json:"fetched_at"`
}

func NormalizeSelection(organization string, repositories []string) (Selection, error) {
	organization = strings.TrimSpace(organization)
	if organization == "" {
		return Selection{}, fmt.Errorf("organization is required")
	}

	selection := Selection{Organization: organization}
	for _, repository := range repositories {
		repository = strings.TrimSpace(repository)
		owner, name, ok := strings.Cut(repository, "/")
		if !ok || owner == "" || name == "" || strings.Contains(name, "/") || hasWhitespace(owner) || hasWhitespace(name) {
			return Selection{}, fmt.Errorf("invalid repository %q", repository)
		}
		if !strings.EqualFold(owner, organization) {
			return Selection{}, fmt.Errorf("repository %q does not belong to organization %q", repository, organization)
		}
		if !containsRepository(selection.Repositories, repository) {
			selection.Repositories = append(selection.Repositories, repository)
		}
	}

	if len(selection.Repositories) == 0 {
		return Selection{}, fmt.Errorf("at least one repository is required")
	}

	sort.Slice(selection.Repositories, func(i, j int) bool {
		return strings.ToLower(selection.Repositories[i]) < strings.ToLower(selection.Repositories[j])
	})

	return selection, nil
}

func SortPullRequests(prs []PullRequest) {
	sort.SliceStable(prs, func(i, j int) bool {
		if prs[i].Draft != prs[j].Draft {
			return !prs[i].Draft
		}
		if !prs[i].UpdatedAt.Equal(prs[j].UpdatedAt) {
			return prs[i].UpdatedAt.Before(prs[j].UpdatedAt)
		}
		if prs[i].Repository != prs[j].Repository {
			return prs[i].Repository < prs[j].Repository
		}
		return prs[i].Number < prs[j].Number
	})
}

func ReviewStateFromGitHub(decision string) ReviewState {
	switch decision {
	case "APPROVED":
		return ReviewApproved
	case "CHANGES_REQUESTED":
		return ReviewChangesRequested
	case "REVIEW_REQUIRED":
		return ReviewRequired
	case "":
		return ReviewNone
	default:
		return ReviewUnknown
	}
}

func CheckStateFromGitHub(status string) CheckState {
	switch status {
	case "SUCCESS":
		return ChecksPassing
	case "FAILURE", "ERROR":
		return ChecksFailing
	case "PENDING", "EXPECTED":
		return ChecksPending
	case "":
		return ChecksNone
	default:
		return ChecksUnknown
	}
}

func containsRepository(repositories []string, repository string) bool {
	for _, candidate := range repositories {
		if strings.EqualFold(candidate, repository) {
			return true
		}
	}
	return false
}

func hasWhitespace(value string) bool {
	return strings.IndexFunc(value, unicode.IsSpace) >= 0
}
