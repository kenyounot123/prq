package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kenyounot123/prq/internal/app"
	"github.com/kenyounot123/prq/internal/store"
)

func TestParseCommandRoutesOnlySupportedForms(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    command
		wantErr bool
	}{
		{name: "dashboard", args: nil, want: commandDashboard},
		{name: "setup", args: []string{"setup"}, want: commandSetup},
		{name: "long help", args: []string{"--help"}, want: commandHelp},
		{name: "short help", args: []string{"-h"}, want: commandHelp},
		{name: "version", args: []string{"--version"}, want: commandVersion},
		{name: "unknown command", args: []string{"list"}, wantErr: true},
		{name: "extra setup argument", args: []string{"setup", "extra"}, wantErr: true},
		{name: "positional version", args: []string{"version"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCommand(test.args)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseCommand(%q) error = nil, want usage error", test.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCommand(%q) error = %v", test.args, err)
			}
			if got != test.want {
				t.Fatalf("parseCommand(%q) = %v, want %v", test.args, got, test.want)
			}
		})
	}
}

func TestRunWithDepsMissingConfigRunsSetupThenDashboard(t *testing.T) {
	selection := testSelection()
	var calls []string
	var gotCached *app.Snapshot
	var gotWarning error
	deps := runtimeDeps{
		loadConfig: func() (app.Selection, error) {
			calls = append(calls, "load config")
			return app.Selection{}, fmt.Errorf("read /tmp/config.json: %w", store.ErrNotFound)
		},
		loadCache: func(app.Selection) (app.Snapshot, error) {
			t.Fatal("missing configuration must not load the old cache")
			return app.Snapshot{}, nil
		},
		runSetup: func(context.Context) (app.Selection, error) {
			calls = append(calls, "setup")
			return selection, nil
		},
		runDashboard: func(_ context.Context, got app.Selection, cached *app.Snapshot, warning error) error {
			calls = append(calls, "dashboard")
			if !reflect.DeepEqual(got, selection) {
				t.Fatalf("dashboard selection = %#v, want %#v", got, selection)
			}
			gotCached = cached
			gotWarning = warning
			return nil
		},
	}
	var stdout, stderr bytes.Buffer

	exitCode := runWithDeps(t.Context(), nil, &stdout, &stderr, deps)

	if exitCode != 0 {
		t.Fatalf("runWithDeps() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !reflect.DeepEqual(calls, []string{"load config", "setup", "dashboard"}) {
		t.Fatalf("calls = %#v, want setup before dashboard", calls)
	}
	if gotCached != nil {
		t.Fatalf("dashboard cache = %#v, want nil after first-run setup", gotCached)
	}
	if gotWarning != nil {
		t.Fatalf("dashboard cache warning = %v, want nil", gotWarning)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q, want no command output", stdout.String(), stderr.String())
	}
}

func TestRunWithDepsInvalidConfigReportsPathAndSetupGuidance(t *testing.T) {
	const configPath = "/tmp/prq-config/config.json"
	deps := runtimeDeps{
		loadConfig: func() (app.Selection, error) {
			return app.Selection{}, fmt.Errorf("decode %s: %w", configPath, store.ErrInvalid)
		},
		runSetup: func(context.Context) (app.Selection, error) {
			t.Fatal("invalid configuration must not be overwritten by setup")
			return app.Selection{}, nil
		},
		runDashboard: func(context.Context, app.Selection, *app.Snapshot, error) error {
			t.Fatal("invalid configuration must not open the dashboard")
			return nil
		},
	}
	var stdout, stderr bytes.Buffer

	exitCode := runWithDeps(t.Context(), nil, &stdout, &stderr, deps)

	if exitCode == 0 {
		t.Fatal("runWithDeps() exit code = 0, want nonzero")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{configPath, "prq setup"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunWithDepsExplicitSetupAlwaysRunsSetupThenDashboard(t *testing.T) {
	selection := testSelection()
	var calls []string
	deps := runtimeDeps{
		loadConfig: func() (app.Selection, error) {
			t.Fatal("explicit setup must not load configuration first")
			return app.Selection{}, nil
		},
		loadCache: func(app.Selection) (app.Snapshot, error) {
			t.Fatal("explicit setup must not load the invalidated cache")
			return app.Snapshot{}, nil
		},
		runSetup: func(context.Context) (app.Selection, error) {
			calls = append(calls, "setup")
			return selection, nil
		},
		runDashboard: func(_ context.Context, got app.Selection, cached *app.Snapshot, warning error) error {
			calls = append(calls, "dashboard")
			if !reflect.DeepEqual(got, selection) {
				t.Fatalf("dashboard selection = %#v, want %#v", got, selection)
			}
			if cached != nil || warning != nil {
				t.Fatalf("dashboard cache = %#v, warning = %v, want nil after setup", cached, warning)
			}
			return nil
		},
	}
	var stdout, stderr bytes.Buffer

	exitCode := runWithDeps(t.Context(), []string{"setup"}, &stdout, &stderr, deps)

	if exitCode != 0 {
		t.Fatalf("runWithDeps() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !reflect.DeepEqual(calls, []string{"setup", "dashboard"}) {
		t.Fatalf("calls = %#v, want setup before dashboard", calls)
	}
}

func TestRunWithDepsValidConfigLoadsCacheBeforeDashboard(t *testing.T) {
	selection := testSelection()
	snapshot := testSnapshot()
	var calls []string
	deps := runtimeDeps{
		loadConfig: func() (app.Selection, error) {
			calls = append(calls, "load config")
			return selection, nil
		},
		loadCache: func(got app.Selection) (app.Snapshot, error) {
			calls = append(calls, "load cache")
			if !reflect.DeepEqual(got, selection) {
				t.Fatalf("cache selection = %#v, want %#v", got, selection)
			}
			return snapshot, nil
		},
		runSetup: func(context.Context) (app.Selection, error) {
			t.Fatal("valid configuration must not run setup")
			return app.Selection{}, nil
		},
		runDashboard: func(_ context.Context, got app.Selection, cached *app.Snapshot, warning error) error {
			calls = append(calls, "dashboard")
			if !reflect.DeepEqual(got, selection) {
				t.Fatalf("dashboard selection = %#v, want %#v", got, selection)
			}
			if cached == nil || !reflect.DeepEqual(*cached, snapshot) {
				t.Fatalf("dashboard cache = %#v, want %#v", cached, snapshot)
			}
			if warning != nil {
				t.Fatalf("dashboard cache warning = %v, want nil", warning)
			}
			return nil
		},
	}
	var stdout, stderr bytes.Buffer

	exitCode := runWithDeps(t.Context(), nil, &stdout, &stderr, deps)

	if exitCode != 0 {
		t.Fatalf("runWithDeps() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !reflect.DeepEqual(calls, []string{"load config", "load cache", "dashboard"}) {
		t.Fatalf("calls = %#v, want config then cache then dashboard", calls)
	}
}

func TestRunWithDepsMissingCacheStartsDashboardSilentlyWithoutCache(t *testing.T) {
	assertCacheLoadFailure(
		t,
		fmt.Errorf("read /tmp/cache.json: %w", store.ErrNotFound),
		false,
	)
}

func TestRunWithDepsCorruptCacheWarnsDashboardAndRefreshesWithoutCache(t *testing.T) {
	assertCacheLoadFailure(
		t,
		fmt.Errorf("decode /tmp/cache.json: %w", store.ErrInvalid),
		true,
	)
}

func TestRunWithDepsSelectionMismatchIgnoresCrashWindowCacheWithoutWarning(t *testing.T) {
	assertCacheLoadFailure(
		t,
		fmt.Errorf("load /tmp/cache.json: %w", store.ErrSelectionMismatch),
		false,
	)
}

func TestRunWithDepsHelpAndVersionWriteOnlyStdoutWithoutDependencies(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "help",
			args: []string{"--help"},
			want: "Usage:\n  prq\n  prq setup\n  prq export [--org ORG] [--account LOGIN]\n  prq --help\n  prq --version\n",
		},
		{
			name: "short help",
			args: []string{"-h"},
			want: "Usage:\n  prq\n  prq setup\n  prq export [--org ORG] [--account LOGIN]\n  prq --help\n  prq --version\n",
		},
		{name: "version", args: []string{"--version"}, want: "prq dev\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runWithDeps(
				t.Context(),
				test.args,
				&stdout,
				&stderr,
				panicRuntimeDeps(t),
			)

			if exitCode != 0 {
				t.Fatalf("runWithDeps() exit code = %d, want 0", exitCode)
			}
			if stdout.String() != test.want {
				t.Fatalf("stdout = %q, want %q", stdout.String(), test.want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRunWithDepsUsageAndOperationalErrorsWriteOnlyStderr(t *testing.T) {
	setupErr := errors.New("setup unavailable")
	dashboardErr := errors.New("dashboard unavailable")
	tests := []struct {
		name string
		args []string
		deps runtimeDeps
		want []string
	}{
		{
			name: "usage",
			args: []string{"list"},
			deps: panicRuntimeDeps(t),
			want: []string{"unsupported arguments", "list", "Usage:"},
		},
		{
			name: "setup",
			args: []string{"setup"},
			deps: runtimeDeps{
				runSetup: func(context.Context) (app.Selection, error) {
					return app.Selection{}, setupErr
				},
			},
			want: []string{"setup unavailable"},
		},
		{
			name: "dashboard",
			deps: runtimeDeps{
				loadConfig: func() (app.Selection, error) {
					return testSelection(), nil
				},
				loadCache: func(app.Selection) (app.Snapshot, error) {
					return app.Snapshot{}, store.ErrNotFound
				},
				runDashboard: func(context.Context, app.Selection, *app.Snapshot, error) error {
					return dashboardErr
				},
			},
			want: []string{"dashboard unavailable"},
		},
		{
			name: "authentication",
			args: []string{"setup"},
			deps: runtimeDeps{
				runSetup: func(context.Context) (app.Selection, error) {
					return app.Selection{}, errGitHubAuthentication
				},
			},
			want: []string{"GitHub authentication required; run: gh auth login"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runWithDeps(t.Context(), test.args, &stdout, &stderr, test.deps)

			if exitCode == 0 {
				t.Fatal("runWithDeps() exit code = 0, want nonzero")
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			for _, want := range test.want {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("stderr = %q, want %q", stderr.String(), want)
				}
			}
		})
	}
}

func TestCheckedInFakeGitHubSupportsReadOnlyAllowlist(t *testing.T) {
	fakeGitHub := filepath.Join(repositoryRoot(t), "testdata", "bin", "gh")
	tests := []struct {
		name       string
		args       []string
		stdin      string
		wantOutput string
		wantExit   int
	}{
		{
			name:     "authentication",
			args:     []string{"auth", "status", "--hostname", "github.com"},
			wantExit: 0,
		},
		{
			name: "viewer",
			args: []string{"api", "graphql", "--hostname", "github.com", "-f", "query=" + `query CurrentViewer {
  viewer { login }
}`},
			wantOutput: `"login": "OctoCat"`,
			wantExit:   0,
		},
		{
			name: "organizations",
			args: []string{"api", "graphql", "--hostname", "github.com", "-f", "query=" + `query ViewerOrganizations($cursor: String) {
  viewer {
    organizations(first: 100, after: $cursor) {
      nodes { login }
      pageInfo { hasNextPage endCursor }
    }
  }
}`},
			wantOutput: `"login": "Zoo-Systems"`,
			wantExit:   0,
		},
		{
			name: "repositories",
			args: []string{"api", "graphql", "--hostname", "github.com", "-f", "query=" + `query OrganizationRepositories($organization: String!, $cursor: String) {
  organization(login: $organization) {
    repositories(first: 100, after: $cursor, orderBy: {field: NAME, direction: ASC}) {
      nodes { nameWithOwner }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, "-f", "organization=Acme"},
			wantOutput: `"nameWithOwner": "Acme/Zulu"`,
			wantExit:   0,
		},
		{
			name: "pull requests",
			args: []string{"api", "graphql", "--hostname", "github.com", "-f", "query=" + `query OpenPullRequests($search: String!, $cursor: String) {
  search(query: $search, type: ISSUE, first: 100, after: $cursor) {
    nodes {
      ... on PullRequest {
        id
        number
        title
        body
        url
        isDraft
        headRefName
        createdAt
        updatedAt
        reviewDecision
        statusCheckRollup { state }
        repository { nameWithOwner }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}`, "-f", "search=is:pr is:open author:OctoCat org:Acme"},
			wantOutput: `"id": "PR_approved"`,
			wantExit:   0,
		},
		{
			name:     "authentication missing hostname",
			args:     []string{"auth", "status"},
			wantExit: 64,
		},
		{
			name:     "authentication with other hostname",
			args:     []string{"auth", "status", "--hostname", "enterprise.example"},
			wantExit: 64,
		},
		{
			name:     "API missing hostname",
			args:     []string{"api", "graphql", "-f", "query=query CurrentViewer { viewer { login } }"},
			wantExit: 64,
		},
		{
			name:     "API with other hostname",
			args:     []string{"api", "graphql", "--hostname", "enterprise.example", "-f", "query=query CurrentViewer { viewer { login } }"},
			wantExit: 64,
		},
		{
			name: "typed GraphQL variable",
			args: []string{"api", "graphql", "--hostname", "github.com", "-f", "query=" + `query ViewerOrganizations($cursor: String) {
  viewer {
    organizations(first: 100, after: $cursor) {
      nodes { login }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, "-F", "cursor=organizations-cursor-1"},
			wantExit: 64,
		},
		{
			name:     "unknown command",
			args:     []string{"pr", "list"},
			wantExit: 64,
		},
		{
			name:     "approved query with unrecognized flag",
			args:     []string{"api", "graphql", "--hostname", "CurrentViewer"},
			wantExit: 64,
		},
		{
			name:     "approved query with extra flag",
			args:     []string{"api", "graphql", "--hostname", "github.com", "-f", "query=query CurrentViewer { viewer { login } }", "--method", "POST"},
			wantExit: 64,
		},
		{
			name: "known operation name with altered document",
			args: []string{"api", "graphql", "--hostname", "github.com", "-f", "query=" + `query CurrentViewer {
  viewer { login name }
}`},
			wantExit: 64,
		},
		{
			name:     "mutation argument",
			args:     []string{"api", "graphql", "--hostname", "github.com", "-f", "query=mutation Merge { mergePullRequest(input: {}) { pullRequest { id } } }"},
			wantExit: 65,
		},
		{
			name:     "mutation stdin",
			args:     []string{"auth", "status", "--hostname", "github.com"},
			stdin:    "mutation",
			wantExit: 65,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, exitCode := runTestExecutable(t, fakeGitHub, test.args, test.stdin)
			if exitCode != test.wantExit {
				t.Fatalf("fake gh exit code = %d, want %d; output = %q", exitCode, test.wantExit, output)
			}
			if !strings.Contains(output, test.wantOutput) {
				t.Fatalf("fake gh output = %q, want %q", output, test.wantOutput)
			}
		})
	}
}

func TestCheckedInFakePlatformExecutablesCaptureExactInputs(t *testing.T) {
	captureDirectory := t.TempDir()
	t.Setenv("PRQ_TEST_CAPTURE_DIR", captureDirectory)
	fakeBin := filepath.Join(repositoryRoot(t), "testdata", "bin")

	clipboardText := "one\n• two & three\n"
	output, exitCode := runTestExecutable(
		t,
		filepath.Join(fakeBin, "pbcopy"),
		nil,
		clipboardText,
	)
	if exitCode != 0 {
		t.Fatalf("fake pbcopy exit code = %d, want 0; output = %q", exitCode, output)
	}
	clipboardCapture, err := os.ReadFile(filepath.Join(captureDirectory, "clipboard.txt"))
	if err != nil {
		t.Fatalf("read clipboard capture: %v", err)
	}
	if string(clipboardCapture) != clipboardText {
		t.Fatalf("clipboard capture = %q, want %q", clipboardCapture, clipboardText)
	}

	wantURLs := []string{
		"https://github.com/Acme/API/pull/17",
		"https://github.com/Acme/Web/pull/42",
	}
	output, exitCode = runTestExecutable(
		t,
		filepath.Join(fakeBin, "open"),
		wantURLs,
		"",
	)
	if exitCode != 0 {
		t.Fatalf("fake open exit code = %d, want 0; output = %q", exitCode, output)
	}
	openCapture, err := os.ReadFile(filepath.Join(captureDirectory, "open.txt"))
	if err != nil {
		t.Fatalf("read open capture: %v", err)
	}
	if string(openCapture) != strings.Join(wantURLs, "\n")+"\n" {
		t.Fatalf("open capture = %q, want one argument per line", openCapture)
	}
}

func TestRuntimeDepsWireConcretePackagesThroughFakeExecutables(t *testing.T) {
	root := repositoryRoot(t)
	fakeBin := filepath.Join(root, "testdata", "bin")
	captureDirectory := t.TempDir()
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PRQ_TEST_CAPTURE_DIR", captureDirectory)
	t.Setenv("GH_HOST", "enterprise.invalid")
	t.Setenv("PRQ_TEST_GH_UNRELATED_AUTH_ERROR", "expired enterprise account")

	storeRoot := t.TempDir()
	paths := store.Paths{
		Config: filepath.Join(storeRoot, "config", "config.json"),
		Cache:  filepath.Join(storeRoot, "cache", "pull-requests.json"),
	}
	selection := app.Selection{
		Organization: "Acme",
		Repositories: []string{"Acme/API", "Acme/Web"},
	}
	if err := (store.ConfigStore{Path: paths.Config}).Save(selection); err != nil {
		t.Fatalf("save integration config: %v", err)
	}

	programCalls := 0
	deps, err := newRuntimeDeps(runtimeOptions{
		paths:  paths,
		input:  strings.NewReader(""),
		output: io.Discard,
		goos:   "darwin",
		runProgram: func(_ context.Context, model tea.Model, _ io.Reader, _ io.Writer) error {
			programCalls++
			refresh := model.Init()
			if refresh == nil {
				t.Fatal("dashboard model did not start a refresh")
			}
			model, _ = model.Update(refresh())
			model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 32})

			model, action := model.Update(tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c'}))
			if action == nil {
				captured, _ := os.ReadFile(filepath.Join(captureDirectory, "gh.txt"))
				t.Fatalf(
					"selected-copy key did not return a command after refresh:\n%s\ncaptured gh:\n%s",
					model.View().Content,
					captured,
				)
			}
			model, _ = model.Update(action())

			_, action = model.Update(tea.KeyPressMsg(tea.Key{Text: "o", Code: 'o'}))
			if action == nil {
				t.Fatal("open key did not return a command")
			}
			action()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("newRuntimeDeps() error = %v", err)
	}
	var stdout, stderr bytes.Buffer

	exitCode := runWithDeps(t.Context(), nil, &stdout, &stderr, deps)

	if exitCode != 0 {
		t.Fatalf("runWithDeps() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if programCalls != 1 {
		t.Fatalf("dashboard program calls = %d, want 1", programCalls)
	}
	cached, err := (store.CacheStore{Path: paths.Cache}).Load(selection)
	if err != nil {
		t.Fatalf("load refreshed cache: %v", err)
	}
	if len(cached.PullRequests) != 4 {
		t.Fatalf("cached pull requests = %d, want 4 from fake GitHub pages", len(cached.PullRequests))
	}

	gitHubCapture, err := os.ReadFile(filepath.Join(captureDirectory, "gh.txt"))
	if err != nil {
		t.Fatalf("read GitHub capture: %v", err)
	}
	capturedCalls := string(gitHubCapture)
	for _, want := range []string{"CurrentViewer", "OpenPullRequests", "cursor=pull-requests-cursor-1"} {
		if !strings.Contains(capturedCalls, want) {
			t.Fatalf("GitHub capture missing %q:\n%s", want, capturedCalls)
		}
	}
	if got := strings.Count(capturedCalls, "\n--hostname\ngithub.com\n"); got != 4 {
		t.Fatalf("GitHub hostname pins = %d, want 4 API calls despite GH_HOST:\n%s", got, capturedCalls)
	}
	if strings.Contains(strings.ToLower(capturedCalls), "mutation") {
		t.Fatalf("GitHub capture contains a mutation:\n%s", capturedCalls)
	}
	if strings.Contains(capturedCalls, "\n-F\n") {
		t.Fatalf("GitHub capture contains typed GraphQL fields:\n%s", capturedCalls)
	}

	clipboardCapture, err := os.ReadFile(filepath.Join(captureDirectory, "clipboard.txt"))
	if err != nil {
		t.Fatalf("read clipboard capture: %v", err)
	}
	if string(clipboardCapture) != "https://github.com/Acme/API/pull/17" {
		t.Fatalf("clipboard capture = %q, want selected PR URL", clipboardCapture)
	}
	openCapture, err := os.ReadFile(filepath.Join(captureDirectory, "open.txt"))
	if err != nil {
		t.Fatalf("read open capture: %v", err)
	}
	if string(openCapture) != "https://github.com/Acme/API/pull/17\n" {
		t.Fatalf("open capture = %q, want one selected PR URL argument", openCapture)
	}
}

func TestRuntimeReportsMissingGitHubCLIWithInstallGuidance(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var stdout, stderr bytes.Buffer

	exitCode := run(t.Context(), nil, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatal("run() exit code = 0, want nonzero")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "GitHub CLI not found; install it with: brew install gh\n" {
		t.Fatalf("stderr = %q, want missing-gh guidance", stderr.String())
	}
}

func TestNewRuntimeDepsMissingGitHubPreservesPresentationAndLookupErrors(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := newRuntimeDeps(runtimeOptions{})

	if !errors.Is(err, errGitHubCLINotFound) {
		t.Fatalf("newRuntimeDeps() error = %v, want missing-CLI presentation sentinel", err)
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("newRuntimeDeps() error = %v, want exec.ErrNotFound cause", err)
	}
	var lookupError *exec.Error
	if !errors.As(err, &lookupError) {
		t.Fatalf("newRuntimeDeps() error = %v, want *exec.Error cause", err)
	}
}

func TestRuntimeReportsAuthenticationFailureWithLoginGuidance(t *testing.T) {
	root := repositoryRoot(t)
	fakeBin := filepath.Join(root, "testdata", "bin")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PRQ_TEST_CAPTURE_DIR", t.TempDir())
	t.Setenv("PRQ_TEST_GH_AUTH_ERROR", "not logged in")
	storeRoot := t.TempDir()
	paths := store.Paths{
		Config: filepath.Join(storeRoot, "config.json"),
		Cache:  filepath.Join(storeRoot, "cache.json"),
	}
	deps, err := newRuntimeDeps(runtimeOptions{
		paths:      paths,
		input:      strings.NewReader(""),
		output:     io.Discard,
		goos:       "darwin",
		runProgram: func(context.Context, tea.Model, io.Reader, io.Writer) error { return nil },
	})
	if err != nil {
		t.Fatalf("newRuntimeDeps() error = %v", err)
	}
	var stdout, stderr bytes.Buffer

	exitCode := runWithDeps(t.Context(), []string{"setup"}, &stdout, &stderr, deps)

	if exitCode == 0 {
		t.Fatal("runWithDeps() exit code = 0, want nonzero")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "GitHub authentication required; run: gh auth login\n" {
		t.Fatalf("stderr = %q, want authentication guidance", stderr.String())
	}
	if _, err := os.Stat(paths.Config); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config stat error = %v, want no configuration write", err)
	}
}

func TestRuntimeSetupAuthenticationPreservesPresentationAndCommandErrors(t *testing.T) {
	root := repositoryRoot(t)
	fakeBin := filepath.Join(root, "testdata", "bin")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PRQ_TEST_GH_AUTH_ERROR", "authentication expired")
	deps, err := newRuntimeDeps(runtimeOptions{goos: "darwin"})
	if err != nil {
		t.Fatalf("newRuntimeDeps() error = %v", err)
	}

	_, err = deps.runSetup(t.Context())

	if !errors.Is(err, errGitHubAuthentication) {
		t.Fatalf("runSetup() error = %v, want authentication presentation sentinel", err)
	}
	var commandError *exec.ExitError
	if !errors.As(err, &commandError) {
		t.Fatalf("runSetup() error = %v, want *exec.ExitError cause", err)
	}
	if commandError.ExitCode() != 1 {
		t.Fatalf("authentication exit code = %d, want 1", commandError.ExitCode())
	}
}

func TestRuntimeValidConfigSurfacesAuthenticationGuidanceOnInitialRefreshAndRetry(t *testing.T) {
	root := repositoryRoot(t)
	fakeBin := filepath.Join(root, "testdata", "bin")
	captureDirectory := t.TempDir()
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PRQ_TEST_CAPTURE_DIR", captureDirectory)
	t.Setenv("PRQ_TEST_GH_AUTH_ERROR", "authentication expired")

	storeRoot := t.TempDir()
	paths := store.Paths{
		Config: filepath.Join(storeRoot, "config.json"),
		Cache:  filepath.Join(storeRoot, "cache.json"),
	}
	selection := app.Selection{
		Organization: "Acme",
		Repositories: []string{"Acme/API"},
	}
	if err := (store.ConfigStore{Path: paths.Config}).Save(selection); err != nil {
		t.Fatalf("save integration config: %v", err)
	}
	cached := app.Snapshot{
		PullRequests: []app.PullRequest{{
			ID:         "PR_cached",
			Repository: "Acme/API",
			Number:     7,
			Title:      "Cached pull request remains visible",
			Body:       "Cached body.",
			URL:        "https://github.com/Acme/API/pull/7",
			CreatedAt:  time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
			UpdatedAt:  time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
			Review:     app.ReviewNone,
			Checks:     app.ChecksNone,
		}},
		FetchedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
	}
	if err := (store.CacheStore{Path: paths.Cache}).Save(selection, cached); err != nil {
		t.Fatalf("save integration cache: %v", err)
	}

	const guidance = "GitHub authentication required; run: gh auth login"
	programCalls := 0
	deps, err := newRuntimeDeps(runtimeOptions{
		paths:  paths,
		input:  strings.NewReader(""),
		output: io.Discard,
		goos:   "darwin",
		runProgram: func(_ context.Context, model tea.Model, _ io.Reader, _ io.Writer) error {
			programCalls++
			model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 32})
			if view := model.View().Content; !strings.Contains(view, "Cached pull request remains visible") {
				t.Fatalf("initial dashboard did not render cached data before refresh:\n%s", view)
			}

			refresh := model.Init()
			if refresh == nil {
				t.Fatal("dashboard model did not start the initial refresh")
			}
			model, _ = model.Update(refresh())
			if view := model.View().Content; !strings.Contains(view, guidance) {
				t.Fatalf("initial refresh view missing authentication guidance:\n%s", view)
			}

			var retry tea.Cmd
			model, retry = model.Update(tea.KeyPressMsg(tea.Key{Text: "r", Code: 'r'}))
			if retry == nil {
				t.Fatal("retry key did not return a refresh command")
			}
			model, _ = model.Update(retry())
			if view := model.View().Content; !strings.Contains(view, guidance) {
				t.Fatalf("retry view missing authentication guidance:\n%s", view)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("newRuntimeDeps() error = %v", err)
	}
	var stdout, stderr bytes.Buffer

	exitCode := runWithDeps(t.Context(), nil, &stdout, &stderr, deps)

	if exitCode != 0 {
		t.Fatalf("runWithDeps() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if programCalls != 1 {
		t.Fatalf("dashboard program calls = %d, want 1", programCalls)
	}
	captured, err := os.ReadFile(filepath.Join(captureDirectory, "gh.txt"))
	if err != nil {
		t.Fatalf("read GitHub capture: %v", err)
	}
	if got := strings.Count(string(captured), "---\nauth\nstatus\n--hostname\ngithub.com\n"); got != 2 {
		t.Fatalf("authentication checks = %d, want 2 initial/retry calls:\n%s", got, captured)
	}
	if strings.Contains(string(captured), "\napi\n") {
		t.Fatalf("expired authentication reached a GraphQL request:\n%s", captured)
	}
}

func TestBuiltExecutableSupportsHelpAndVersion(t *testing.T) {
	root := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "prq-test")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./cmd/prq")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build prq executable: %v\n%s", err, output)
	}

	tests := []struct {
		arg  string
		want string
	}{
		{arg: "--help", want: "Usage:\n  prq\n  prq setup\n  prq export [--org ORG] [--account LOGIN]\n  prq --help\n  prq --version\n"},
		{arg: "--version", want: "prq dev\n"},
	}
	for _, test := range tests {
		t.Run(test.arg, func(t *testing.T) {
			command := exec.CommandContext(t.Context(), binary, test.arg)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("%s error = %v; output = %q", test.arg, err, output)
			}
			if string(output) != test.want {
				t.Fatalf("%s output = %q, want %q", test.arg, output, test.want)
			}
		})
	}
}

func assertCacheLoadFailure(t *testing.T, cacheErr error, wantWarning bool) {
	t.Helper()
	selection := testSelection()
	deps := runtimeDeps{
		loadConfig: func() (app.Selection, error) {
			return selection, nil
		},
		loadCache: func(got app.Selection) (app.Snapshot, error) {
			if !reflect.DeepEqual(got, selection) {
				t.Fatalf("cache selection = %#v, want %#v", got, selection)
			}
			return app.Snapshot{}, cacheErr
		},
		runDashboard: func(_ context.Context, _ app.Selection, cached *app.Snapshot, warning error) error {
			if cached != nil {
				t.Fatalf("dashboard cache = %#v, want nil", cached)
			}
			if wantWarning && !errors.Is(warning, cacheErr) {
				t.Fatalf("dashboard warning = %v, want %v", warning, cacheErr)
			}
			if !wantWarning && warning != nil {
				t.Fatalf("dashboard warning = %v, want nil", warning)
			}
			return nil
		},
	}
	var stdout, stderr bytes.Buffer

	exitCode := runWithDeps(t.Context(), nil, &stdout, &stderr, deps)

	if exitCode != 0 {
		t.Fatalf("runWithDeps() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
}

func panicRuntimeDeps(t *testing.T) runtimeDeps {
	t.Helper()
	panicCall := func(name string) {
		t.Fatalf("%s called for output-only command", name)
	}
	return runtimeDeps{
		loadConfig: func() (app.Selection, error) {
			panicCall("loadConfig")
			return app.Selection{}, nil
		},
		loadCache: func(app.Selection) (app.Snapshot, error) {
			panicCall("loadCache")
			return app.Snapshot{}, nil
		},
		runSetup: func(context.Context) (app.Selection, error) {
			panicCall("runSetup")
			return app.Selection{}, nil
		},
		runDashboard: func(context.Context, app.Selection, *app.Snapshot, error) error {
			panicCall("runDashboard")
			return nil
		},
	}
}

func runTestExecutable(t *testing.T, path string, args []string, stdin string) (string, int) {
	t.Helper()
	command := exec.CommandContext(t.Context(), path, args...)
	command.Stdin = strings.NewReader(stdin)
	output, err := command.CombinedOutput()
	if err == nil {
		return string(output), 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return string(output), exitError.ExitCode()
	}
	t.Fatalf("run %s: %v", path, err)
	return "", -1
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	root := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("locate repository root from %s: %v", workingDirectory, err)
	}
	return root
}

func testSelection() app.Selection {
	return app.Selection{
		Organization: "Acme",
		Repositories: []string{"Acme/API"},
	}
}

func testSnapshot() app.Snapshot {
	return app.Snapshot{
		FetchedAt: time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC),
	}
}
