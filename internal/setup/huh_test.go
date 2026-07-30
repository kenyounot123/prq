package setup

import (
	"context"
	"errors"
	"testing"

	"charm.land/huh/v2"
)

func TestHuhPrompterImplementsPrompter(t *testing.T) {
	var _ Prompter = HuhPrompter{}
}

func TestHuhPrompterRunsOrganizationFormWithContext(t *testing.T) {
	errCanceled := errors.New("form canceled")
	ctx := context.Background()
	var gotContext context.Context
	prompt := HuhPrompter{
		runForm: func(runContext context.Context, form *huh.Form) error {
			gotContext = runContext
			if form == nil {
				t.Fatal("organization form is nil")
			}
			return errCanceled
		},
	}

	_, err := prompt.SelectOrganization(ctx, []string{"Acme"})

	if !errors.Is(err, errCanceled) {
		t.Fatalf("SelectOrganization() error = %v, want cancellation", err)
	}
	if gotContext != ctx {
		t.Fatal("SelectOrganization() did not pass its context to the form")
	}
}

func TestHuhPrompterRunsRepositoryFormWithContext(t *testing.T) {
	errCanceled := errors.New("form canceled")
	ctx := context.Background()
	var gotContext context.Context
	prompt := HuhPrompter{
		runForm: func(runContext context.Context, form *huh.Form) error {
			gotContext = runContext
			if form == nil {
				t.Fatal("repository form is nil")
			}
			return errCanceled
		},
	}

	_, err := prompt.SelectRepositories(ctx, "Acme", []string{"Acme/api"})

	if !errors.Is(err, errCanceled) {
		t.Fatalf("SelectRepositories() error = %v, want cancellation", err)
	}
	if gotContext != ctx {
		t.Fatal("SelectRepositories() did not pass its context to the form")
	}
}
