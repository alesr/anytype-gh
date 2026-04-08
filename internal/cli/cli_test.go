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
		a := &app{out: &out}

		store := &mockStateStore{path: "/tmp/state.json"}
		prompt := &mockPromptService{readLineValue: "code-123"}
		auth := &mockAuthService{
			startChallengeValue: "challenge-1",
			exchangeValue:       "api-key-secret",
		}
		a.authSvc = auth
		a.prompt = prompt
		a.store = store

		err := a.runAuth(context.Background())
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Authentication complete. App key saved to /tmp/state.json")
		assert.NotContains(t, out.String(), "App key preview")
		assert.Equal(t, anytypeIntegrationName, auth.startChallengeAppName)
		assert.Equal(t, "challenge-1", auth.exchangeChallengeID)
		assert.Equal(t, "code-123", auth.exchangeCode)
	})

	t.Run("returns wrapped start challenge error", func(t *testing.T) {
		t.Parallel()

		startChallengeErr := errors.New("start challenge")

		a := &app{out: &bytes.Buffer{}}
		store := &mockStateStore{path: "/tmp/state.json"}
		prompt := &mockPromptService{readLineValue: "code-123"}
		auth := &mockAuthService{startChallengeErr: startChallengeErr}
		a.authSvc = auth
		a.prompt = prompt
		a.store = store

		err := a.runAuth(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, startChallengeErr)
	})

	t.Run("returns wrapped prompt read error", func(t *testing.T) {
		t.Parallel()

		readInputErr := errors.New("read input")

		a := &app{out: &bytes.Buffer{}}
		store := &mockStateStore{path: "/tmp/state.json"}
		prompt := &mockPromptService{readLineErr: readInputErr}
		auth := &mockAuthService{startChallengeValue: "challenge-1"}
		a.authSvc = auth
		a.prompt = prompt
		a.store = store

		err := a.runAuth(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, readInputErr)
	})

	t.Run("returns wrapped exchange error", func(t *testing.T) {
		t.Parallel()

		exchangeErr := errors.New("exchange code")

		a := &app{out: &bytes.Buffer{}}
		store := &mockStateStore{path: "/tmp/state.json"}
		prompt := &mockPromptService{readLineValue: "code-123"}
		auth := &mockAuthService{
			startChallengeValue: "challenge-1",
			exchangeErr:         exchangeErr,
		}
		a.authSvc = auth
		a.prompt = prompt
		a.store = store

		err := a.runAuth(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, exchangeErr)
	})
}

func TestApp_RunSpaces(t *testing.T) {
	t.Parallel()

	t.Run("returns error when app key missing", func(t *testing.T) {
		t.Parallel()

		a := &app{out: &bytes.Buffer{}}
		store := &mockStateStore{state: state.NewAppState()}
		space := &mockSpaceService{}
		prompt := &mockPromptService{}
		a.spaceSvc = space
		a.prompt = prompt
		a.store = store

		err := a.runSpaces(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, errAnytypeAppKeyNotFound)
	})

	t.Run("returns wrapped load error", func(t *testing.T) {
		t.Parallel()

		loadErr := errors.New("load state")

		a := &app{out: &bytes.Buffer{}}
		store := &mockStateStore{loadErr: loadErr}
		space := &mockSpaceService{}
		prompt := &mockPromptService{}
		a.spaceSvc = space
		a.prompt = prompt
		a.store = store

		err := a.runSpaces(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, loadErr)
	})

	t.Run("returns wrapped list error", func(t *testing.T) {
		t.Parallel()

		listSpacesErr := errors.New("list spaces")

		a := &app{out: &bytes.Buffer{}}
		store := &mockStateStore{state: &state.AppState{AnytypeAppKey: "app-key", Repositories: map[string]state.RepositoryState{}}}
		space := &mockSpaceService{listErr: listSpacesErr}
		prompt := &mockPromptService{}
		a.spaceSvc = space
		a.prompt = prompt
		a.store = store

		err := a.runSpaces(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, listSpacesErr)
	})

	t.Run("returns wrapped selection error", func(t *testing.T) {
		t.Parallel()

		selectOptionErr := errors.New("select option")

		a := &app{out: &bytes.Buffer{}}
		store := &mockStateStore{state: &state.AppState{AnytypeAppKey: "app-key", Repositories: map[string]state.RepositoryState{}}}
		space := &mockSpaceService{
			listValue: []spaces.Space{{ID: "space-1", Name: "Main"}},
		}
		prompt := &mockPromptService{chooseIndexErr: selectOptionErr}
		a.spaceSvc = space
		a.prompt = prompt
		a.store = store

		err := a.runSpaces(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, selectOptionErr)
	})

	t.Run("returns wrapped set default error", func(t *testing.T) {
		t.Parallel()

		setDefaultErr := errors.New("set default")

		a := &app{out: &bytes.Buffer{}}
		store := &mockStateStore{state: &state.AppState{AnytypeAppKey: "app-key", Repositories: map[string]state.RepositoryState{}}}
		space := &mockSpaceService{
			listValue:      []spaces.Space{{ID: "space-1", Name: "Main"}},
			setDefaultErr:  setDefaultErr,
			setDefaultArgs: []string{},
		}
		prompt := &mockPromptService{chooseIndexValues: []int{0}}
		a.spaceSvc = space
		a.prompt = prompt
		a.store = store

		err := a.runSpaces(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, setDefaultErr)
	})
}

