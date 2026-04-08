package syncing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alesr/anytype-gh/internal/anytypefactory"
	"github.com/alesr/anytype-gh/internal/clients/github"
	"github.com/alesr/anytype-gh/internal/repositories/state"
	anytypesdk "github.com/epheo/anytype-go"
)

const listPageSize = 100

var (
	statusCodePattern = regexp.MustCompile(`\bstatus\s+(\d{3})\b`)

	// enumerate sync errors

	errLoadState                   = errors.New("load state")
	errSaveState                   = errors.New("save state")
	errRepositoryOwnerNameRequired = errors.New("repository owner and name are required")
	errAnytypeSpaceIDRequired      = errors.New("anytype space id is required")
	errCouldNotCreateAnytypePage   = errors.New("could not create anytype page")
	errCouldNotUpdateAnytypePage   = errors.New("could not update anytype page")
	errCouldNotListAnytypePages    = errors.New("could not list anytype pages")
)

type (
	Params struct {
		Owner   string
		Name    string
		AppKey  string
		SpaceID string
	}

	Result struct {
		Action     string
		RepoFull   string
		ObjectID   string
		ReadmeSHA  string
		SpaceID    string
		SkippedMsg string
	}

	page struct {
		Title    string
		Markdown string
	}

	gitHubGateway interface {
		GetReadme(ctx context.Context, owner, name string) (github.Readme, error)
	}

	stateStore interface {
		Load(ctx context.Context) (*state.AppState, error)
		Save(ctx context.Context, state *state.AppState) error
	}

	clock interface {
		Now() time.Time
	}
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	github  gitHubGateway
	anytype *anytypefactory.Factory
	state   stateStore
	clock   clock
}

type syncRequest struct {
	spaceID      string
	client       anytypesdk.Client
	readme       github.Readme
	currentState *state.AppState
	repoState    state.RepositoryState
	pageTitle    string
}

func New(github gitHubGateway, baseURL string, store stateStore) *Service {
	return &Service{
		github:  github,
		anytype: anytypefactory.New(baseURL),
		state:   store,
		clock:   realClock{},
	}
}

func (s *Service) Sync(ctx context.Context, params Params) (Result, error) {
	req, err := s.buildSyncRequest(ctx, params)
	if err != nil {
		return Result{}, err
	}

	skipped, result, repoState, err := s.skipIfUnchanged(ctx, req)
	if err != nil {
		return Result{}, err
	}
	req.repoState = repoState

	if skipped {
		return result, nil
	}

	result, repoState, err = s.applySync(ctx, req)
	if err != nil {
		return Result{}, err
	}

	repoState.LastReadmeSHA = req.readme.SHA
	repoState.LastSyncedAt = s.clock.Now()
	repoState.SpaceID = req.spaceID

	if err := s.saveRepoState(ctx, req.currentState, req.readme.RepoFullName, repoState); err != nil {
		return Result{}, err
	}

	result.ObjectID = repoState.ObjectID
	return result, nil
}

func (s *Service) buildSyncRequest(ctx context.Context, params Params) (syncRequest, error) {
	owner := strings.TrimSpace(params.Owner)
	name := strings.TrimSpace(params.Name)

	if owner == "" || name == "" {
		return syncRequest{}, errRepositoryOwnerNameRequired
	}

	spaceID := strings.TrimSpace(params.SpaceID)
	if spaceID == "" {
		return syncRequest{}, errAnytypeSpaceIDRequired
	}

	client, err := s.anytype.AuthedClient(params.AppKey)
	if err != nil {
		return syncRequest{}, err
	}

	readme, err := s.github.GetReadme(ctx, owner, name)
	if err != nil {
		return syncRequest{}, err
	}

	currentState, err := s.state.Load(ctx)
	if err != nil {
		return syncRequest{}, fmt.Errorf("%w: %w", errLoadState, err)
	}

	repoState := currentState.Repositories[readme.RepoFullName]
	pageTitle := fmt.Sprintf("README - %s", readme.RepoFullName)

	if strings.TrimSpace(repoState.ObjectID) == "" {
		objectID, lookupErr := findAnytypePageByTitle(ctx, client, spaceID, pageTitle)
		if lookupErr != nil {
			return syncRequest{}, lookupErr
		}
		repoState.ObjectID = objectID
	}
	return syncRequest{
		spaceID:      spaceID,
		client:       client,
		readme:       readme,
		currentState: currentState,
		repoState:    repoState,
		pageTitle:    pageTitle,
	}, nil
}

