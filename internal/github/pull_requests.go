package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kenyounot123/prq/internal/app"
)

func (c *Client) ListOpenPullRequests(
	ctx context.Context,
	organization string,
	repositories []string,
) ([]app.PullRequest, error) {
	selection, err := app.NormalizeSelection(organization, repositories)
	if err != nil {
		return nil, fmt.Errorf("list open pull requests: validate selection: %w", err)
	}

	viewer, err := c.CurrentViewer(ctx)
	if err != nil {
		return nil, fmt.Errorf("list open pull requests: %w", err)
	}

	allowedRepositories := make(map[string]struct{}, len(selection.Repositories))
	for _, repository := range selection.Repositories {
		allowedRepositories[strings.ToLower(repository)] = struct{}{}
	}

	searchQuery := fmt.Sprintf(
		"is:pr is:open author:%s org:%s",
		viewer,
		selection.Organization,
	)
	var pullRequests []app.PullRequest
	cursor := ""

	for {
		args := []string{
			"api",
			"graphql",
			"--hostname",
			"github.com",
			"-f",
			"query=" + openPullRequestsQuery,
			"-f",
			"search=" + searchQuery,
		}
		if cursor != "" {
			args = append(args, "-f", "cursor="+cursor)
		}

		output, err := c.runner.Run(ctx, args...)
		if err != nil {
			return nil, fmt.Errorf("list open pull requests: %w", err)
		}

		var response graphQLResponse[struct {
			Search struct {
				Nodes    []pullRequestNode `json:"nodes"`
				PageInfo pageInfo          `json:"pageInfo"`
			} `json:"search"`
		}]
		if err := decodeGraphQLResponse(output, "list open pull requests", &response); err != nil {
			return nil, err
		}

		for _, node := range response.Data.Search.Nodes {
			if _, allowed := allowedRepositories[strings.ToLower(node.Repository.NameWithOwner)]; !allowed {
				continue
			}
			pullRequest, err := normalizePullRequest(node)
			if err != nil {
				return nil, err
			}
			pullRequests = append(pullRequests, pullRequest)
		}

		nextCursor, done, err := advanceCursor(response.Data.Search.PageInfo)
		if err != nil {
			return nil, fmt.Errorf("list open pull requests: %w", err)
		}
		if done {
			return pullRequests, nil
		}
		cursor = nextCursor
	}
}

type pullRequestNode struct {
	ID                string `json:"id"`
	Number            int    `json:"number"`
	Title             string `json:"title"`
	Body              string `json:"body"`
	URL               string `json:"url"`
	IsDraft           bool   `json:"isDraft"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
	ReviewDecision    string `json:"reviewDecision"`
	StatusCheckRollup struct {
		State string `json:"state"`
	} `json:"statusCheckRollup"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

func normalizePullRequest(node pullRequestNode) (app.PullRequest, error) {
	createdAt, err := time.Parse(time.RFC3339, node.CreatedAt)
	if err != nil {
		return app.PullRequest{}, fmt.Errorf("list open pull requests: parse created timestamp: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339, node.UpdatedAt)
	if err != nil {
		return app.PullRequest{}, fmt.Errorf("list open pull requests: parse updated timestamp: %w", err)
	}

	return app.PullRequest{
		ID:         node.ID,
		Repository: node.Repository.NameWithOwner,
		Number:     node.Number,
		Title:      node.Title,
		Body:       node.Body,
		URL:        node.URL,
		Draft:      node.IsDraft,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
		Review:     app.ReviewStateFromGitHub(node.ReviewDecision),
		Checks:     app.CheckStateFromGitHub(node.StatusCheckRollup.State),
	}, nil
}
