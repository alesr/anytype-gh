package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/alesr/anytype-gh/internal/clients/github"
	"github.com/alesr/anytype-gh/internal/repositories/state"
	"github.com/alesr/anytype-gh/internal/services/spaces"
	"github.com/alesr/anytype-gh/internal/services/syncing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApp_RunAuth(t *testing.T) {
	t.Parallel()

	t.Run("success does not print key preview", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		a := &cli{out: &out}

		store := &mockStateStore{path: "/tmp/state.json"}

		prompt := &mockPromptService{
			readLineFunc: func(string) (string, error) { return "code-123", nil },
		}

		auth := &mockAuthService{
			startChallengeFunc: func(context.Context, string) (string, error) { return "challenge-1", nil },
			exchangeCodeAndPersistFunc: func(context.Context, string, string) (string, error) {
				return "api-key-secret", nil
			},
		}

		a.authSvc = auth
		a.prompt = prompt
		a.store = store

		err := a.runAuth(context.TODO())
		require.NoError(t, err)

		assert.Contains(t, out.String(), "Authentication complete. App key saved to /tmp/state.json")
		assert.Equal(t, anytypeIntegrationName, auth.startChallengeAppName)
		assert.Equal(t, "challenge-1", auth.exchangeChallengeID)
		assert.Equal(t, "code-123", auth.exchangeCode)
	})

	t.Run("returns wrapped start challenge error", func(t *testing.T) {
		t.Parallel()

		startChallengeErr := errors.New("start challenge")

		a := &cli{out: &bytes.Buffer{}}

		store := &mockStateStore{path: "/tmp/state.json"}

		prompt := &mockPromptService{
			readLineFunc: func(string) (string, error) { return "code-123", nil },
		}

		auth := &mockAuthService{
			startChallengeFunc: func(context.Context, string) (string, error) { return "", startChallengeErr },
		}

		a.authSvc = auth
		a.prompt = prompt
		a.store = store

		err := a.runAuth(context.TODO())
		require.Error(t, err)

		assert.ErrorIs(t, err, startChallengeErr)
	})

	t.Run("returns wrapped prompt read error", func(t *testing.T) {
		t.Parallel()

		readInputErr := errors.New("read input")

		a := &cli{out: &bytes.Buffer{}}

		store := &mockStateStore{path: "/tmp/state.json"}

		prompt := &mockPromptService{
			readLineFunc: func(string) (string, error) { return "", readInputErr },
		}

		auth := &mockAuthService{
			startChallengeFunc: func(context.Context, string) (string, error) { return "challenge-1", nil },
		}

		a.authSvc = auth
		a.prompt = prompt
		a.store = store

		err := a.runAuth(context.TODO())
		require.Error(t, err)

		assert.ErrorIs(t, err, readInputErr)
	})

	t.Run("returns wrapped exchange error", func(t *testing.T) {
		t.Parallel()

		exchangeErr := errors.New("exchange code")

		a := &cli{out: &bytes.Buffer{}}

		store := &mockStateStore{path: "/tmp/state.json"}

		prompt := &mockPromptService{
			readLineFunc: func(string) (string, error) { return "code-123", nil },
		}

		auth := &mockAuthService{
			startChallengeFunc: func(context.Context, string) (string, error) { return "challenge-1", nil },
			exchangeCodeAndPersistFunc: func(context.Context, string, string) (string, error) {
				return "", exchangeErr
			},
		}

		a.authSvc = auth
		a.prompt = prompt
		a.store = store

		err := a.runAuth(context.TODO())
		require.Error(t, err)

		assert.ErrorIs(t, err, exchangeErr)
	})
}

