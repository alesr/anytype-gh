package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultPath(t *testing.T) {
	t.Parallel()

	path, err := DefaultPath()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, filepath.Join(appDirName, stateFile)))
}

func TestFileStore_Load(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		prepare       func(t *testing.T, path string)
		wantErrIs     error
		wantSchema    int
		wantRepoCount int
	}{
		{
			name:          "returns default state when file missing",
			wantSchema:    StateSchemaVersion,
			wantRepoCount: 0,
		},
		{
			name: "returns error for invalid json",
			prepare: func(t *testing.T, path string) {
				t.Helper()
				err := os.WriteFile(path, []byte("{invalid"), 0o600)
				require.NoError(t, err)
			},
			wantErrIs: errDecodeStateFile,
		},
		{
			name: "loads and normalizes stored state",
			prepare: func(t *testing.T, path string) {
				t.Helper()
				payload := `{"schema_version":0,"anytype_app_key":"app-key","default_space":"space-1"}`
				err := os.WriteFile(path, []byte(payload), 0o600)
				require.NoError(t, err)
			},
			wantSchema:    StateSchemaVersion,
			wantRepoCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), stateFile)
			if tc.prepare != nil {
				tc.prepare(t, path)
			}

			store := NewFileStore(path)
			state, err := store.Load(context.Background())

			if tc.wantErrIs != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErrIs)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantSchema, state.SchemaVersion)
			assert.Len(t, state.Repositories, tc.wantRepoCount)
		})
	}
}

func TestFileStore_Save(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "nested", stateFile)
	store := NewFileStore(statePath)

	state := NewAppState()
	state.AnytypeAppKey = "app-key"
	state.DefaultSpace = "space-1"
	state.Repositories["octo/private-repo"] = RepositoryState{ObjectID: "obj-1", LastReadmeSHA: "sha-1"}

	err := store.Save(context.Background(), state)
	require.NoError(t, err)

	loaded, err := store.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "app-key", loaded.AnytypeAppKey)
	assert.Equal(t, "space-1", loaded.DefaultSpace)
	assert.Equal(t, "obj-1", loaded.Repositories["octo/private-repo"].ObjectID)

	info, err := os.Stat(statePath)
	require.NoError(t, err)

	assert.EqualValues(t, fileMode, info.Mode().Perm())
}
