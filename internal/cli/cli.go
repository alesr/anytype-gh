// cli package provides the command-line interface for the anytype-gh tool.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alesr/anytype-gh/internal/clients/github"
	"github.com/alesr/anytype-gh/internal/repositories/state"
	"github.com/alesr/anytype-gh/internal/services/spaces"
	"github.com/alesr/anytype-gh/internal/services/syncing"
)

const (
	anytypeIntegrationName = "anytypeGH"

	defaultOperationTimeout = 20 * time.Second
	syncOperationTimeout    = 60 * time.Second
)

var (
	mainMenuOptions = []string{
		"Authenticate Anytype",
		"Choose default space",
		"Sync repository README",
		"Exit",
	}

	// enumerate cli errors

	errStoreDependencyRequired        = errors.New("store dependency is required")
	errAuthServiceDependencyRequired  = errors.New("auth service dependency is required")
	errSpaceServiceDependencyRequired = errors.New("space service dependency is required")
	errRepoServiceDependencyRequired  = errors.New("repo service dependency is required")
	errSyncServiceDependencyRequired  = errors.New("sync service dependency is required")
	errPromptDependencyRequired       = errors.New("prompt dependency is required")
	errAnytypeAppKeyNotFound          = errors.New("anytype app key not found")
	errNoAnytypeSpaces                = errors.New("no spaces available in anytype")
	errNoRepositories                 = errors.New("no repositories available for this token")
)

type (
	authService interface {
		StartChallenge(ctx context.Context, appName string) (string, error)
		ExchangeCodeAndPersist(ctx context.Context, challengeID, code string) (string, error)
	}

	spaceService interface {
		List(ctx context.Context, appKey string) ([]spaces.Space, error)
		SetDefault(ctx context.Context, spaceID string) error
	}

	repoService interface {
		ListAccessibleRepos(ctx context.Context) ([]github.Repository, error)
	}

	syncService interface {
		Sync(ctx context.Context, params syncing.Params) (syncing.Result, error)
	}

	promptService interface {
		readLine(label string) (string, error)
		chooseIndex(label string, options []string) (int, error)
	}

	stateStore interface {
		Load(ctx context.Context) (*state.AppState, error)
		Save(ctx context.Context, state *state.AppState) error
		Path() string
	}
)

type cli struct {
	in  io.Reader
	out io.Writer

	store    stateStore
	authSvc  authService
	spaceSvc spaceService
	repoSvc  repoService
	syncSvc  syncService
	prompt   promptService
}

// NewApp builds an interactive application with injected collaborators,
// making behavior explicit at the composition root.
func NewApp(
	input io.Reader,
	output io.Writer,
	store stateStore,
	authSvc authService,
	spaceSvc spaceService,
	repoSvc repoService,
	syncSvc syncService,
) (*cli, error) {
	app := &cli{
		in:       input,
		out:      output,
		store:    store,
		authSvc:  authSvc,
		spaceSvc: spaceSvc,
		repoSvc:  repoSvc,
		syncSvc:  syncSvc,
		prompt:   newPrompter(input, output),
	}
	if err := app.validate(); err != nil {
		return nil, err
	}
	return app, nil
}

