package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		envContent  string
		preEnv      map[string]string
		wantToken   string
		wantBaseURL string
		wantErrIs   error
	}{
		{
			name:        "loads values from env file",
			envContent:  "GH_TOKEN=gh-token\nANYTYPE_BASE_URL=http://localhost:39999",
			wantToken:   "gh-token",
			wantBaseURL: "http://localhost:39999",
		},
		{
			name:        "uses defaults when optional values missing",
			envContent:  "GH_TOKEN=gh-token",
			wantToken:   "gh-token",
			wantBaseURL: defaultAnytypeBaseURL,
		},
		{
			name:       "keeps existing env values over file",
			envContent: "GH_TOKEN=file-token",
			preEnv: map[string]string{
				"GH_TOKEN": "existing-token",
			},
			wantToken:   "existing-token",
			wantBaseURL: defaultAnytypeBaseURL,
		},
		{
			name:       "returns error on invalid line",
			envContent: "INVALID_LINE",
			wantErrIs:  errInvalidEnvLine,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanupRuntimeEnv(t)

			for key, value := range tc.preEnv {
				err := os.Setenv(key, value)
				require.NoError(t, err)
			}

			tempDir := t.TempDir()
			envPath := filepath.Join(tempDir, ".env.local")
			if tc.envContent != "" {
				err := os.WriteFile(envPath, []byte(tc.envContent), 0o600)
				require.NoError(t, err)
			}

			runtime, err := Load(envPath)
			if tc.wantErrIs != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErrIs)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantToken, runtime.GitHubToken)
			assert.Equal(t, tc.wantBaseURL, runtime.AnytypeBaseURL)
		})
	}
}

func TestLoad_DoesNotMutateProcessEnvFromFile(t *testing.T) {
	cleanupRuntimeEnv(t)

	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env.local")
	err := os.WriteFile(envPath, []byte("GH_TOKEN=file-token\nANYTYPE_BASE_URL=http://localhost:39999"), 0o600)
	require.NoError(t, err)

	runtime, err := Load(envPath)
	require.NoError(t, err)
	assert.Equal(t, "file-token", runtime.GitHubToken)
	assert.Equal(t, "http://localhost:39999", runtime.AnytypeBaseURL)

	_, exists := os.LookupEnv("GH_TOKEN")
	assert.False(t, exists)
	_, exists = os.LookupEnv("ANYTYPE_BASE_URL")
	assert.False(t, exists)
}

func TestTrimMatchingQuotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "double quotes", input: "\"value\"", want: "value"},
		{name: "single quotes", input: "'value'", want: "value"},
		{name: "mismatched quotes", input: "\"value'", want: "\"value'"},
		{name: "too short", input: "a", want: "a"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := trimMatchingQuotes(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveEnvFilePathWith(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		getwd         func() (string, error)
		userConfigDir func() (string, error)
		executable    func() (string, error)
		existing      map[string]struct{}
		want          string
	}{
		{
			name: "prefers current working directory",
			getwd: func() (string, error) {
				return "/tmp/project", nil
			},
			userConfigDir: func() (string, error) {
				return "/Users/me/.config", nil
			},
			executable: func() (string, error) {
				return "/usr/local/bin/anytype-gh", nil
			},
			existing: map[string]struct{}{
				filepath.Join("/tmp/project", ".env.local"):                    {},
				filepath.Join("/Users/me/.config", "anytype-gh", ".env.local"): {},
			},
			want: filepath.Join("/tmp/project", ".env.local"),
		},
		{
			name: "falls back to user config directory",
			getwd: func() (string, error) {
				return "/tmp/project", nil
			},
			userConfigDir: func() (string, error) {
				return "/Users/me/.config", nil
			},
			executable: func() (string, error) {
				return "/usr/local/bin/anytype-gh", nil
			},
			existing: map[string]struct{}{
				filepath.Join("/Users/me/.config", "anytype-gh", ".env.local"): {},
			},
			want: filepath.Join("/Users/me/.config", "anytype-gh", ".env.local"),
		},
		{
			name: "falls back to executable directory",
			getwd: func() (string, error) {
				return "/tmp/project", nil
			},
			userConfigDir: func() (string, error) {
				return "/Users/me/.config", nil
			},
			executable: func() (string, error) {
				return "/usr/local/bin/anytype-gh", nil
			},
			existing: map[string]struct{}{
				filepath.Join("/usr/local/bin", ".env.local"): {},
			},
			want: filepath.Join("/usr/local/bin", ".env.local"),
		},
		{
			name: "returns empty when no candidate exists",
			getwd: func() (string, error) {
				return "/tmp/project", nil
			},
			userConfigDir: func() (string, error) {
				return "/Users/me/.config", nil
			},
			executable: func() (string, error) {
				return "/usr/local/bin/anytype-gh", nil
			},
			existing: map[string]struct{}{},
			want:     "",
		},
		{
			name: "continues when providers fail",
			getwd: func() (string, error) {
				return "", errors.New("wd failed")
			},
			userConfigDir: func() (string, error) {
				return "", errors.New("config failed")
			},
			executable: func() (string, error) {
				return "/usr/local/bin/anytype-gh", nil
			},
			existing: map[string]struct{}{
				filepath.Join("/usr/local/bin", ".env.local"): {},
			},
			want: filepath.Join("/usr/local/bin", ".env.local"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := resolveEnvFilePathWith(tc.getwd, tc.userConfigDir, tc.executable, func(path string) bool {
				_, ok := tc.existing[path]
				return ok
			})

			assert.Equal(t, tc.want, got)
		})
	}
}

func cleanupRuntimeEnv(t *testing.T) {
	t.Helper()

	keys := []string{"GH_TOKEN", "ANYTYPE_BASE_URL"}
	original := make(map[string]*string, len(keys))

	for _, key := range keys {
		value, exists := os.LookupEnv(key)
		if exists {
			v := value
			original[key] = &v
		} else {
			original[key] = nil
		}

		err := os.Unsetenv(key)
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		for _, key := range keys {
			value := original[key]
			if value == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *value)
		}
	})
}
