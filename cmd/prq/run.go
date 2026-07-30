package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kenyounot123/prq/internal/app"
	"github.com/kenyounot123/prq/internal/github"
	"github.com/kenyounot123/prq/internal/platform"
	"github.com/kenyounot123/prq/internal/setup"
	"github.com/kenyounot123/prq/internal/store"
	"github.com/kenyounot123/prq/internal/tui"
)

const usage = `Usage:
  prq
  prq setup
  prq --help
  prq --version
`

var (
	version                 = "dev"
	errGitHubCLINotFound    = errors.New("GitHub CLI not found")
	errGitHubAuthentication = errors.New("GitHub authentication required")
)

type command int

const (
	commandDashboard command = iota
	commandSetup
	commandHelp
	commandVersion
)

type runtimeDeps struct {
	loadConfig   func() (app.Selection, error)
	loadCache    func() (app.Snapshot, error)
	runSetup     func(context.Context) (app.Selection, error)
	runDashboard func(context.Context, app.Selection, *app.Snapshot, error) error
}

type programRunner func(context.Context, tea.Model, io.Reader, io.Writer) error

type runtimeOptions struct {
	paths      store.Paths
	input      io.Reader
	output     io.Writer
	goos       string
	runProgram programRunner
}

func parseCommand(args []string) (command, error) {
	switch {
	case len(args) == 0:
		return commandDashboard, nil
	case len(args) == 1 && args[0] == "setup":
		return commandSetup, nil
	case len(args) == 1 && (args[0] == "--help" || args[0] == "-h"):
		return commandHelp, nil
	case len(args) == 1 && args[0] == "--version":
		return commandVersion, nil
	default:
		return commandDashboard, fmt.Errorf("unsupported arguments %q", args)
	}
}

func run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
) int {
	selectedCommand, parseErr := parseCommand(args)
	if parseErr != nil || selectedCommand == commandHelp || selectedCommand == commandVersion {
		return runWithDeps(ctx, args, stdout, stderr, runtimeDeps{})
	}

	paths, err := store.DefaultPaths()
	if err != nil {
		return reportError(stderr, fmt.Errorf("locate PRQ data files: %w", err))
	}
	deps, err := newRuntimeDeps(runtimeOptions{
		paths:  paths,
		input:  os.Stdin,
		output: stdout,
		goos:   runtime.GOOS,
	})
	if err != nil {
		return reportError(stderr, err)
	}
	return runWithDeps(ctx, args, stdout, stderr, deps)
}

func newRuntimeDeps(options runtimeOptions) (runtimeDeps, error) {
	configStore := store.ConfigStore{Path: options.paths.Config}
	cacheStore := store.CacheStore{Path: options.paths.Cache}

	runner, err := github.NewExecRunner()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return runtimeDeps{}, fmt.Errorf("%w: %v", errGitHubCLINotFound, err)
		}
		return runtimeDeps{}, err
	}
	gitHubClient := github.NewClient(runner)
	setupFlow := setup.Flow{
		GitHub: setupGitHub{Client: gitHubClient},
		Config: configStore,
		Cache:  cacheStore,
		Prompt: setup.HuhPrompter{},
	}
	service := app.Service{
		Reader: gitHubClient,
		Cache:  cacheStore,
		Now:    time.Now,
	}
	executor := platform.SystemExecutor{}
	clipboard := platform.Clipboard{Exec: executor, GOOS: options.goos}
	browser := platform.Browser{Exec: executor, GOOS: options.goos}

	runProgram := options.runProgram
	if runProgram == nil {
		runProgram = func(ctx context.Context, model tea.Model, input io.Reader, output io.Writer) error {
			_, err := tea.NewProgram(
				model,
				tea.WithContext(ctx),
				tea.WithInput(input),
				tea.WithOutput(output),
			).Run()
			return err
		}
	}

	return runtimeDeps{
		loadConfig: configStore.Load,
		loadCache:  cacheStore.Load,
		runSetup:   setupFlow.Run,
		runDashboard: func(
			ctx context.Context,
			selection app.Selection,
			cached *app.Snapshot,
			cacheWarning error,
		) error {
			model := tui.NewModel(
				ctx,
				selection,
				cached,
				cacheWarning,
				tui.Dependencies{
					Refresh: service.Refresh,
					Copy:    clipboard.Copy,
					Open:    browser.Open,
					Now:     time.Now,
				},
			)
			if err := runProgram(ctx, model, options.input, options.output); err != nil {
				return fmt.Errorf("run dashboard: %w", err)
			}
			return nil
		},
	}, nil
}

func runWithDeps(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps runtimeDeps,
) int {
	selectedCommand, err := parseCommand(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprint(stderr, usage)
		return 2
	}

	switch selectedCommand {
	case commandHelp:
		fmt.Fprint(stdout, usage)
		return 0
	case commandVersion:
		fmt.Fprintf(stdout, "prq %s\n", version)
		return 0
	case commandSetup:
		selection, err := deps.runSetup(ctx)
		if err != nil {
			return reportError(stderr, err)
		}
		if err := deps.runDashboard(ctx, selection, nil, nil); err != nil {
			return reportError(stderr, err)
		}
		return 0
	case commandDashboard:
		return runDashboardCommand(ctx, stderr, deps)
	default:
		return reportError(stderr, fmt.Errorf("unsupported command"))
	}
}

func runDashboardCommand(ctx context.Context, stderr io.Writer, deps runtimeDeps) int {
	selection, err := deps.loadConfig()
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			selection, err = deps.runSetup(ctx)
			if err != nil {
				return reportError(stderr, err)
			}
			if err := deps.runDashboard(ctx, selection, nil, nil); err != nil {
				return reportError(stderr, err)
			}
			return 0
		}
		if errors.Is(err, store.ErrInvalid) || errors.Is(err, store.ErrVersion) {
			fmt.Fprintf(stderr, "Invalid configuration: %v; run: prq setup\n", err)
			return 1
		}
		return reportError(stderr, err)
	}

	cached, cacheErr := deps.loadCache()
	var cachedSnapshot *app.Snapshot
	var cacheWarning error
	switch {
	case cacheErr == nil:
		cachedSnapshot = &cached
	case errors.Is(cacheErr, store.ErrNotFound):
	default:
		cacheWarning = cacheErr
	}

	if err := deps.runDashboard(ctx, selection, cachedSnapshot, cacheWarning); err != nil {
		return reportError(stderr, err)
	}
	return 0
}

func reportError(stderr io.Writer, err error) int {
	if errors.Is(err, errGitHubCLINotFound) {
		fmt.Fprintln(stderr, "GitHub CLI not found; install it with: brew install gh")
		return 1
	}
	if errors.Is(err, errGitHubAuthentication) {
		fmt.Fprintln(stderr, "GitHub authentication required; run: gh auth login")
		return 1
	}
	fmt.Fprintln(stderr, err)
	return 1
}

type setupGitHub struct {
	*github.Client
}

func (g setupGitHub) CheckAuth(ctx context.Context) error {
	if err := g.Client.CheckAuth(ctx); err != nil {
		return fmt.Errorf("%w: %v", errGitHubAuthentication, err)
	}
	return nil
}
