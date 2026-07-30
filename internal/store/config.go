package store

import (
	"fmt"

	"github.com/kenyounot123/prq/internal/app"
)

const configVersion = 1

type ConfigStore struct {
	Path string
}

type configFile struct {
	Version int `json:"version"`
	app.Selection
}

func (s ConfigStore) Load() (app.Selection, error) {
	var stored configFile
	if err := readJSON(s.Path, &stored); err != nil {
		return app.Selection{}, err
	}
	if stored.Version != configVersion {
		return app.Selection{}, fmt.Errorf("load config %s: version %d: %w", s.Path, stored.Version, ErrVersion)
	}
	selection, err := app.NormalizeSelection(stored.Organization, stored.Repositories)
	if err != nil {
		return app.Selection{}, fmt.Errorf("load config %s: %w: %v", s.Path, ErrInvalid, err)
	}
	return selection, nil
}

func (s ConfigStore) Save(selection app.Selection) error {
	normalized, err := app.NormalizeSelection(selection.Organization, selection.Repositories)
	if err != nil {
		return fmt.Errorf("save config %s: %w: %v", s.Path, ErrInvalid, err)
	}
	if err := writeJSONAtomic(s.Path, configFile{Version: configVersion, Selection: normalized}); err != nil {
		return fmt.Errorf("save config %s: %w", s.Path, err)
	}
	return nil
}
