package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kenyounot123/prq/internal/app"
	"github.com/kenyounot123/prq/internal/store"
)

func TestParseCommandRoutesExport(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "bare", args: []string{"export"}},
		{name: "org and account", args: []string{"export", "--org", "acme", "--account", "octo"}},
		{name: "equals form", args: []string{"export", "--org=acme"}},
		{name: "unknown flag", args: []string{"export", "--bogus"}, wantErr: true},
		{name: "positional argument", args: []string{"export", "acme"}, wantErr: true},
		{name: "flag without value", args: []string{"export", "--org"}, wantErr: true},
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
			if err != nil || got != commandExport {
				t.Fatalf("parseCommand(%q) = %v, %v; want export", test.args, got, err)
			}
		})
	}
}

func TestParseExportOptionsTrimsValues(t *testing.T) {
	got, err := parseExportOptions([]string{"--org", " acme ", "--account", " octo "})
	if err != nil {
		t.Fatal(err)
	}
	if want := (exportOptions{Organization: "acme", Account: "octo"}); got != want {
		t.Fatalf("parseExportOptions = %+v, want %+v", got, want)
	}
}

type fakeExportReader struct {
	authErr       error
	repositories  map[string][]string
	pullRequests  []app.PullRequest
	listedOrg     string
	listedRepos   []string
	reposListedOf []string
}

func (f *fakeExportReader) CheckAuth(context.Context) error { return f.authErr }

func (f *fakeExportReader) ListRepositories(_ context.Context, org string) ([]string, error) {
	f.reposListedOf = append(f.reposListedOf, org)
	return f.repositories[org], nil
}

func (f *fakeExportReader) ListOpenPullRequests(_ context.Context, org string, repos []string) ([]app.PullRequest, error) {
	f.listedOrg, f.listedRepos = org, repos
	return f.pullRequests, nil
}

func exportTestDeps(reader *fakeExportReader, config app.Selection, configErr error, gotAccount *string) exportDeps {
	return exportDeps{
		loadConfig: func() (app.Selection, error) { return config, configErr },
		newReader: func(account string) (exportReader, error) {
			if gotAccount != nil {
				*gotAccount = account
			}
			return reader, nil
		},
		now: func() time.Time { return time.Date(2026, 11, 3, 12, 0, 0, 0, time.FixedZone("EST", -5*3600)) },
	}
}

func TestRunExportUsesConfiguredSelectionByDefault(t *testing.T) {
	older := app.PullRequest{Repository: "acme/api", Number: 1, Branch: "feat/ACME-1", UpdatedAt: time.Unix(100, 0).UTC()}
	newer := app.PullRequest{Repository: "acme/web", Number: 2, UpdatedAt: time.Unix(200, 0).UTC()}
	reader := &fakeExportReader{pullRequests: []app.PullRequest{newer, older}}
	config := app.Selection{Organization: "acme", Repositories: []string{"acme/web", "acme/api"}}

	var stdout bytes.Buffer
	if err := runExport(context.Background(), exportOptions{}, &stdout, exportTestDeps(reader, config, nil, nil)); err != nil {
		t.Fatal(err)
	}

	if reader.listedOrg != "acme" || !reflect.DeepEqual(reader.listedRepos, []string{"acme/api", "acme/web"}) {
		t.Fatalf("listed %q %q, want configured selection", reader.listedOrg, reader.listedRepos)
	}
	if len(reader.reposListedOf) != 0 {
		t.Fatalf("listed repositories of %q, want none for the configured organization", reader.reposListedOf)
	}
	var got exportDocument
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if got.Organization != "acme" || !got.FetchedAt.Equal(time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC)) {
		t.Fatalf("document header = %q %v", got.Organization, got.FetchedAt)
	}
	if len(got.PullRequests) != 2 || got.PullRequests[0].Number != 1 || got.PullRequests[0].Branch != "feat/ACME-1" {
		t.Fatalf("pull requests = %+v, want sorted with branch preserved", got.PullRequests)
	}
}

func TestRunExportOtherOrganizationListsItsRepositoriesWithAccount(t *testing.T) {
	reader := &fakeExportReader{repositories: map[string][]string{"other": {"other/one"}}}
	var account string
	var stdout bytes.Buffer
	options := exportOptions{Organization: "other", Account: "octo"}
	if err := runExport(context.Background(), options, &stdout, exportTestDeps(reader, app.Selection{}, store.ErrNotFound, &account)); err != nil {
		t.Fatal(err)
	}
	if account != "octo" {
		t.Fatalf("reader account = %q, want octo", account)
	}
	if reader.listedOrg != "other" || !reflect.DeepEqual(reader.listedRepos, []string{"other/one"}) {
		t.Fatalf("listed %q %q", reader.listedOrg, reader.listedRepos)
	}
	if !strings.Contains(stdout.String(), `"pull_requests": []`) {
		t.Fatalf("stdout = %s, want an empty pull request list rather than null", stdout.String())
	}
}

func TestRunExportWithoutConfigOrOrganizationGivesGuidance(t *testing.T) {
	err := runExport(context.Background(), exportOptions{}, &bytes.Buffer{}, exportTestDeps(&fakeExportReader{}, app.Selection{}, store.ErrNotFound, nil))
	if err == nil || !strings.Contains(err.Error(), "--org") || !strings.Contains(err.Error(), "prq setup") {
		t.Fatalf("error = %v, want guidance naming --org and prq setup", err)
	}
}

func TestRunExportAuthenticationFailurePresentsLoginGuidance(t *testing.T) {
	reader := &fakeExportReader{authErr: errors.New("not logged in")}
	err := runExport(context.Background(), exportOptions{}, &bytes.Buffer{}, exportTestDeps(reader, app.Selection{Organization: "acme", Repositories: []string{"acme/api"}}, nil, nil))
	if !errors.Is(err, errGitHubAuthentication) {
		t.Fatalf("error = %v, want authentication guidance", err)
	}
}
