package github

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
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

	want := [][]string{{"auth", "status", "--hostname", "github.com"}}
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
		"--hostname",
		"github.com",
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
			"--hostname",
			"github.com",
			"-f",
			"query=" + viewerOrganizationsQuery,
		},
		{
			"api",
			"graphql",
			"--hostname",
			"github.com",
			"-f",
			"query=" + viewerOrganizationsQuery,
			"-f",
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
			"--hostname",
			"github.com",
			"-f",
			"query=" + organizationRepositoriesQuery,
			"-f",
			"organization=Acme",
		},
		{
			"api",
			"graphql",
			"--hostname",
			"github.com",
			"-f",
			"query=" + organizationRepositoriesQuery,
			"-f",
			"organization=Acme",
			"-f",
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

func TestListOrganizationsPreservesOpaqueCursorAsRawString(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "cursor.txt")
	if err := os.WriteFile(filePath, []byte("file contents must not become a cursor"), 0o600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"at file":  "@" + filePath,
		"at stdin": "@-",
		"template": "{owner}",
		"boolean":  "true",
		"integer":  "123",
		"null":     "null",
	}

	for name, cursor := range values {
		t.Run(name, func(t *testing.T) {
			firstPage := []byte(`{
				"data": {
					"viewer": {
						"organizations": {
							"nodes": [],
							"pageInfo": {"hasNextPage": true, "endCursor": ` + strconv.Quote(cursor) + `}
						}
					}
				}
			}`)
			runner := &fakeRunner{
				outputs: [][]byte{
					firstPage,
					[]byte(`{
						"data": {
							"viewer": {
								"organizations": {
									"nodes": [],
									"pageInfo": {"hasNextPage": false, "endCursor": null}
								}
							}
						}
					}`),
				},
				errs: []error{nil, nil},
			}

			if _, err := NewClient(runner).ListOrganizations(context.Background()); err != nil {
				t.Fatal(err)
			}
			got := runner.calls[1][len(runner.calls[1])-2:]
			want := []string{"-f", "cursor=" + cursor}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("second-page raw field = %#v, want byte-for-byte %#v", got, want)
			}
		})
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

	_, err := runner.Run(context.Background(), "auth", "status", "--hostname", "github.com")
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
		"--hostname",
		"github.com",
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
		"--hostname",
		"github.com",
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

func TestExecRunnerRejectsMissingOrUnexpectedGitHubHostnameBeforeExecution(t *testing.T) {
	runner := &ExecRunner{Path: filepath.Join(t.TempDir(), "gh-must-not-run")}
	tests := map[string][]string{
		"auth missing hostname": {"auth", "status"},
		"auth different host":   {"auth", "status", "--hostname", "enterprise.example"},
		"API missing hostname": {
			"api", "graphql",
			"-f", "query=" + currentViewerQuery,
		},
		"API different host": {
			"api", "graphql",
			"--hostname", "enterprise.example",
			"-f", "query=" + currentViewerQuery,
		},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := runner.Run(context.Background(), args...)
			if err == nil {
				t.Fatal("Run() error = nil, want hostname rejection")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "github.com") {
				t.Fatalf("Run() error = %q, want github.com requirement", err)
			}
			if strings.Contains(err.Error(), "no such file") {
				t.Fatalf("Run() executed gh before hostname rejection: %q", err)
			}
		})
	}
}

func TestExecRunnerPreservesRawGraphQLVariablesAtProcessBoundary(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "fake-gh")
	capturePath := filepath.Join(directory, "argv")
	filePath := filepath.Join(directory, "cursor.txt")
	if err := os.WriteFile(filePath, []byte("file contents must not become a cursor"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"set -eu\n" +
		": >\"$PRQ_ARGV_CAPTURE\"\n" +
		"for argument in \"$@\"; do printf '%s\\0' \"$argument\" >>\"$PRQ_ARGV_CAPTURE\"; done\n" +
		"printf '%s\\n' '{\"data\":{\"viewer\":{\"organizations\":{\"nodes\":[],\"pageInfo\":{\"hasNextPage\":false,\"endCursor\":null}}}}}'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRQ_ARGV_CAPTURE", capturePath)

	values := []string{"@" + filePath, "@-", "{owner}", "true", "123", "null"}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			args := []string{
				"api", "graphql",
				"--hostname", "github.com",
				"-f", "query=" + viewerOrganizationsQuery,
				"-f", "cursor=" + value,
			}
			if _, err := (&ExecRunner{Path: path}).Run(context.Background(), args...); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			captured, err := os.ReadFile(capturePath)
			if err != nil {
				t.Fatal(err)
			}
			fields := bytes.Split(bytes.TrimSuffix(captured, []byte{0}), []byte{0})
			got := make([]string, len(fields))
			for index := range fields {
				got[index] = string(fields[index])
			}
			if !reflect.DeepEqual(got, args) {
				t.Fatalf("process argv = %#v, want byte-for-byte %#v", got, args)
			}
			if bytes.Contains(captured, []byte("file contents must not become a cursor")) {
				t.Fatalf("process argv contains contents read from @file: %q", captured)
			}
		})
	}
}

func TestExecRunnerRejectsTypedGraphQLFieldsAndMissingRequiredVariables(t *testing.T) {
	runner := &ExecRunner{Path: filepath.Join(t.TempDir(), "gh-must-not-run")}
	tests := map[string]struct {
		args []string
		want string
	}{
		"typed cursor": {
			args: []string{
				"api", "graphql",
				"--hostname", "github.com",
				"-f", "query=" + viewerOrganizationsQuery,
				"-F", "cursor=@-",
			},
			want: "raw",
		},
		"missing organization": {
			args: []string{
				"api", "graphql",
				"--hostname", "github.com",
				"-f", "query=" + organizationRepositoriesQuery,
			},
			want: "organization",
		},
		"missing search": {
			args: []string{
				"api", "graphql",
				"--hostname", "github.com",
				"-f", "query=" + openPullRequestsQuery,
			},
			want: "search",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := runner.Run(context.Background(), test.args...)
			if err == nil {
				t.Fatal("Run() error = nil, want variable rejection")
			}
			if !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("Run() error = %q, want %q context", err, test.want)
			}
			if strings.Contains(err.Error(), "no such file") {
				t.Fatalf("Run() executed gh before variable rejection: %q", err)
			}
		})
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
