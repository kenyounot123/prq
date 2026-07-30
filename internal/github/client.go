package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type Client struct {
	runner Runner
}

func NewClient(runner Runner) *Client {
	return &Client{runner: runner}
}

func (c *Client) CheckAuth(ctx context.Context) error {
	if _, err := c.runner.Run(ctx, "auth", "status", "--hostname", "github.com"); err != nil {
		return fmt.Errorf("check GitHub authentication: %w", err)
	}
	return nil
}

func (c *Client) CurrentViewer(ctx context.Context) (string, error) {
	output, err := c.runner.Run(
		ctx,
		"api",
		"graphql",
		"--hostname",
		"github.com",
		"-f",
		"query="+currentViewerQuery,
	)
	if err != nil {
		return "", fmt.Errorf("get current viewer: %w", err)
	}

	var response graphQLResponse[struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
	}]
	if err := decodeGraphQLResponse(output, "get current viewer", &response); err != nil {
		return "", err
	}
	if response.Data.Viewer.Login == "" {
		return "", fmt.Errorf("get current viewer: response contains an empty login")
	}
	return response.Data.Viewer.Login, nil
}

func (c *Client) ListOrganizations(ctx context.Context) ([]string, error) {
	var organizations []string
	cursor := ""

	for {
		args := []string{
			"api",
			"graphql",
			"--hostname",
			"github.com",
			"-f",
			"query=" + viewerOrganizationsQuery,
		}
		if cursor != "" {
			args = append(args, "-f", "cursor="+cursor)
		}

		output, err := c.runner.Run(ctx, args...)
		if err != nil {
			return nil, fmt.Errorf("list organizations: %w", err)
		}

		var response graphQLResponse[struct {
			Viewer struct {
				Organizations struct {
					Nodes []struct {
						Login string `json:"login"`
					} `json:"nodes"`
					PageInfo pageInfo `json:"pageInfo"`
				} `json:"organizations"`
			} `json:"viewer"`
		}]
		if err := decodeGraphQLResponse(output, "list organizations", &response); err != nil {
			return nil, err
		}

		for _, organization := range response.Data.Viewer.Organizations.Nodes {
			organizations = append(organizations, organization.Login)
		}
		nextCursor, done, err := advanceCursor(response.Data.Viewer.Organizations.PageInfo)
		if err != nil {
			return nil, fmt.Errorf("list organizations: %w", err)
		}
		if done {
			break
		}
		cursor = nextCursor
	}

	sortCaseInsensitive(organizations)
	return organizations, nil
}

func (c *Client) ListRepositories(ctx context.Context, organization string) ([]string, error) {
	var repositories []string
	cursor := ""

	for {
		args := []string{
			"api",
			"graphql",
			"--hostname",
			"github.com",
			"-f",
			"query=" + organizationRepositoriesQuery,
			"-f",
			"organization=" + organization,
		}
		if cursor != "" {
			args = append(args, "-f", "cursor="+cursor)
		}

		output, err := c.runner.Run(ctx, args...)
		if err != nil {
			return nil, fmt.Errorf("list repositories: %w", err)
		}

		var response graphQLResponse[struct {
			Organization struct {
				Repositories struct {
					Nodes []struct {
						NameWithOwner string `json:"nameWithOwner"`
					} `json:"nodes"`
					PageInfo pageInfo `json:"pageInfo"`
				} `json:"repositories"`
			} `json:"organization"`
		}]
		if err := decodeGraphQLResponse(output, "list repositories", &response); err != nil {
			return nil, err
		}

		for _, repository := range response.Data.Organization.Repositories.Nodes {
			repositories = append(repositories, repository.NameWithOwner)
		}
		nextCursor, done, err := advanceCursor(response.Data.Organization.Repositories.PageInfo)
		if err != nil {
			return nil, fmt.Errorf("list repositories: %w", err)
		}
		if done {
			break
		}
		cursor = nextCursor
	}

	sortCaseInsensitive(repositories)
	return repositories, nil
}

type graphQLResponse[T any] struct {
	Data   T                 `json:"data"`
	Errors []json.RawMessage `json:"errors"`
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

func decodeGraphQLResponse[T any](output []byte, operation string, response *graphQLResponse[T]) error {
	if err := json.Unmarshal(output, response); err != nil {
		return fmt.Errorf("%s: decode GitHub GraphQL response: %w", operation, err)
	}
	if len(response.Errors) > 0 {
		return fmt.Errorf("%s: GitHub GraphQL returned %d error(s)", operation, len(response.Errors))
	}
	return nil
}

func advanceCursor(info pageInfo) (cursor string, done bool, err error) {
	if !info.HasNextPage {
		return "", true, nil
	}
	if info.EndCursor == "" {
		return "", false, errors.New("response has next page with empty end cursor")
	}
	return info.EndCursor, false, nil
}

func sortCaseInsensitive(values []string) {
	slices.SortFunc(values, func(left, right string) int {
		if folded := strings.Compare(strings.ToLower(left), strings.ToLower(right)); folded != 0 {
			return folded
		}
		return strings.Compare(left, right)
	})
}