func (s *Service) skipIfUnchanged(ctx context.Context, req syncRequest) (bool, Result, state.RepositoryState, error) {
	if req.repoState.ObjectID == "" || req.repoState.LastReadmeSHA != req.readme.SHA {
		return false, Result{}, req.repoState, nil
	}

	exists, err := findAnytypePageByID(ctx, req.client, req.spaceID, req.repoState.ObjectID)
	if err != nil {
		return false, Result{}, req.repoState, err
	}
	if !exists {
		// Object was deleted externally; continue with normal sync to recreate.
		req.repoState.ObjectID = ""
		return false, Result{}, req.repoState, nil
	}

	if req.currentState.Repositories[req.readme.RepoFullName].ObjectID == "" {
		req.repoState.SpaceID = req.spaceID
		if saveErr := s.saveRepoState(ctx, req.currentState, req.readme.RepoFullName, req.repoState); saveErr != nil {
			return false, Result{}, req.repoState, saveErr
		}
	}
	return true, Result{
		Action:     "skipped",
		RepoFull:   req.readme.RepoFullName,
		ObjectID:   req.repoState.ObjectID,
		ReadmeSHA:  req.readme.SHA,
		SpaceID:    req.spaceID,
		SkippedMsg: "README SHA unchanged",
	}, req.repoState, nil
}

func (s *Service) applySync(ctx context.Context, req syncRequest) (Result, state.RepositoryState, error) {
	result := Result{
		RepoFull:  req.readme.RepoFullName,
		ReadmeSHA: req.readme.SHA,
		SpaceID:   req.spaceID,
	}

	targetPage := page{Title: req.pageTitle, Markdown: req.readme.Content}

	if req.repoState.ObjectID == "" {
		objectID, err := createAnytypePage(ctx, req.client, req.spaceID, targetPage)
		if err != nil {
			return Result{}, req.repoState, err
		}
		req.repoState.ObjectID = objectID
		result.Action = "created"
		return result, req.repoState, nil
	}

	result.Action = "updated"
	result.ObjectID = req.repoState.ObjectID

	if err := updateAnytypePage(ctx, req.client, req.spaceID, req.repoState.ObjectID, targetPage); err != nil {
		if !isNotFound(err) {
			return Result{}, req.repoState, err
		}

		objectID, createErr := createAnytypePage(ctx, req.client, req.spaceID, targetPage)
		if createErr != nil {
			return Result{}, req.repoState, createErr
		}

		req.repoState.ObjectID = objectID
		result.Action = "recreated"
	}
	return result, req.repoState, nil
}

func (s *Service) saveRepoState(ctx context.Context, currentState *state.AppState, repoFullName string, repoState state.RepositoryState) error {
	currentState.Repositories[repoFullName] = repoState
	if err := s.state.Save(ctx, currentState); err != nil {
		return fmt.Errorf("%w: %w", errSaveState, err)
	}
	return nil
}

func createAnytypePage(ctx context.Context, client anytypesdk.Client, spaceID string, page page) (string, error) {
	response, err := client.Space(spaceID).Objects().Create(
		ctx,
		anytypesdk.CreateObjectRequest{
			TypeKey: "page",
			Name:    page.Title,
			Body:    page.Markdown,
		},
	)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errCouldNotCreateAnytypePage, err)
	}

	if response == nil || response.Object == nil || strings.TrimSpace(response.Object.ID) == "" {
		return "", errors.New("anytype create page response did not include object id")
	}
	return response.Object.ID, nil
}

func updateAnytypePage(ctx context.Context, client anytypesdk.Client, spaceID, objectID string, page page) error {
	_, err := client.Space(spaceID).Object(objectID).Update(ctx, anytypesdk.UpdateObjectRequest{
		Name:     page.Title,
		Markdown: page.Markdown,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", errCouldNotUpdateAnytypePage, err)
	}
	return nil
}

func findAnytypePageByTitle(ctx context.Context, client anytypesdk.Client, spaceID, title string) (string, error) {
	for offset := 0; ; offset += listPageSize {
		objects, err := client.Space(spaceID).Objects().List(ctx, anytypesdk.WithLimit(listPageSize), anytypesdk.WithOffset(offset))
		if err != nil {
			return "", fmt.Errorf("%w: %w", errCouldNotListAnytypePages, err)
		}

		for _, object := range objects {
			if object.Archived {
				continue
			}
			if strings.TrimSpace(object.Name) == title {
				return object.ID, nil
			}
		}

		if len(objects) < listPageSize {
			break
		}
	}
	return "", nil
}

func findAnytypePageByID(ctx context.Context, client anytypesdk.Client, spaceID, objectID string) (bool, error) {
	for offset := 0; ; offset += listPageSize {
		objects, err := client.Space(spaceID).Objects().List(ctx, anytypesdk.WithLimit(listPageSize), anytypesdk.WithOffset(offset))
		if err != nil {
			return false, fmt.Errorf("%w: %w", errCouldNotListAnytypePages, err)
		}

		for _, object := range objects {
			if object.Archived {
				continue
			}
			if object.ID == objectID {
				return true, nil
			}
		}

		if len(objects) < listPageSize {
			break
		}
	}

	return false, nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return hasHTTPStatus(err, http.StatusNotFound)
}

func hasHTTPStatus(err error, statusCode int) bool {
	for current := err; current != nil; current = errors.Unwrap(current) {
		matches := statusCodePattern.FindStringSubmatch(strings.ToLower(current.Error()))
		if len(matches) == 2 && matches[1] == fmt.Sprintf("%d", statusCode) {
			return true
		}
	}
	return false
}
