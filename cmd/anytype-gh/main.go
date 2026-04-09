package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alesr/anytype-gh/internal/cli"
	"github.com/alesr/anytype-gh/internal/clients/github"
	"github.com/alesr/anytype-gh/internal/config"
	"github.com/alesr/anytype-gh/internal/repositories/state"
	"github.com/alesr/anytype-gh/internal/services/auth"
	"github.com/alesr/anytype-gh/internal/services/spaces"
	"github.com/alesr/anytype-gh/internal/services/syncing"
	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := loadConfigWithBootstrap()
	if err != nil {
		return fmt.Errorf("could not load config: %w", err)
	}

	statePath, err := state.DefaultPath()
	if err != nil {
		return fmt.Errorf("could not resolve state path: %w", err)
	}

	store := state.NewFileStore(statePath)
	ghClient := github.NewClient(cfg.GitHubToken)

	app, err := cli.NewApp(
		os.Stdin,
		os.Stdout,
		store,
		auth.New(cfg.AnytypeBaseURL, store),
		spaces.New(cfg.AnytypeBaseURL, store),
		ghClient,
		syncing.New(ghClient, cfg.AnytypeBaseURL, store),
	)
	if err != nil {
		return fmt.Errorf("could not initialize app: %w", err)
	}
	return app.Run(ctx)
}

func loadConfigWithBootstrap() (config.Config, error) {
	envFile := config.ResolveEnvFilePath()
	configFile := config.ResolveConfigFilePath()

	cfg, err := config.Load(envFile, configFile)
	if err != nil {
		return config.Config{}, err
	}

	if strings.TrimSpace(cfg.GitHubToken) != "" {
		if err := cfg.Validate(); err != nil {
			return config.Config{}, fmt.Errorf("could not validate config: %w", err)
		}
		return cfg, nil
	}

	if !isInteractiveTTY() {
		return config.Config{}, fmt.Errorf(
			"could not validate config: missing GitHub token (GH_TOKEN). set GH_TOKEN or add github.token to %s",
			configFile,
		)
	}

	token, err := promptGitHubToken()
	if err != nil {
		return config.Config{}, fmt.Errorf("could not collect GitHub token: %w", err)
	}

	cfg.GitHubToken = token
	if err := cfg.Validate(); err != nil {
		return config.Config{}, fmt.Errorf("could not validate config: %w", err)
	}
	if err := config.SaveConfigFile(configFile, cfg); err != nil {
		return config.Config{}, fmt.Errorf("could not persist config: %w", err)
	}
	return cfg, nil
}

func promptGitHubToken() (string, error) {
	var token string

	field := huh.NewInput().
		Title("Enter your GitHub token (GH_TOKEN):").
		EchoMode(huh.EchoModePassword).
		Value(&token)

	if err := field.Run(); err != nil {
		return "", err
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("GitHub token is required")
	}
	return token, nil
}

func isInteractiveTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
