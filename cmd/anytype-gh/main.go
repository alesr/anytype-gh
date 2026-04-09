package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alesr/anytype-gh/internal/cli"
	"github.com/alesr/anytype-gh/internal/clients/github"
	"github.com/alesr/anytype-gh/internal/config"
	"github.com/alesr/anytype-gh/internal/repositories/state"
	"github.com/alesr/anytype-gh/internal/services/auth"
	"github.com/alesr/anytype-gh/internal/services/spaces"
	"github.com/alesr/anytype-gh/internal/services/syncing"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load(config.ResolveEnvFilePath())
	if err != nil {
		return fmt.Errorf("could not load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("could not validate config: %w", err)
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
