package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type fakeRunner struct {
	outputs [][]byte
	errs    []error
	calls   [][]string
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, slices.Clone(args))
	i := len(f.calls) - 1
	return f.outputs[i], f.errs[i]
}

func TestCheckAuthInvokesGitHubAuthStatus(t *testing.T) {
	runner := &fakeRunner{
		outputs: [][]byte{nil},
		errs:    []error{nil},
	}

	if err := NewClient(runner).CheckAuth(context.Background()); err != nil {
		t.Fatalf("CheckAuth() error = %v", err)
	}

	want := [][]string{{"auth", "status"}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, want)
	}
}

func TestCurrentViewerReturnsFixtureLogin(t *testing.T) {
	runner := &fakeRunner{
		outputs: [][]byte{readFixture(t, "viewer.json")},
		errs:    []error{nil},
	}

	login, err := NewClient(runner).CurrentViewer(context.Background())
	if err != nil {
		t.Fatalf("CurrentViewer() error = %v", err)
	}
	if login != "OctoCat" {
		t.Fatalf("CurrentViewer() = %q, want %q", login, "OctoCat")
	}

	want := [][]string{{
		"api",
		"graphql",
		"-f",
		"query=" + currentViewerQuery,
	}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, want)
	}
}

func TestListOrganizationsPaginatesAndSortsCaseInsensitively(t *testing.T) {
	runner := &fakeRunner{
		outputs: [][]byte{
			readFixture(t, "organizations-page-1.json"),
			readFixture(t, "organizations-page-2.json"),
		},
		errs: []error{nil, nil},
	}

	organizations, err := NewClient(runner).ListOrganizations(context.Background())
	if err != nil {
		t.Fatalf("ListOrganizations() error = %v", err)
	}
	wantOrganizations := []string{"alpha-labs", "BetaWorks", "Zoo-Systems"}
	if !reflect.DeepEqual(organizations, wantOrganizations) {
		t.Fatalf("ListOrganizations() = %#v, want %#v", organizations, wantOrganizations)
	}

	wantCalls := [][]string{
		{
			"api",
			"graphql",
			"-f",
			"query=" + viewerOrganizationsQuery,
		},
		{
			"api",
			"graphql",
			"-f",
			"query=" + viewerOrganizationsQuery,
			"-F",
			"cursor=organizations-cursor-1",
		},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, wantCalls)
	}
}

func TestListRepositoriesPaginatesReturnsCanonicalNamesAndSortsCaseInsensitively(t *testing.T) {
	runner := &fakeRunner{
		outputs: [][]byte{
			readFixture(t, "repositories.json"),
			[]byte(`{
				"data": {
					"organization": {
						"repositories": {
							"nodes": [{"nameWithOwner": "Acme/Beta"}],
							"pageInfo": {"hasNextPage": false, "endCursor": null}
						}
					}
				}
			}`),
		},
		errs: []error{nil, nil},
	}

	repositories, err := NewClient(runner).ListRepositories(context.Background(), "Acme")
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}
	wantRepositories := []string{"acme/alpha", "Acme/Beta", "Acme/Zulu"}
	if !reflect.DeepEqual(repositories, wantRepositories) {
		t.Fatalf("ListRepositories() = %#v, want %#v", repositories, wantRepositories)
	}

	wantCalls := [][]string{
		{
			"api",
			"graphql",
			"-f",
			"query=" + organizationRepositoriesQuery,
			"-F",
			"organization=Acme",
		},
		{
			"api",
			"graphql",
			"-f",
			"query=" + organizationRepositoriesQuery,
			"-F",
			"organization=Acme",
			"-F",
			"cursor=repositories-cursor-1",
		},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("runner calls = %#v, want %#v", runner.calls, wantCalls)
	}
}

func TestListOrganizationsRejectsGraphQLErrorsWithOperationContext(t *testing.T) {
	runner := &fakeRunner{
		outputs: [][]byte{[]byte(`{
			"data": {
				"viewer": {
					"organizations": {
						"nodes": [{"login": "incomplete"}],
						"pageInfo": {"hasNextPage": false, "endCursor": null}
					}
				}
			},
			"errors": [{"message": "private response detail"}]
		}`)},
		errs: []error{nil},
	}

	_, err := NewClient(runner).ListOrganizations(context.Background())
	if err == nil {
		t.Fatal("ListOrganizations() error = nil, want GraphQL error")
	}
	if !strings.Contains(err.Error(), "list organizations") {
		t.Fatalf("ListOrganizations() error = %q, want operation context", err)
	}
}

func TestListOrganizationsRejectsMissingPaginationCursor(t *testing.T) {
	runner := &fakeRunner{
		outputs: [][]byte{[]byte(`{
			"data": {
				"viewer": {
					"organizations": {
						"nodes": [],
						"pageInfo": {"hasNextPage": true, "endCursor": null}
					}
				}
			}
		}`)},
		errs: []error{nil},
	}

	_, err := NewClient(runner).ListOrganizations(context.Background())
	if err == nil {
		t.Fatal("ListOrganizations() error = nil, want missing cursor error")
	}
	if !strings.Contains(err.Error(), "empty end cursor") {
		t.Fatalf("ListOrganizations() error = %q, want missing cursor context", err)
	}
}

