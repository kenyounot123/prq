package setup

import (
	"context"
	"fmt"
	"strings"

	"github.com/kenyounot123/prq/internal/app"
)

type GitHub interface {
	CheckAuth(context.Context) error
	ListOrganizations(context.Context) ([]string, error)
	ListRepositories(context.Context, string) ([]string, error)
}

type ConfigWriter interface {
	Save(app.Selection) error
}

type CacheInvalidator interface {
	Clear() error
}

type Prompter interface {
	SelectOrganization(context.Context, []string) (string, error)
	SelectRepositories(context.Context, string, []string) ([]string, error)
}

type Flow struct {
	GitHub GitHub
	Config ConfigWriter
	Cache  CacheInvalidator
	Prompt Prompter
}

func (f Flow) Run(ctx context.Context) (app.Selection, error) {
	if err := f.GitHub.CheckAuth(ctx); err != nil {
		return app.Selection{}, fmt.Errorf("check authentication: %w", err)
	}

	organizations, err := f.GitHub.ListOrganizations(ctx)
	if err != nil {
		return app.Selection{}, fmt.Errorf("list organizations: %w", err)
	}
	if len(organizations) == 0 {
		return app.Selection{}, fmt.Errorf("list organizations: no GitHub organizations available")
	}

	selectedOrganization, err := f.Prompt.SelectOrganization(ctx, organizations)
	if err != nil {
		return app.Selection{}, fmt.Errorf("select organization: %w", err)
	}
	organization, ok := canonicalValue(organizations, selectedOrganization)
	if !ok {
		return app.Selection{}, fmt.Errorf("validate selection: organization %q is not available", selectedOrganization)
	}

	repositories, err := f.GitHub.ListRepositories(ctx, organization)
	if err != nil {
		return app.Selection{}, fmt.Errorf("list repositories: %w", err)
	}
	if len(repositories) == 0 {
		return app.Selection{}, fmt.Errorf("list repositories: no repositories available for %q", organization)
	}

	selectedRepositories, err := f.Prompt.SelectRepositories(ctx, organization, repositories)
	if err != nil {
		return app.Selection{}, fmt.Errorf("select repositories: %w", err)
	}
	canonicalRepositories := make([]string, 0, len(selectedRepositories))
	for _, selectedRepository := range selectedRepositories {
		repository, ok := canonicalValue(repositories, selectedRepository)
		if !ok {
			return app.Selection{}, fmt.Errorf("validate selection: repository %q is not available", selectedRepository)
		}
		canonicalRepositories = append(canonicalRepositories, repository)
	}

	selection, err := app.NormalizeSelection(organization, canonicalRepositories)
	if err != nil {
		return app.Selection{}, fmt.Errorf("validate selection: %w", err)
	}
	if err := f.Config.Save(selection); err != nil {
		return app.Selection{}, fmt.Errorf("save configuration: %w", err)
	}
	// The cache is selection-bound, so a cache left behind after this
	// best-effort cleanup cannot be loaded for the newly committed selection.
	_ = f.Cache.Clear()
	return selection, nil
}

func canonicalValue(values []string, value string) (string, bool) {
	for _, candidate := range values {
		if strings.EqualFold(candidate, value) {
			return candidate, true
		}
	}
	return "", false
}