func TestApp_RunSpaces(t *testing.T) {
	t.Parallel()

	t.Run("returns error when app key missing", func(t *testing.T) {
		t.Parallel()

		a := &cli{out: &bytes.Buffer{}}

		currentState := state.NewAppState()

		store := &mockStateStore{
			loadFunc: func(context.Context) (*state.AppState, error) { return currentState, nil },
		}

		space := &mockSpaceService{}
		prompt := &mockPromptService{}

		a.spaceSvc = space
		a.prompt = prompt
		a.store = store

		err := a.runSpaces(context.TODO())
		require.Error(t, err)

		assert.ErrorIs(t, err, errAnytypeAppKeyNotFound)
	})

	t.Run("returns wrapped load error", func(t *testing.T) {
		t.Parallel()

		loadErr := errors.New("load state")

		a := &cli{out: &bytes.Buffer{}}

		store := &mockStateStore{
			loadFunc: func(context.Context) (*state.AppState, error) { return nil, loadErr },
		}

		var (
			space  *mockSpaceService
			prompt *mockPromptService
		)

		a.spaceSvc = space
		a.prompt = prompt
		a.store = store

		err := a.runSpaces(context.TODO())
		require.Error(t, err)

		assert.ErrorIs(t, err, loadErr)
	})

	t.Run("returns wrapped list error", func(t *testing.T) {
		t.Parallel()

		listSpacesErr := errors.New("list spaces")

		a := &cli{out: &bytes.Buffer{}}

		currentState := &state.AppState{
			AnytypeAppKey: "app-key",
			Repositories:  map[string]state.RepositoryState{},
		}

		store := &mockStateStore{
			loadFunc: func(context.Context) (*state.AppState, error) { return currentState, nil },
		}

		space := &mockSpaceService{
			listFunc: func(context.Context, string) ([]spaces.Space, error) { return nil, listSpacesErr },
		}

		var prompt *mockPromptService

		a.spaceSvc = space
		a.prompt = prompt
		a.store = store

		err := a.runSpaces(context.TODO())
		require.Error(t, err)

		assert.ErrorIs(t, err, listSpacesErr)
	})

	t.Run("returns wrapped selection error", func(t *testing.T) {
		t.Parallel()

		selectOptionErr := errors.New("select option")

		a := &cli{out: &bytes.Buffer{}}

		currentState := &state.AppState{
			AnytypeAppKey: "app-key",
			Repositories:  map[string]state.RepositoryState{},
		}

		store := &mockStateStore{
			loadFunc: func(context.Context) (*state.AppState, error) { return currentState, nil },
		}

		space := &mockSpaceService{
			listFunc: func(context.Context, string) ([]spaces.Space, error) {
				return []spaces.Space{{ID: "space-1", Name: "Main"}}, nil
			},
		}

		prompt := &mockPromptService{
			chooseIndexFunc: func(string, []string) (int, error) { return -1, selectOptionErr },
		}

		a.spaceSvc = space
		a.prompt = prompt
		a.store = store

		err := a.runSpaces(context.TODO())
		require.Error(t, err)

		assert.ErrorIs(t, err, selectOptionErr)
	})

	t.Run("returns wrapped set default error", func(t *testing.T) {
		t.Parallel()

		setDefaultErr := errors.New("set default")

		a := &cli{out: &bytes.Buffer{}}

		currentState := &state.AppState{
			AnytypeAppKey: "app-key",
			Repositories:  map[string]state.RepositoryState{},
		}

		store := &mockStateStore{
			loadFunc: func(context.Context) (*state.AppState, error) { return currentState, nil },
			saveFunc: func(context.Context, *state.AppState) error { return nil },
		}

		space := &mockSpaceService{
			listFunc: func(context.Context, string) ([]spaces.Space, error) {
				return []spaces.Space{{ID: "space-1", Name: "Main"}}, nil
			},
			setDefaultFunc: func(context.Context, string) error { return setDefaultErr },
		}

		prompt := &mockPromptService{
			chooseIndexFunc: func(string, []string) (int, error) { return 0, nil },
		}

		a.spaceSvc = space
		a.prompt = prompt
		a.store = store

		err := a.runSpaces(context.TODO())
		require.Error(t, err)

		assert.ErrorIs(t, err, setDefaultErr)
	})
}

