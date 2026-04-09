package cli

import (
	"context"

	"github.com/alesr/anytype-gh/internal/clients/github"
	"github.com/alesr/anytype-gh/internal/repositories/state"
	"github.com/alesr/anytype-gh/internal/services/spaces"
	"github.com/alesr/anytype-gh/internal/services/syncing"
)

var _ authService = (*mockAuthService)(nil)

type mockAuthService struct {
	startChallengeFunc         func(context.Context, string) (string, error)
	exchangeCodeAndPersistFunc func(context.Context, string, string) (string, error)

	startChallengeAppName string
	exchangeChallengeID   string
	exchangeCode          string
}

func (m *mockAuthService) StartChallenge(ctx context.Context, appName string) (string, error) {
	m.startChallengeAppName = appName
	if m.startChallengeFunc == nil {
		return "", nil
	}
	return m.startChallengeFunc(ctx, appName)
}

func (m *mockAuthService) ExchangeCodeAndPersist(ctx context.Context, challengeID, code string) (string, error) {
	m.exchangeChallengeID = challengeID
	m.exchangeCode = code
	if m.exchangeCodeAndPersistFunc == nil {
		return "", nil
	}
	return m.exchangeCodeAndPersistFunc(ctx, challengeID, code)
}

type mockSpaceService struct {
	listFunc       func(context.Context, string) ([]spaces.Space, error)
	setDefaultFunc func(context.Context, string) error
	setDefaultArgs []string
}

var _ spaceService = (*mockSpaceService)(nil)

func (m *mockSpaceService) List(ctx context.Context, appKey string) ([]spaces.Space, error) {
	if m.listFunc == nil {
		return nil, nil
	}
	return m.listFunc(ctx, appKey)
}

func (m *mockSpaceService) SetDefault(ctx context.Context, spaceID string) error {
	m.setDefaultArgs = append(m.setDefaultArgs, spaceID)
	if m.setDefaultFunc == nil {
		return nil
	}
	return m.setDefaultFunc(ctx, spaceID)
}

type mockRepoService struct {
	listAccessibleReposFunc func(context.Context) ([]github.Repository, error)
}

var _ repoService = (*mockRepoService)(nil)

func (m *mockRepoService) ListAccessibleRepos(ctx context.Context) ([]github.Repository, error) {
	if m.listAccessibleReposFunc == nil {
		return nil, nil
	}
	return m.listAccessibleReposFunc(ctx)
}

type mockSyncService struct {
	syncFunc   func(context.Context, syncing.Params) (syncing.Result, error)
	syncParams *syncing.Params
}

var _ syncService = (*mockSyncService)(nil)

func (m *mockSyncService) Sync(ctx context.Context, params syncing.Params) (syncing.Result, error) {
	m.syncParams = &params
	if m.syncFunc == nil {
		return syncing.Result{}, nil
	}
	return m.syncFunc(ctx, params)
}

type mockPromptService struct {
	readLineFunc    func(string) (string, error)
	chooseIndexFunc func(string, []string) (int, error)
}

var _ promptService = (*mockPromptService)(nil)

func (m *mockPromptService) readLine(label string) (string, error) {
	if m.readLineFunc == nil {
		return "", nil
	}
	return m.readLineFunc(label)
}

func (m *mockPromptService) chooseIndex(label string, options []string) (int, error) {
	if m.chooseIndexFunc == nil {
		return 0, nil
	}
	return m.chooseIndexFunc(label, options)
}

var _ stateStore = (*mockStateStore)(nil)

type mockStateStore struct {
	loadFunc func(context.Context) (*state.AppState, error)
	saveFunc func(context.Context, *state.AppState) error
	path     string
}

func (m *mockStateStore) Load(ctx context.Context) (*state.AppState, error) {
	if m.loadFunc == nil {
		return state.NewAppState(), nil
	}
	return m.loadFunc(ctx)
}

func (m *mockStateStore) Save(ctx context.Context, appState *state.AppState) error {
	if m.saveFunc == nil {
		return nil
	}
	return m.saveFunc(ctx, appState)
}

func (m *mockStateStore) Path() string { return m.path }