func TestApp_RunSync(t *testing.T) {
	t.Parallel()

	t.Run("syncs with stored default space and prints action", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		a := &app{out: &out}

		store := &mockStateStore{
			state: &state.AppState{
				AnytypeAppKey: "app-key",
				DefaultSpace:  "space-1",
				Repositories:  map[string]state.RepositoryState{},
			},
		}
		space := &mockSpaceService{}
		repo := &mockRepoService{
			listValue: []github.Repository{
				{Owner: "octo", Name: "repo", FullName: "octo/repo", Private: true},
			},
		}
		sync := &mockSyncService{
			syncValue: syncing.Result{
				Action:   "updated",
				ObjectID: "obj-1",
				RepoFull: "octo/repo",
			},
		}
		prompt := &mockPromptService{chooseIndexValues: []int{0}}
		a.spaceSvc = space
		a.repoSvc = repo
		a.syncSvc = sync
		a.prompt = prompt
		a.store = store

		err := a.runSync(context.Background())
		require.NoError(t, err)
		assert.Contains(t, out.String(), "Updated Anytype page obj-1 for octo/repo")
		require.NotNil(t, sync.syncParams)
		assert.Equal(t, "octo", sync.syncParams.Owner)
		assert.Equal(t, "repo", sync.syncParams.Name)
		assert.Equal(t, "app-key", sync.syncParams.AppKey)
		assert.Equal(t, "space-1", sync.syncParams.SpaceID)
	})

	t.Run("returns wrapped repo list error", func(t *testing.T) {
		t.Parallel()

		listReposErr := errors.New("list repos")

		a := &app{out: &bytes.Buffer{}}
		store := &mockStateStore{
			state: &state.AppState{
				AnytypeAppKey: "app-key",
				DefaultSpace:  "space-1",
				Repositories:  map[string]state.RepositoryState{},
			},
		}
		space := &mockSpaceService{}
		repo := &mockRepoService{listErr: listReposErr}
		sync := &mockSyncService{}
		prompt := &mockPromptService{}
		a.spaceSvc = space
		a.repoSvc = repo
		a.syncSvc = sync
		a.prompt = prompt
		a.store = store

		err := a.runSync(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, listReposErr)
	})

	t.Run("returns wrapped sync error", func(t *testing.T) {
		t.Parallel()

		syncErr := errors.New("sync")

		a := &app{out: &bytes.Buffer{}}
		store := &mockStateStore{
			state: &state.AppState{
				AnytypeAppKey: "app-key",
				DefaultSpace:  "space-1",
				Repositories:  map[string]state.RepositoryState{},
			},
		}
		space := &mockSpaceService{}
		repo := &mockRepoService{
			listValue: []github.Repository{
				{Owner: "octo", Name: "repo", FullName: "octo/repo"},
			},
		}
		sync := &mockSyncService{syncErr: syncErr}
		prompt := &mockPromptService{chooseIndexValues: []int{0}}
		a.spaceSvc = space
		a.repoSvc = repo
		a.syncSvc = sync
		a.prompt = prompt
		a.store = store

		err := a.runSync(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, syncErr)
	})
}

