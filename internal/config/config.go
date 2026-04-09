// Package config centralizes runtime configuration rules so startup behavior
// stays predictable across local runs and installed binaries.
package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

const (
	defaultAnytypeBaseURL = "http://localhost:31009"
	defaultEnvFileName    = ".env.local"
)

var (
	errParseEnv           = errors.New("parse env")
	errOpenEnvFile        = errors.New("open env file")
	errGitHubTokenMissing = errors.New("missing GitHub token (GH_TOKEN)")
)

// Config hold configuration values used throughout the application.
type Config struct {
	GitHubToken    string `env:"GH_TOKEN"`
	AnytypeBaseURL string `env:"ANYTYPE_BASE_URL"`
}

// Validate fails fast on required values so startup errors are immediate and
// explicit instead of surfacing later in downstream clients.
func (c Config) Validate() error {
	if strings.TrimSpace(c.GitHubToken) == "" {
		return errGitHubTokenMissing
	}
	return nil
}

// Load resolves config with process env taking precedence over dotenv
// values, so explicit shell settings always win.
func Load(envFile string) (Config, error) {
	if envFile == "" {
		envFile = defaultEnvFileName
	}

	fileEnv, err := loadDotEnv(envFile)
	if err != nil {
		return Config{}, err
	}

	opts := env.Options{
		Environment: mergedEnv(fileEnv, os.Environ()),
	}

	var cfg Config
	if err := env.ParseWithOptions(&cfg, opts); err != nil {
		return Config{}, fmt.Errorf("%w: %w", errParseEnv, err)
	}

	cfg.GitHubToken = strings.TrimSpace(cfg.GitHubToken)
	cfg.AnytypeBaseURL = strings.TrimSpace(cfg.AnytypeBaseURL)

	if cfg.AnytypeBaseURL == "" {
		cfg.AnytypeBaseURL = defaultAnytypeBaseURL
	}
	return cfg, nil
}

func loadDotEnv(path string) (map[string]string, error) {
	values, err := godotenv.Read(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("%w: %w", errOpenEnvFile, err)
	}

	if values == nil {
		return map[string]string{}, nil
	}
	return values, nil
}

func mergedEnv(fileEnv map[string]string, environ []string) map[string]string {
	// start from .env file values, then let process env override them.
	merged := make(map[string]string, len(fileEnv)+len(environ))
	maps.Copy(merged, fileEnv)

	for _, entry := range environ {
		key, value, found := strings.Cut(entry, "=")
		if !found || key == "" {
			continue
		}
		merged[key] = value
	}
	return merged
}

// ResolveEnvFilePath chooses a practical .env.local location for CLI use so
// installed binaries still behave predictably outside the repository root.
func ResolveEnvFilePath() string {
	path := resolveEnvFilePathWith(
		os.Getwd,
		os.UserConfigDir,
		os.Executable,
		func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && !info.IsDir()
		},
	)
	if path != "" {
		return path
	}
	return defaultEnvFileName
}

func resolveEnvFilePathWith(
	getwd func() (string, error),
	userCfgDir func() (string, error),
	executable func() (string, error),
	fileExists func(string) bool,
) string {
	if fileExists == nil {
		return ""
	}

	resolve := func(provider func() (string, error), buildPath func(string) string) string {
		if provider == nil {
			return ""
		}
		base, err := provider()
		if err != nil {
			return ""
		}
		base = strings.TrimSpace(base)
		if base == "" {
			return ""
		}
		candidate := buildPath(base)
		if fileExists(candidate) {
			return candidate
		}
		return ""
	}

	candidates := []string{
		resolve(getwd, func(base string) string { return filepath.Join(base, ".env.local") }),
		resolve(userCfgDir, func(base string) string { return filepath.Join(base, "anytype-gh", ".env.local") }),
		resolve(executable, func(base string) string { return filepath.Join(filepath.Dir(base), ".env.local") }),
	}

	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}
