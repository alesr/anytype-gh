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
	"gopkg.in/yaml.v3"
)

const (
	defaultAnytypeBaseURL = "http://localhost:31009"
	defaultEnvFileName    = ".env.local"
	defaultConfigFileName = "config.yaml"
	defaultConfigDirName  = "anytype-gh"
)

var (
	errParseEnv           = errors.New("parse env")
	errOpenEnvFile        = errors.New("open env file")
	errOpenConfigFile     = errors.New("open config file")
	errParseConfigFile    = errors.New("parse config file")
	errCreateConfigDir    = errors.New("create config directory")
	errWriteConfigFile    = errors.New("write config file")
	errGitHubTokenMissing = errors.New("missing GitHub token (GH_TOKEN)")
)

// Config hold configuration values used throughout the application.
type Config struct {
	GitHubToken    string `env:"GH_TOKEN"`
	AnytypeBaseURL string `env:"ANYTYPE_BASE_URL"`
}

type fileConfig struct {
	GitHub struct {
		Token string `yaml:"token"`
	} `yaml:"github"`
	Anytype struct {
		BaseURL string `yaml:"base_url"`
	} `yaml:"anytype"`
}

// Validate fails fast on required values so startup errors are immediate and
// explicit instead of surfacing later in downstream clients.
func (c Config) Validate() error {
	if strings.TrimSpace(c.GitHubToken) == "" {
		return errGitHubTokenMissing
	}
	return nil
}

// Load resolves config with process env taking precedence over YAML and dotenv
// values, so explicit shell settings always win.
func Load(envFile string, configFile string) (Config, error) {
	if envFile == "" {
		envFile = defaultEnvFileName
	}
	if configFile == "" {
		configFile = defaultConfigFileName
	}

	fileEnv, err := loadDotEnv(envFile)
	if err != nil {
		return Config{}, err
	}

	fileCfg, err := loadYAMLConfig(configFile)
	if err != nil {
		return Config{}, err
	}

	opts := env.Options{
		Environment: mergedEnv(mergedMaps(fileEnv, fileCfg), os.Environ()),
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

func loadYAMLConfig(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("%w: %w", errOpenConfigFile, err)
	}

	if len(raw) == 0 {
		return map[string]string{}, nil
	}

	var cfg fileConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("%w: %w", errParseConfigFile, err)
	}

	values := map[string]string{}
	if token := strings.TrimSpace(cfg.GitHub.Token); token != "" {
		values["GH_TOKEN"] = token
	}

	if baseURL := strings.TrimSpace(cfg.Anytype.BaseURL); baseURL != "" {
		values["ANYTYPE_BASE_URL"] = baseURL
	}
	return values, nil
}

func mergedMaps(base map[string]string, override map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(override))
	maps.Copy(merged, base)
	maps.Copy(merged, override)
	return merged
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

// ResolveConfigFilePath returns the canonical user config path used for
// persistent startup configuration.
func ResolveConfigFilePath() string {
	configDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configDir) == "" {
		return defaultConfigFileName
	}
	return filepath.Join(configDir, defaultConfigDirName, defaultConfigFileName)
}

// SaveConfigFile persists startup configuration to the user config directory.
func SaveConfigFile(path string, cfg Config) error {
	path = strings.TrimSpace(path)
	if path == "" {
		path = ResolveConfigFilePath()
	}

	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("%w: %w", errCreateConfigDir, err)
	}

	file := fileConfig{}
	file.GitHub.Token = strings.TrimSpace(cfg.GitHubToken)
	file.Anytype.BaseURL = strings.TrimSpace(cfg.AnytypeBaseURL)

	content, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("%w: %w", errWriteConfigFile, err)
	}

	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("%w: %w", errWriteConfigFile, err)
	}
	return nil
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