func TestNewApp(t *testing.T) {
	t.Parallel()

	store := &mockStateStore{}
	authSvc := &mockAuthService{}
	spaceSvc := &mockSpaceService{}
	repoSvc := &mockRepoService{}
	syncSvc := &mockSyncService{}

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

var _ authService = (*mockAuthService)(nil)

type mockAuthService struct {
	startChallengeAppName string
	startChallengeValue   string
	startChallengeErr     error
	exchangeChallengeID   string
	exchangeCode          string
	exchangeValue         string
	exchangeErr           error
}

func (m *mockAuthService) StartChallenge(_ context.Context, appName string) (string, error) {
	m.startChallengeAppName = appName
	if m.startChallengeErr != nil {
		return "", m.startChallengeErr
	}
	return m.startChallengeValue, nil
}

func (m *mockAuthService) ExchangeCodeAndPersist(_ context.Context, challengeID, code string) (string, error) {
	m.exchangeChallengeID = challengeID
	m.exchangeCode = code
	if m.exchangeErr != nil {
		return "", m.exchangeErr
	}
	return m.exchangeValue, nil
}

type mockSpaceService struct {
	listValue      []spaces.Space
	listErr        error
	setDefaultErr  error
	setDefaultArgs []string
}

var _ spaceService = (*mockSpaceService)(nil)

func (m *mockSpaceService) List(_ context.Context, _ string) ([]spaces.Space, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listValue, nil
}

func (m *mockSpaceService) SetDefault(_ context.Context, spaceID string) error {
	m.setDefaultArgs = append(m.setDefaultArgs, spaceID)
	if m.setDefaultErr != nil {
		return m.setDefaultErr
	}
	return nil
}

type mockRepoService struct {
	listValue []github.Repository
	listErr   error
}

var _ repoService = (*mockRepoService)(nil)

func (m *mockRepoService) ListAccessibleRepos(_ context.Context) ([]github.Repository, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listValue, nil
}

type mockSyncService struct {
	syncParams *syncing.Params
	syncValue  syncing.Result
	syncErr    error
}

var _ syncService = (*mockSyncService)(nil)

func (m *mockSyncService) Sync(_ context.Context, params syncing.Params) (syncing.Result, error) {
	m.syncParams = &params
	if m.syncErr != nil {
		return syncing.Result{}, m.syncErr
	}
	return m.syncValue, nil
}

type mockPromptService struct {
	readLineValue     string
	readLineErr       error
	chooseIndexValues []int
	chooseIndexErr    error
	chooseCallCount   int
}

var _ promptService = (*mockPromptService)(nil)

func (m *mockPromptService) readLine(_ string) (string, error) {
	if m.readLineErr != nil {
		return "", m.readLineErr
	}
	return m.readLineValue, nil
}

func (m *mockPromptService) chooseIndex(_ string, _ []string) (int, error) {
	if m.chooseIndexErr != nil {
		return -1, m.chooseIndexErr
	}
	if m.chooseCallCount < len(m.chooseIndexValues) {
		value := m.chooseIndexValues[m.chooseCallCount]
		m.chooseCallCount++
		return value, nil
	}
	return 0, nil
}

var _ stateStore = (*mockStateStore)(nil)

type mockStateStore struct {
	state   *state.AppState
	loadErr error
	saveErr error
	path    string
}

func (m *mockStateStore) Load(context.Context) (*state.AppState, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.state == nil {
		m.state = state.NewAppState()
	}
	return m.state, nil
}

func (m *mockStateStore) Save(_ context.Context, state *state.AppState) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.state = state
	return nil
}

func (m *mockStateStore) Path() string {
	return m.path
}
