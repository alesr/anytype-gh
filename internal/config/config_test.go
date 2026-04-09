package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		envContent  string
		yamlContent string
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
			name:        "loads values from yaml config",
			yamlContent: "github:\n  token: gh-yaml\nanytype:\n  base_url: http://localhost:31111\n",
			wantToken:   "gh-yaml",
			wantBaseURL: "http://localhost:31111",
		},
		{
			name:        "yaml values override dotenv values",
			envContent:  "GH_TOKEN=gh-dotenv\nANYTYPE_BASE_URL=http://localhost:39999",
			yamlContent: "github:\n  token: gh-yaml\nanytype:\n  base_url: http://localhost:32222\n",
			wantToken:   "gh-yaml",
			wantBaseURL: "http://localhost:32222",
		},
		{
			name:        "uses defaults when optional values missing",
			envContent:  "GH_TOKEN=gh-token",
			wantToken:   "gh-token",
			wantBaseURL: defaultAnytypeBaseURL,
		},
		{
			name:        "keeps existing env values over file",
			envContent:  "GH_TOKEN=dotenv-token",
			yamlContent: "github:\n  token: yaml-token\nanytype:\n  base_url: http://localhost:32222\n",
			preEnv: map[string]string{
				"GH_TOKEN":         "existing-token",
				"ANYTYPE_BASE_URL": "http://localhost:33333",
			},
			wantToken:   "existing-token",
			wantBaseURL: "http://localhost:33333",
		},
		{
			name:       "returns error on invalid line",
			envContent: "GH_TOKEN=\"unterminated",
			wantErrIs:  errOpenEnvFile,
		},
		{
			name:        "returns error on invalid yaml",
			yamlContent: "github:\n  token: [unterminated\n",
			wantErrIs:   errParseConfigFile,
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

			envPath := filepath.Join(tempDir, defaultEnvFileName)
			configPath := filepath.Join(tempDir, defaultConfigFileName)
			if tc.envContent != "" {
				err := os.WriteFile(envPath, []byte(tc.envContent), 0o600)
				require.NoError(t, err)
			}
			if tc.yamlContent != "" {
				err := os.WriteFile(configPath, []byte(tc.yamlContent), 0o600)
				require.NoError(t, err)
			}

			cfg, err := Load(envPath, configPath)
			if tc.wantErrIs != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErrIs)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantToken, cfg.GitHubToken)
			assert.Equal(t, tc.wantBaseURL, cfg.AnytypeBaseURL)
		})
	}
}

func TestLoad_DoesNotMutateProcessEnvFromFile(t *testing.T) {
	cleanupRuntimeEnv(t)

	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, defaultEnvFileName)

	err := os.WriteFile(envPath, []byte("GH_TOKEN=file-token\nANYTYPE_BASE_URL=http://localhost:39999"), 0o600)
	require.NoError(t, err)

	configPath := filepath.Join(tempDir, defaultConfigFileName)

	err = os.WriteFile(configPath, []byte("github:\n  token: yaml-token\n"), 0o600)
	require.NoError(t, err)

	cfg, err := Load(envPath, configPath)
	require.NoError(t, err)

	assert.Equal(t, "yaml-token", cfg.GitHubToken)
	assert.Equal(t, "http://localhost:39999", cfg.AnytypeBaseURL)

	_, exists := os.LookupEnv("GH_TOKEN")
	assert.False(t, exists)

	_, exists = os.LookupEnv("ANYTYPE_BASE_URL")
	assert.False(t, exists)
}

func TestSaveConfigFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, defaultConfigFileName)

	err := SaveConfigFile(configPath, Config{
		GitHubToken:    "gh-token",
		AnytypeBaseURL: "http://localhost:31009",
	})
	require.NoError(t, err)

	cfg, err := Load("", configPath)
	require.NoError(t, err)

	assert.Equal(t, "gh-token", cfg.GitHubToken)
	assert.Equal(t, "http://localhost:31009", cfg.AnytypeBaseURL)
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cfg       Config
		wantErr   bool
		wantErrIs error
	}{
		{
			name: "valid when GitHub token is set",
			cfg: Config{
				GitHubToken:    "gh-token",
				AnytypeBaseURL: defaultAnytypeBaseURL,
			},
			wantErr: false,
		},
		{
			name: "invalid when token is empty",
			cfg: Config{
				GitHubToken:    "",
				AnytypeBaseURL: defaultAnytypeBaseURL,
			},
			wantErr:   true,
			wantErrIs: errGitHubTokenMissing,
		},
		{
			name: "invalid when token is only spaces",
			cfg: Config{
				GitHubToken:    "   ",
				AnytypeBaseURL: defaultAnytypeBaseURL,
			},
			wantErr:   true,
			wantErrIs: errGitHubTokenMissing,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErrIs)
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

func TestResolveConfigFilePath(t *testing.T) {
	got := ResolveConfigFilePath()
	assert.NotEmpty(t, got)
	assert.True(t, strings.HasSuffix(got, filepath.Join(defaultConfigDirName, defaultConfigFileName)) || got == defaultConfigFileName)
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
