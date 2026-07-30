package setup

import (
	"context"
	"errors"

	"charm.land/huh/v2"
)

type HuhPrompter struct {
	runForm func(context.Context, *huh.Form) error
}

func (p HuhPrompter) SelectOrganization(
	ctx context.Context,
	organizations []string,
) (string, error) {
	var selected string
	field := huh.NewSelect[string]().
		Title("Choose a GitHub organization").
		Options(huh.NewOptions(organizations...)...).
		Value(&selected)
	form := huh.NewForm(huh.NewGroup(field))

	if err := p.run(ctx, form); err != nil {
		return "", err
	}
	return selected, nil
}

func (p HuhPrompter) SelectRepositories(
	ctx context.Context,
	_ string,
	repositories []string,
) ([]string, error) {
	var selected []string
	field := huh.NewMultiSelect[string]().
		Title("Choose repositories").
		Description("Press / to filter; space toggles a repository").
		Options(huh.NewOptions(repositories...)...).
		Validate(func(values []string) error {
			if len(values) == 0 {
				return errors.New("select at least one repository")
			}
			return nil
		}).
		Value(&selected)
	form := huh.NewForm(huh.NewGroup(field))

	if err := p.run(ctx, form); err != nil {
		return nil, err
	}
	return selected, nil
}

func (p HuhPrompter) run(ctx context.Context, form *huh.Form) error {
	if p.runForm != nil {
		return p.runForm(ctx, form)
	}
	return form.RunWithContext(ctx)
}
