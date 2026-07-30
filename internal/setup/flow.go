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

	organization, err := f.Prompt.SelectOrganization(ctx, organizations)
	if err != nil {
		return app.Selection{}, fmt.Errorf("select organization: %w", err)
	}
	if !containsFold(organizations, organization) {
		return app.Selection{}, fmt.Errorf("validate selection: organization %q is not available", organization)
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
	for _, repository := range selectedRepositories {
		if !containsFold(repositories, repository) {
			return app.Selection{}, fmt.Errorf("validate selection: repository %q is not available", repository)
		}
	}

	selection, err := app.NormalizeSelection(organization, selectedRepositories)
	if err != nil {
		return app.Selection{}, fmt.Errorf("validate selection: %w", err)
	}
	if err := f.Config.Save(selection); err != nil {
		return app.Selection{}, fmt.Errorf("save configuration: %w", err)
	}
	if err := f.Cache.Clear(); err != nil {
		return app.Selection{}, fmt.Errorf("clear cache: %w", err)
	}
	return selection, nil
}

func containsFold(values []string, value string) bool {
	for _, candidate := range values {
		if strings.EqualFold(candidate, value) {
			return true
		}
	}
	return false
}
