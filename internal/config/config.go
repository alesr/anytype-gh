// Package config centralizes runtime configuration rules so startup behavior
// stays predictable across local runs and installed binaries.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/caarlos0/env/v11"
)

const (
	defaultAnytypeBaseURL = "http://localhost:31009"
)

var (
	errParseEnv       = errors.New("parse env")
	errOpenEnvFile    = errors.New("open env file")
	errInvalidEnvLine = errors.New("invalid env line")
	errInvalidEnvKey  = errors.New("invalid env key in line")
	errReadEnvFile    = errors.New("read env file")
)

// Runtime captures values that directly influence external side effects.
// Keeping this struct small reduces ambiguity about which env vars matter.
type Runtime struct {
	GitHubToken    string `env:"GH_TOKEN"`
	AnytypeBaseURL string `env:"ANYTYPE_BASE_URL"`
}

// Load resolves runtime config with process env taking precedence over dotenv
// values, so explicit shell settings always win.
func Load(envFile string) (Runtime, error) {
	if envFile == "" {
		envFile = ".env.local"
	}

	fileEnv, err := loadDotEnv(envFile)
	if err != nil {
		return Runtime{}, err
	}

	var runtime Runtime
	if err := env.ParseWithOptions(&runtime, env.Options{
		Environment: mergedEnv(fileEnv, os.Environ()),
	}); err != nil {
		return Runtime{}, fmt.Errorf("%w: %w", errParseEnv, err)
	}

	runtime.GitHubToken = strings.TrimSpace(runtime.GitHubToken)
	runtime.AnytypeBaseURL = strings.TrimSpace(runtime.AnytypeBaseURL)

	if runtime.AnytypeBaseURL == "" {
		runtime.AnytypeBaseURL = defaultAnytypeBaseURL
	}
	return runtime, nil
}

func loadDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("%w: %w", errOpenEnvFile, err)
	}
	defer file.Close()

	values := map[string]string{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if after, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimSpace(after)
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("%w: %q", errInvalidEnvLine, line)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if key == "" {
			return nil, fmt.Errorf("%w: %q", errInvalidEnvKey, line)
		}

		value = trimMatchingQuotes(value)
		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", errReadEnvFile, err)
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

func trimMatchingQuotes(value string) string {
	if len(value) < 2 {
		return value
	}

	first := value[0]
	last := value[len(value)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

// ResolveEnvFilePath chooses a practical .env.local location for CLI use so
// installed binaries still behave predictably outside the repository root.
func ResolveEnvFilePath() string {
	const envFileName = ".env.local"

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
	return envFileName
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