func TestRunnerErrorDoesNotExposeResponseBody(t *testing.T) {
	const responseBody = `{"token":"response-body-must-not-appear"}`
	runner := &fakeRunner{
		outputs: [][]byte{[]byte(responseBody)},
		errs:    []error{errors.New("exit status 1")},
	}

	_, err := NewClient(runner).CurrentViewer(context.Background())
	if err == nil {
		t.Fatal("CurrentViewer() error = nil, want runner error")
	}
	if strings.Contains(err.Error(), responseBody) {
		t.Fatalf("CurrentViewer() error exposed response body: %q", err)
	}
	if !strings.Contains(err.Error(), "get current viewer") {
		t.Fatalf("CurrentViewer() error = %q, want operation context", err)
	}
}

func TestExecRunnerErrorContainsStderrButNotStdout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake-gh")
	script := "#!/bin/sh\n" +
		"printf '{\"token\":\"stdout-body\"}'\n" +
		"printf 'concise stderr\\n' >&2\n" +
		"exit 7\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	runner := &ExecRunner{Path: path}

	_, err := runner.Run(context.Background(), "auth", "status")
	if err == nil {
		t.Fatal("Run() error = nil, want command error")
	}
	if strings.Contains(err.Error(), "stdout-body") {
		t.Fatalf("Run() error exposed stdout: %q", err)
	}
	if !strings.Contains(err.Error(), "concise stderr") {
		t.Fatalf("Run() error = %q, want stderr", err)
	}
}

func TestNewExecRunnerLocatesGitHubCLIOnPath(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", directory)

	runner, err := NewExecRunner()
	if err != nil {
		t.Fatalf("NewExecRunner() error = %v", err)
	}
	if runner.Path != path {
		t.Fatalf("NewExecRunner().Path = %q, want %q", runner.Path, path)
	}
}

func TestNewExecRunnerReportsMissingGitHubCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	runner, err := NewExecRunner()
	if err == nil {
		t.Fatal("NewExecRunner() error = nil, want missing gh error")
	}
	if runner != nil {
		t.Fatalf("NewExecRunner() runner = %#v, want nil", runner)
	}
	if !strings.Contains(err.Error(), "locate gh") {
		t.Fatalf("NewExecRunner() error = %q, want lookup context", err)
	}
}

func TestExecRunnerRejectsMutationBeforeExecution(t *testing.T) {
	runner := &ExecRunner{Path: filepath.Join(t.TempDir(), "gh-must-not-run")}
	document := `query SafeLooking { viewer { login } } mutation WriteThing { addComment(input: {}) { clientMutationId } }`

	_, err := runner.Run(
		context.Background(),
		"api",
		"graphql",
		"-f",
		"query="+document,
	)
	if err == nil {
		t.Fatal("Run() error = nil, want mutation rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "mutation") {
		t.Fatalf("Run() error = %q, want mutation rejection", err)
	}
	if strings.Contains(err.Error(), "no such file") {
		t.Fatalf("Run() tried to execute gh before rejecting mutation: %q", err)
	}
}

func TestExecRunnerRejectsUnknownGraphQLDocumentBeforeExecution(t *testing.T) {
	runner := &ExecRunner{Path: filepath.Join(t.TempDir(), "gh-must-not-run")}
	document := `query UnapprovedQuery { viewer { login } }`

	_, err := runner.Run(
		context.Background(),
		"api",
		"graphql",
		"-f",
		"query="+document,
	)
	if err == nil {
		t.Fatal("Run() error = nil, want unknown document rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "fixed") {
		t.Fatalf("Run() error = %q, want fixed-document rejection", err)
	}
	if strings.Contains(err.Error(), "no such file") {
		t.Fatalf("Run() tried to execute gh before rejecting document: %q", err)
	}
}

func TestExecRunnerRejectsUnapprovedCommandBeforeExecution(t *testing.T) {
	runner := &ExecRunner{Path: filepath.Join(t.TempDir(), "gh-must-not-run")}

	_, err := runner.Run(context.Background(), "pr", "merge", "123")
	if err == nil {
		t.Fatal("Run() error = nil, want command rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "read-only") {
		t.Fatalf("Run() error = %q, want read-only command rejection", err)
	}
	if strings.Contains(err.Error(), "no such file") {
		t.Fatalf("Run() tried to execute unapproved command: %q", err)
	}
}

func TestAllGraphQLDocumentsAreReadOnly(t *testing.T) {
	wantNames := []string{
		"current viewer",
		"open pull requests",
		"organization repositories",
		"viewer organizations",
	}
	gotNames := make([]string, 0, len(allQueries))
	for name := range allQueries {
		gotNames = append(gotNames, name)
	}
	slices.Sort(gotNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("allQueries names = %#v, want %#v", gotNames, wantNames)
	}

	mutationToken := regexp.MustCompile(`(?i)\bmutation\b`)
	for name, document := range allQueries {
		normalized := strings.TrimSpace(document)
		if !strings.HasPrefix(normalized, "query ") {
			t.Errorf("%s must start with a named query", name)
		}
		if mutationToken.MatchString(normalized) {
			t.Errorf("%s contains a mutation operation", name)
		}
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return data
}
