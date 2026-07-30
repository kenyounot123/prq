package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kenyounot123/prq/internal/app"
)

func TestDefaultPathsUsesPlatformRoots(t *testing.T) {
	previousConfigDir, previousCacheDir := userConfigDir, userCacheDir
	userConfigDir = func() (string, error) { return "/config-root", nil }
	userCacheDir = func() (string, error) { return "/cache-root", nil }
	t.Cleanup(func() {
		userConfigDir, userCacheDir = previousConfigDir, previousCacheDir
	})

	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if paths.Config != "/config-root/prq/config.json" {
		t.Fatalf("Config = %q", paths.Config)
	}
	if paths.Cache != "/cache-root/prq/pull-requests.json" {
		t.Fatalf("Cache = %q", paths.Cache)
	}
}

func TestConfigStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	s := ConfigStore{Path: path}
	want := app.Selection{
		Organization: "acme",
		Repositories: []string{"acme/api", "acme/web"},
	}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}

func TestConfigStoreLoadClassifiesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	_, err := (ConfigStore{Path: path}).Load()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load() error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("Load() error = %q, want path", err)
	}
}

func TestConfigStoreLoadClassifiesMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (ConfigStore{Path: path}).Load()
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Load() error = %v, want ErrInvalid", err)
	}
}

func TestConfigStoreLoadRejectsUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeJSONFixture(t, path, map[string]any{
		"version":      99,
		"organization": "acme",
		"repositories": []string{"acme/api"},
	})

	_, err := (ConfigStore{Path: path}).Load()
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("Load() error = %v, want ErrVersion", err)
	}
}

func TestConfigStoreLoadRejectsInvalidSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeJSONFixture(t, path, map[string]any{
		"version":      1,
		"organization": "acme",
		"repositories": []string{"other/api"},
	})

	_, err := (ConfigStore{Path: path}).Load()
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Load() error = %v, want ErrInvalid", err)
	}
}

func TestConfigStoreSaveRejectsInvalidSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	err := (ConfigStore{Path: path}).Save(app.Selection{
		Organization: "acme",
		Repositories: []string{"other/api"},
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Save() error = %v, want ErrInvalid", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("config stat error = %v, want not exist", statErr)
	}
}

func TestConfigStoreSavePreservesOldFileWhenRenameFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s := ConfigStore{Path: path}
	old := app.Selection{Organization: "acme", Repositories: []string{"acme/api"}}
	if err := s.Save(old); err != nil {
		t.Fatal(err)
	}

	previousRename := renameFile
	renameFile = func(string, string) error { return errors.New("rename blocked") }
	t.Cleanup(func() { renameFile = previousRename })
	if err := s.Save(app.Selection{Organization: "acme", Repositories: []string{"acme/web"}}); err == nil {
		t.Fatal("Save() error = nil, want temporary write failure")
	}

	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, old) {
		t.Fatalf("Load() = %#v, want preserved %#v", got, old)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Fatalf("directory entries = %#v, want only config.json", entries)
	}
}

func writeJSONFixture(t *testing.T, path string, value any) {
	t.Helper()
	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}
