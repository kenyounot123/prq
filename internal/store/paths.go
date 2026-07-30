package store

import (
	"errors"
	"os"
	"path/filepath"
)

var (
	ErrNotFound          = errors.New("store file not found")
	ErrInvalid           = errors.New("store data is invalid")
	ErrVersion           = errors.New("store version is unsupported")
	ErrSelectionMismatch = errors.New("cache selection does not match configuration")
)

var (
	userConfigDir = os.UserConfigDir
	userCacheDir  = os.UserCacheDir
)

type Paths struct {
	Config string
	Cache  string
}

func DefaultPaths() (Paths, error) {
	configRoot, err := userConfigDir()
	if err != nil {
		return Paths{}, err
	}
	cacheRoot, err := userCacheDir()
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		Config: filepath.Join(configRoot, "prq", "config.json"),
		Cache:  filepath.Join(cacheRoot, "prq", "pull-requests.json"),
	}, nil
}
