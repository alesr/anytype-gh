// Package state persists local sync decisions so repeated runs can update the
// same Anytype objects instead of duplicating content.
package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	appDirName  = "anytype-gh"
	stateFile   = "state.json"
	fileMode    = 0o600
	dirMode     = 0o700
	tempPattern = "state-*.tmp"
)

var (
	errResolveUserConfigDir     = errors.New("resolve user config dir")
	errReadStateFile            = errors.New("read state file")
	errDecodeStateFile          = errors.New("decode state file")
	errCreateStateDirectory     = errors.New("create state directory")
	errEncodeStateFile          = errors.New("encode state file")
	errCreateTemporaryStateFile = errors.New("create temporary state file")
	errChmodTemporaryStateFile  = errors.New("chmod temporary state file")
	errWriteTemporaryStateFile  = errors.New("write temporary state file")
	errCloseTemporaryStateFile  = errors.New("close temporary state file")
	errReplaceStateFile         = errors.New("replace state file")
	errChmodStateFile           = errors.New("chmod state file")
)

type fileStore struct {
	path string
}

// NewFileStore exists to keep persistence swappable in tests and future
// backends while defaulting to the simplest local option.
func NewFileStore(path string) *fileStore { return &fileStore{path: path} }

// DefaultPath keeps state under the user config directory so credentials and
// sync metadata are user-scoped instead of repo-scoped.
func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("%w: %w", errResolveUserConfigDir, err)
	}
	return filepath.Join(configDir, appDirName, stateFile), nil
}

// Path helps callers communicate where file store state is written.
func (s *fileStore) Path() string { return s.path }

// Load treats missing state as first-run to avoid forcing setup files before
// the user can authenticate.
func (s *fileStore) Load(_ context.Context) (*AppState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewAppState(), nil
		}
		return nil, fmt.Errorf("%w: %w", errReadStateFile, err)
	}

	state := NewAppState()
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("%w: %w", errDecodeStateFile, err)
	}

	state.EnsureDefaults()
	return state, nil
}

// Save uses atomic replacement to avoid partial writes corrupting state after
// interruptions or crashes.
func (s *fileStore) Save(_ context.Context, state *AppState) error {
	state.EnsureDefaults()

	if err := os.MkdirAll(filepath.Dir(s.path), dirMode); err != nil {
		return fmt.Errorf("%w: %w", errCreateStateDirectory, err)
	}

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", errEncodeStateFile, err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(s.path), tempPattern)
	if err != nil {
		return fmt.Errorf("%w: %w", errCreateTemporaryStateFile, err)
	}

	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := tmpFile.Chmod(fileMode); err != nil {
		tmpFile.Close()
		return fmt.Errorf("%w: %w", errChmodTemporaryStateFile, err)
	}

	if _, err := tmpFile.Write(payload); err != nil {
		tmpFile.Close()
		return fmt.Errorf("%w: %w", errWriteTemporaryStateFile, err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("%w: %w", errCloseTemporaryStateFile, err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("%w: %w", errReplaceStateFile, err)
	}

	if err := os.Chmod(s.path, fileMode); err != nil {
		return fmt.Errorf("%w: %w", errChmodStateFile, err)
	}
	return nil
}