func TestApp_RunSync(t *testing.T) {
	t.Parallel()

	t.Run("syncs with stored default space and prints action", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		a := &cli{out: &out}

		store := &mockStateStore{
			loadFunc: func(context.Context) (*state.AppState, error) {
				return &state.AppState{
					AnytypeAppKey: "app-key",
					DefaultSpace:  "space-1",
					Repositories:  map[string]state.RepositoryState{},
				}, nil
			},
		}

		var space *mockSpaceService

		repo := &mockRepoService{
			listAccessibleReposFunc: func(context.Context) ([]github.Repository, error) {
				return []github.Repository{
					{Owner: "octo", Name: "repo", FullName: "octo/repo", Private: true},
				}, nil
			},
		}

		sync := &mockSyncService{
			syncFunc: func(context.Context, syncing.Params) (syncing.Result, error) {
				return syncing.Result{
					Action:   "updated",
					ObjectID: "obj-1",
					RepoFull: "octo/repo",
				}, nil
			},
		}

		prompt := &mockPromptService{
			chooseIndexFunc: func(string, []string) (int, error) { return 0, nil },
		}

		a.spaceSvc = space
		a.repoSvc = repo
		a.syncSvc = sync
		a.prompt = prompt
		a.store = store

		err := a.runSync(context.TODO())
		require.NoError(t, err)

		require.NotNil(t, sync.syncParams)

		assert.Contains(t, out.String(), "Updated Anytype page obj-1 for octo/repo")
		assert.Equal(t, "octo", sync.syncParams.Owner)
		assert.Equal(t, "repo", sync.syncParams.Name)
		assert.Equal(t, "app-key", sync.syncParams.AppKey)
		assert.Equal(t, "space-1", sync.syncParams.SpaceID)
	})

	t.Run("returns wrapped repo list error", func(t *testing.T) {
		t.Parallel()

		listReposErr := errors.New("list repos")

		a := &cli{out: &bytes.Buffer{}}

		store := &mockStateStore{
			loadFunc: func(context.Context) (*state.AppState, error) {
				return &state.AppState{
					AnytypeAppKey: "app-key",
					DefaultSpace:  "space-1",
					Repositories:  map[string]state.RepositoryState{},
				}, nil
			},
		}

		var space *mockSpaceService

		repo := &mockRepoService{
			listAccessibleReposFunc: func(context.Context) ([]github.Repository, error) {
				return nil, listReposErr
			},
		}

		var (
			sync   *mockSyncService
			prompt *mockPromptService
		)

		a.spaceSvc = space
		a.repoSvc = repo
		a.syncSvc = sync
		a.prompt = prompt
		a.store = store

		err := a.runSync(context.TODO())
		require.Error(t, err)

		assert.ErrorIs(t, err, listReposErr)
	})

	t.Run("returns wrapped sync error", func(t *testing.T) {
		t.Parallel()

		syncErr := errors.New("sync")

		a := &cli{out: &bytes.Buffer{}}

		store := &mockStateStore{
			loadFunc: func(context.Context) (*state.AppState, error) {
				return &state.AppState{
					AnytypeAppKey: "app-key",
					DefaultSpace:  "space-1",
					Repositories:  map[string]state.RepositoryState{},
				}, nil
			},
		}

		var space *mockSpaceService

		repo := &mockRepoService{
			listAccessibleReposFunc: func(context.Context) ([]github.Repository, error) {
				return []github.Repository{
					{Owner: "octo", Name: "repo", FullName: "octo/repo"},
				}, nil
			},
		}

		sync := &mockSyncService{
			syncFunc: func(context.Context, syncing.Params) (syncing.Result, error) {
				return syncing.Result{}, syncErr
			},
		}

		prompt := &mockPromptService{
			chooseIndexFunc: func(string, []string) (int, error) { return 0, nil },
		}

		a.spaceSvc = space
		a.repoSvc = repo
		a.syncSvc = sync
		a.prompt = prompt
		a.store = store

		err := a.runSync(context.TODO())
		require.Error(t, err)

		assert.ErrorIs(t, err, syncErr)
	})
}

func TestNewApp(t *testing.T) {
	t.Parallel()

	var (
		store    *mockStateStore
		authSvc  *mockAuthService
		spaceSvc *mockSpaceService
		repoSvc  *mockRepoService
		syncSvc  *mockSyncService
	)

	t.Run("returns app when dependencies are complete", func(t *testing.T) {
		t.Parallel()

		app, err := NewApp(bytes.NewBufferString(""), &bytes.Buffer{}, store, authSvc, spaceSvc, repoSvc, syncSvc)
		require.NoError(t, err)
		require.NotNil(t, app)
		require.NotNil(t, app.prompt)
	})

	t.Run("fails fast when store is missing", func(t *testing.T) {
		t.Parallel()

		_, err := NewApp(bytes.NewBufferString(""), &bytes.Buffer{}, nil, authSvc, spaceSvc, repoSvc, syncSvc)
		require.Error(t, err)
		assert.ErrorIs(t, err, errStoreDependencyRequired)
	})
}