// Run executes the interactive menu loop until the user exits or an operation fails.
func (a *cli) Run(ctx context.Context) error {
	for {
		choice, err := a.prompt.chooseIndex("Choose an action:", mainMenuOptions)
		if err != nil {
			return fmt.Errorf("could not select action: %w", err)
		}

		switch choice {
		case 0:
			if err := a.runAuth(ctx); err != nil {
				return err
			}
		case 1:
			if err := a.runSpaces(ctx); err != nil {
				return err
			}
		case 2:
			if err := a.runSync(ctx); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (a *cli) validate() error {
	switch {
	case a.store == nil:
		return errStoreDependencyRequired
	case a.authSvc == nil:
		return errAuthServiceDependencyRequired
	case a.spaceSvc == nil:
		return errSpaceServiceDependencyRequired
	case a.repoSvc == nil:
		return errRepoServiceDependencyRequired
	case a.syncSvc == nil:
		return errSyncServiceDependencyRequired
	case a.prompt == nil:
		return errPromptDependencyRequired
	default:
		return nil
	}
}

func (a *cli) runAuth(ctx context.Context) error {
	challengeCtx, challengeCancel := context.WithTimeout(ctx, defaultOperationTimeout)
	challengeID, err := a.authSvc.StartChallenge(challengeCtx, anytypeIntegrationName)
	challengeCancel()
	if err != nil {
		return fmt.Errorf("could not start authentication challenge: %w", err)
	}

	fmt.Fprintln(a.out, "Anytype should now display a verification code.")
	fmt.Fprintln(a.out, "Open the Anytype app and approve the request.")

	code, err := a.prompt.readLine("Enter Anytype verification code: ")
	if err != nil {
		return fmt.Errorf("could not read verification code: %w", err)
	}

	exchangeCtx, exchangeCancel := context.WithTimeout(ctx, defaultOperationTimeout)
	_, err = a.authSvc.ExchangeCodeAndPersist(exchangeCtx, challengeID, code)
	exchangeCancel()
	if err != nil {
		return fmt.Errorf("could not exchange code: %w", err)
	}

	fmt.Fprintf(a.out, "Authentication complete. App key saved to %s\n", a.store.Path())
	return nil
}

func (a *cli) runSpaces(ctx context.Context) error {
	currentState, err := a.store.Load(ctx)
	if err != nil {
		return fmt.Errorf("could not load state: %w", err)
	}

	appKey := strings.TrimSpace(currentState.AnytypeAppKey)
	if appKey == "" {
		return fmt.Errorf("%w. run `anytype-gh` and choose Authenticate Anytype", errAnytypeAppKeyNotFound)
	}

	listCtx, listCancel := context.WithTimeout(ctx, defaultOperationTimeout)
	spaces, err := a.spaceSvc.List(listCtx, appKey)
	listCancel()
	if err != nil {
		return fmt.Errorf("could not list spaces: %w", err)
	}

	if len(spaces) == 0 {
		return errNoAnytypeSpaces
	}

	labels := make([]string, len(spaces))
	for i := range spaces {
		labels[i] = fmt.Sprintf("%s [%s]", spaces[i].Name, spaces[i].ID)
	}

	choice, err := a.prompt.chooseIndex("Choose a default space:", labels)
	if err != nil {
		return fmt.Errorf("could not select space: %w", err)
	}

	targetDefault := spaces[choice].ID
	if err := a.spaceSvc.SetDefault(ctx, targetDefault); err != nil {
		return fmt.Errorf("could not set default space: %w", err)
	}

	fmt.Fprintf(a.out, "Default space set to %s\n", targetDefault)
	return nil
}

func (a *cli) runSync(ctx context.Context) error {
	currentState, err := a.store.Load(ctx)
	if err != nil {
		return fmt.Errorf("could not load state: %w", err)
	}

	appKey := strings.TrimSpace(currentState.AnytypeAppKey)
	if appKey == "" {
		return fmt.Errorf("%w. run `anytype-gh` and choose Authenticate Anytype", errAnytypeAppKeyNotFound)
	}

	spaceID := strings.TrimSpace(currentState.DefaultSpace)

	if spaceID == "" {
		listCtx, listCancel := context.WithTimeout(ctx, defaultOperationTimeout)

		spaces, listErr := a.spaceSvc.List(listCtx, appKey)
		listCancel()
		if listErr != nil {
			return fmt.Errorf("could not list spaces: %w", listErr)
		}

		if len(spaces) == 0 {
			return errNoAnytypeSpaces
		}

		labels := make([]string, len(spaces))
		for i, s := range spaces {
			labels[i] = fmt.Sprintf("%s [%s]", s.Name, s.ID)
		}

		index, choiceErr := a.prompt.chooseIndex("Choose an Anytype space:", labels)
		if choiceErr != nil {
			return fmt.Errorf("could not select space: %w", choiceErr)
		}

		spaceID = spaces[index].ID
		if err := a.spaceSvc.SetDefault(ctx, spaceID); err != nil {
			return fmt.Errorf("could not store default space: %w", err)
		}
	}

	owner, name, err := a.resolveRepository(ctx)
	if err != nil {
		return err
	}

	syncCtx, syncCancel := context.WithTimeout(ctx, syncOperationTimeout)

	result, err := a.syncSvc.Sync(syncCtx, syncing.Params{
		Owner:   owner,
		Name:    name,
		AppKey:  appKey,
		SpaceID: spaceID,
	})
	syncCancel()
	if err != nil {
		return fmt.Errorf("could not sync: %w", err)
	}

	switch result.Action {
	case "skipped":
		fmt.Fprintf(a.out, "Skipped: %s (%s)\n", result.RepoFull, result.SkippedMsg)
	case "created":
		fmt.Fprintf(a.out, "Created Anytype page for %s in space %s\n", result.RepoFull, result.SpaceID)
	case "updated":
		fmt.Fprintf(a.out, "Updated Anytype page %s for %s\n", result.ObjectID, result.RepoFull)
	case "recreated":
		fmt.Fprintf(a.out, "Recreated Anytype page %s for %s (previous object missing)\n", result.ObjectID, result.RepoFull)
	default:
		fmt.Fprintf(a.out, "Sync result: %s\n", result.Action)
	}
	return nil
}

func (a *cli) resolveRepository(ctx context.Context) (string, string, error) {
	listCtx, listCancel := context.WithTimeout(ctx, defaultOperationTimeout)

	repos, err := a.repoSvc.ListAccessibleRepos(listCtx)
	listCancel()
	if err != nil {
		return "", "", fmt.Errorf("could not list repositories: %w", err)
	}

	if len(repos) == 0 {
		return "", "", errNoRepositories
	}

	labels := make([]string, len(repos))
	for i, repo := range repos {
		visibility := "public"
		if repo.Private {
			visibility = "private"
		}
		labels[i] = fmt.Sprintf("%s (%s)", repo.FullName, visibility)
	}

	index, err := a.prompt.chooseIndex("Choose a repository:", labels)
	if err != nil {
		return "", "", fmt.Errorf("could not select repository: %w", err)
	}

	selected := repos[index]
	return selected.Owner, selected.Name, nil
}
