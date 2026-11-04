package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/kenyounot123/prq/internal/app"
	"github.com/kenyounot123/prq/internal/github"
	"github.com/kenyounot123/prq/internal/store"
)

// exportOptions selects what `prq export` prints.
type exportOptions struct {
	// Organization overrides the configured organization. Without it, the
	// configured organization and repositories are used.
	Organization string
	// Account names a gh account whose token is used instead of the active one.
	Account string
}

// exportDocument is the JSON that `prq export` writes to stdout.
type exportDocument struct {
	Organization string            `json:"organization"`
	Repositories []string          `json:"repositories"`
	FetchedAt    time.Time         `json:"fetched_at"`
	PullRequests []app.PullRequest `json:"pull_requests"`
}

func parseExportOptions(args []string) (exportOptions, error) {
	var options exportOptions
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.Organization, "org", "", "")
	flags.StringVar(&options.Account, "account", "", "")
	if err := flags.Parse(args); err != nil {
		return exportOptions{}, fmt.Errorf("export: %w", err)
	}
	if flags.NArg() > 0 {
		return exportOptions{}, fmt.Errorf("export: unexpected arguments %q", flags.Args())
	}
	options.Organization = strings.TrimSpace(options.Organization)
	options.Account = strings.TrimSpace(options.Account)
	return options, nil
}

// exportReader is the part of the GitHub client that export needs.
type exportReader interface {
	CheckAuth(context.Context) error
	ListRepositories(context.Context, string) ([]string, error)
	ListOpenPullRequests(context.Context, string, []string) ([]app.PullRequest, error)
}

type exportDeps struct {
	loadConfig func() (app.Selection, error)
	newReader  func(account string) (exportReader, error)
	now        func() time.Time
}

func runExport(ctx context.Context, options exportOptions, stdout io.Writer, deps exportDeps) error {
	reader, err := deps.newReader(options.Account)
	if err != nil {
		return err
	}
	if err := reader.CheckAuth(ctx); err != nil {
		return presentedError{presentation: errGitHubAuthentication, cause: err}
	}

	selection, err := exportSelection(ctx, options, reader, deps.loadConfig)
	if err != nil {
		return err
	}

	pullRequests, err := reader.ListOpenPullRequests(ctx, selection.Organization, selection.Repositories)
	if err != nil {
		return err
	}
	if pullRequests == nil {
		pullRequests = []app.PullRequest{}
	}
	app.SortPullRequests(pullRequests)

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(exportDocument{
		Organization: selection.Organization,
		Repositories: selection.Repositories,
		FetchedAt:    deps.now().UTC(),
		PullRequests: pullRequests,
	})
}

// exportSelection uses the configured repositories when the organization
// matches the configuration, and every repository in the organization otherwise.
func exportSelection(
	ctx context.Context,
	options exportOptions,
	reader exportReader,
	loadConfig func() (app.Selection, error),
) (app.Selection, error) {
	configured, err := loadConfig()
	switch {
	case err == nil:
	case errors.Is(err, store.ErrNotFound) && options.Organization != "":
	case errors.Is(err, store.ErrNotFound):
		return app.Selection{}, errors.New("export: no organization configured; pass --org or run: prq setup")
	default:
		return app.Selection{}, err
	}

	if options.Organization == "" || strings.EqualFold(options.Organization, configured.Organization) {
		return app.NormalizeSelection(configured.Organization, configured.Repositories)
	}
	repositories, err := reader.ListRepositories(ctx, options.Organization)
	if err != nil {
		return app.Selection{}, err
	}
	return app.NormalizeSelection(options.Organization, repositories)
}

// newExportReader builds a GitHub client, using the named account's token when
// one is given so the active gh account is never switched.
func newExportReader(account string) (exportReader, error) {
	runner, err := github.NewExecRunner()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, presentedError{presentation: errGitHubCLINotFound, cause: err}
		}
		return nil, err
	}
	if account != "" {
		token, err := accountToken(runner.Path, account)
		if err != nil {
			return nil, err
		}
		runner.Env = []string{"GH_TOKEN=" + token}
	}
	return github.NewClient(runner), nil
}

func accountToken(ghPath, account string) (string, error) {
	command := exec.Command(ghPath, "auth", "token", "--hostname", "github.com", "--user", account)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("get token for GitHub account %q: %s; run: gh auth login", account, message)
	}
	token := strings.TrimSpace(stdout.String())
	if token == "" {
		return "", fmt.Errorf("get token for GitHub account %q: gh returned an empty token", account)
	}
	return token, nil
}
