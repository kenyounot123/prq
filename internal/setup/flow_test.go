package setup

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kenyounot123/prq/internal/app"
)

func TestFlowRunCoordinatesSetupInOrder(t *testing.T) {
	var calls []string
	github := fakeGitHub{
		checkAuth: func(context.Context) error {
			calls = append(calls, "CheckAuth")
			return nil
		},
		listOrganizations: func(context.Context) ([]string, error) {
			calls = append(calls, "ListOrganizations")
			return []string{"Acme", "Other"}, nil
		},
		listRepositories: func(_ context.Context, organization string) ([]string, error) {
			calls = append(calls, "ListRepositories:"+organization)
			return []string{"Acme/Web", "acme/api"}, nil
		},
	}
	prompt := fakePrompter{
		selectOrganization: func(_ context.Context, organizations []string) (string, error) {
			calls = append(calls, "SelectOrganization")
			if !reflect.DeepEqual(organizations, []string{"Acme", "Other"}) {
				t.Fatalf("organizations = %v", organizations)
			}
			return "Acme", nil
		},
		selectRepositories: func(_ context.Context, organization string, repositories []string) ([]string, error) {
			calls = append(calls, "SelectRepositories:"+organization)
			if !reflect.DeepEqual(repositories, []string{"Acme/Web", "acme/api"}) {
				t.Fatalf("repositories = %v", repositories)
			}
			return []string{"Acme/Web", "acme/api", "ACME/web"}, nil
		},
	}
	config := fakeConfigWriter{save: func(selection app.Selection) error {
		calls = append(calls, "Save")
		want := app.Selection{
			Organization: "Acme",
			Repositories: []string{"acme/api", "Acme/Web"},
		}
		if !reflect.DeepEqual(selection, want) {
			t.Fatalf("saved selection = %#v, want %#v", selection, want)
		}
		return nil
	}}
	cache := fakeCacheInvalidator{clear: func() error {
		calls = append(calls, "Clear")
		return nil
	}}

	got, err := (Flow{
		GitHub: github,
		Config: config,
		Cache:  cache,
		Prompt: prompt,
	}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	wantSelection := app.Selection{
		Organization: "Acme",
		Repositories: []string{"acme/api", "Acme/Web"},
	}
	if !reflect.DeepEqual(got, wantSelection) {
		t.Fatalf("Run() = %#v, want %#v", got, wantSelection)
	}
	wantCalls := []string{
		"CheckAuth",
		"ListOrganizations",
		"SelectOrganization",
		"ListRepositories:Acme",
		"SelectRepositories:Acme",
		"Save",
		"Clear",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
}

func TestFlowRunUsesCanonicalDiscoveryValuesForCaseInsensitivePromptSelections(t *testing.T) {
	var repositoryOrganization string
	var promptOrganization string
	var saved app.Selection

	got, err := (Flow{
		GitHub: fakeGitHub{
			checkAuth: func(context.Context) error { return nil },
			listOrganizations: func(context.Context) ([]string, error) {
				return []string{"Acme"}, nil
			},
			listRepositories: func(_ context.Context, organization string) ([]string, error) {
				repositoryOrganization = organization
				return []string{"Acme/API"}, nil
			},
		},
		Prompt: fakePrompter{
			selectOrganization: func(context.Context, []string) (string, error) {
				return "acme", nil
			},
			selectRepositories: func(
				_ context.Context,
				organization string,
				_ []string,
			) ([]string, error) {
				promptOrganization = organization
				return []string{"ACME/API"}, nil
			},
		},
		Config: fakeConfigWriter{save: func(selection app.Selection) error {
			saved = selection
			return nil
		}},
		Cache: fakeCacheInvalidator{clear: func() error { return nil }},
	}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := app.Selection{
		Organization: "Acme",
		Repositories: []string{"Acme/API"},
	}
	if repositoryOrganization != "Acme" {
		t.Fatalf("ListRepositories organization = %q, want canonical %q", repositoryOrganization, "Acme")
	}
	if promptOrganization != "Acme" {
		t.Fatalf("SelectRepositories organization = %q, want canonical %q", promptOrganization, "Acme")
	}
	if !reflect.DeepEqual(saved, want) {
		t.Fatalf("saved selection = %#v, want canonical %#v", saved, want)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Run() = %#v, want canonical %#v", got, want)
	}
}

func TestFlowRunRejectsEmptyDiscoveryResults(t *testing.T) {
	t.Run("organizations", func(t *testing.T) {
		var prompted bool
		_, err := (Flow{
			GitHub: fakeGitHub{
				checkAuth:         func(context.Context) error { return nil },
				listOrganizations: func(context.Context) ([]string, error) { return nil, nil },
			},
			Prompt: fakePrompter{
				selectOrganization: func(context.Context, []string) (string, error) {
					prompted = true
					return "", nil
				},
			},
		}).Run(context.Background())

		if err == nil || !strings.Contains(err.Error(), "no GitHub organizations") {
			t.Fatalf("Run() error = %v, want no-organizations error", err)
		}
		if prompted {
			t.Fatal("organization prompt ran with no organizations")
		}
	})

	t.Run("repositories", func(t *testing.T) {
		var prompted bool
		_, err := (Flow{
			GitHub: fakeGitHub{
				checkAuth:         func(context.Context) error { return nil },
				listOrganizations: func(context.Context) ([]string, error) { return []string{"Acme"}, nil },
				listRepositories:  func(context.Context, string) ([]string, error) { return nil, nil },
			},
			Prompt: fakePrompter{
				selectOrganization: func(context.Context, []string) (string, error) {
					return "Acme", nil
				},
				selectRepositories: func(context.Context, string, []string) ([]string, error) {
					prompted = true
					return nil, nil
				},
			},
		}).Run(context.Background())

		if err == nil || !strings.Contains(err.Error(), "no repositories") {
			t.Fatalf("Run() error = %v, want no-repositories error", err)
		}
		if prompted {
			t.Fatal("repository prompt ran with no repositories")
		}
	})
}

func TestFlowRunWrapsStageErrorsAndStops(t *testing.T) {
	errStage := errors.New("stage failed")
	tests := map[string]struct {
		flow Flow
		want string
	}{
		"authentication": {
			flow: Flow{
				GitHub: fakeGitHub{checkAuth: func(context.Context) error { return errStage }},
			},
			want: "check authentication",
		},
		"list organizations": {
			flow: Flow{
				GitHub: fakeGitHub{
					checkAuth:         func(context.Context) error { return nil },
					listOrganizations: func(context.Context) ([]string, error) { return nil, errStage },
				},
			},
			want: "list organizations",
		},
		"select organization": {
			flow: Flow{
				GitHub: fakeGitHub{
					checkAuth:         func(context.Context) error { return nil },
					listOrganizations: func(context.Context) ([]string, error) { return []string{"Acme"}, nil },
				},
				Prompt: fakePrompter{
					selectOrganization: func(context.Context, []string) (string, error) {
						return "", errStage
					},
				},
			},
			want: "select organization",
		},
		"list repositories": {
			flow: Flow{
				GitHub: fakeGitHub{
					checkAuth:         func(context.Context) error { return nil },
					listOrganizations: func(context.Context) ([]string, error) { return []string{"Acme"}, nil },
					listRepositories:  func(context.Context, string) ([]string, error) { return nil, errStage },
				},
				Prompt: selectingPrompter(),
			},
			want: "list repositories",
		},
		"select repositories": {
			flow: Flow{
				GitHub: successfulGitHub(),
				Prompt: fakePrompter{
					selectOrganization: func(context.Context, []string) (string, error) {
						return "Acme", nil
					},
					selectRepositories: func(context.Context, string, []string) ([]string, error) {
						return nil, errStage
					},
				},
			},
			want: "select repositories",
		},
		"save configuration": {
			flow: Flow{
				GitHub: successfulGitHub(),
				Prompt: selectingPrompter(),
				Config: fakeConfigWriter{save: func(app.Selection) error {
					return errStage
				}},
			},
			want: "save configuration",
		},
		"clear cache": {
			flow: Flow{
				GitHub: successfulGitHub(),
				Prompt: selectingPrompter(),
				Config: fakeConfigWriter{save: func(app.Selection) error {
					return nil
				}},
				Cache: fakeCacheInvalidator{clear: func() error {
					return errStage
				}},
			},
			want: "clear cache",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := tt.flow.Run(context.Background())
			if err == nil {
				t.Fatal("Run() error = nil")
			}
			if !errors.Is(err, errStage) {
				t.Fatalf("Run() error = %v, want wrapped stage error", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run() error = %q, want operation %q", err, tt.want)
			}
			if !reflect.DeepEqual(got, app.Selection{}) {
				t.Fatalf("Run() = %#v after failure, want zero selection", got)
			}
		})
	}
}

func TestFlowRunRejectsInvalidPromptOutputBeforeSaving(t *testing.T) {
	tests := map[string]fakePrompter{
		"unknown organization": {
			selectOrganization: func(context.Context, []string) (string, error) {
				return "Other", nil
			},
		},
		"unknown repository": {
			selectOrganization: func(context.Context, []string) (string, error) {
				return "Acme", nil
			},
			selectRepositories: func(context.Context, string, []string) ([]string, error) {
				return []string{"Acme/secret"}, nil
			},
		},
		"empty repository selection": {
			selectOrganization: func(context.Context, []string) (string, error) {
				return "Acme", nil
			},
			selectRepositories: func(context.Context, string, []string) ([]string, error) {
				return nil, nil
			},
		},
	}

	for name, prompt := range tests {
		t.Run(name, func(t *testing.T) {
			var saved, cleared bool
			_, err := (Flow{
				GitHub: successfulGitHub(),
				Prompt: prompt,
				Config: fakeConfigWriter{save: func(app.Selection) error {
					saved = true
					return nil
				}},
				Cache: fakeCacheInvalidator{clear: func() error {
					cleared = true
					return nil
				}},
			}).Run(context.Background())

			if err == nil || !strings.Contains(err.Error(), "validate selection") {
				t.Fatalf("Run() error = %v, want validation error", err)
			}
			if saved {
				t.Fatal("configuration saved after invalid prompt output")
			}
			if cleared {
				t.Fatal("cache cleared after invalid prompt output")
			}
		})
	}
}

func TestFlowRunDoesNotClearCacheWhenSaveFails(t *testing.T) {
	errSave := errors.New("disk full")
	var cleared bool
	_, err := (Flow{
		GitHub: successfulGitHub(),
		Prompt: selectingPrompter(),
		Config: fakeConfigWriter{save: func(app.Selection) error {
			return errSave
		}},
		Cache: fakeCacheInvalidator{clear: func() error {
			cleared = true
			return nil
		}},
	}).Run(context.Background())

	if !errors.Is(err, errSave) {
		t.Fatalf("Run() error = %v, want wrapped save error", err)
	}
	if cleared {
		t.Fatal("cache cleared before a successful save")
	}
}

type fakeGitHub struct {
	checkAuth         func(context.Context) error
	listOrganizations func(context.Context) ([]string, error)
	listRepositories  func(context.Context, string) ([]string, error)
}

func (f fakeGitHub) CheckAuth(ctx context.Context) error {
	return f.checkAuth(ctx)
}

func (f fakeGitHub) ListOrganizations(ctx context.Context) ([]string, error) {
	return f.listOrganizations(ctx)
}

func (f fakeGitHub) ListRepositories(ctx context.Context, organization string) ([]string, error) {
	return f.listRepositories(ctx, organization)
}

type fakeConfigWriter struct {
	save func(app.Selection) error
}

func (f fakeConfigWriter) Save(selection app.Selection) error {
	return f.save(selection)
}

type fakeCacheInvalidator struct {
	clear func() error
}

func (f fakeCacheInvalidator) Clear() error {
	return f.clear()
}

type fakePrompter struct {
	selectOrganization func(context.Context, []string) (string, error)
	selectRepositories func(context.Context, string, []string) ([]string, error)
}

func (f fakePrompter) SelectOrganization(ctx context.Context, organizations []string) (string, error) {
	return f.selectOrganization(ctx, organizations)
}

func (f fakePrompter) SelectRepositories(
	ctx context.Context,
	organization string,
	repositories []string,
) ([]string, error) {
	return f.selectRepositories(ctx, organization, repositories)
}

func successfulGitHub() fakeGitHub {
	return fakeGitHub{
		checkAuth:         func(context.Context) error { return nil },
		listOrganizations: func(context.Context) ([]string, error) { return []string{"Acme"}, nil },
		listRepositories: func(context.Context, string) ([]string, error) {
			return []string{"Acme/api"}, nil
		},
	}
}

func selectingPrompter() fakePrompter {
	return fakePrompter{
		selectOrganization: func(context.Context, []string) (string, error) {
			return "Acme", nil
		},
		selectRepositories: func(context.Context, string, []string) ([]string, error) {
			return []string{"Acme/api"}, nil
		},
	}
}
