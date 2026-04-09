package syncing

import (
	"context"
	"maps"

	"github.com/alesr/anytype-gh/internal/clients/github"
	"github.com/alesr/anytype-gh/internal/repositories/state"
)

var _ stateStore = (*mockStateStore)(nil)

type mockStateStore struct {
	loadFunc func(context.Context) (*state.AppState, error)
	saveFunc func(context.Context, *state.AppState) error
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

func cloneAppState(appState *state.AppState) *state.AppState {
	if appState == nil {
		return state.NewAppState()
	}

	copyState := *appState
	copyState.Repositories = make(map[string]state.RepositoryState, len(appState.Repositories))
	maps.Copy(copyState.Repositories, appState.Repositories)
	return &copyState
}

var _ gitHubGateway = (*mockGitHubGateway)(nil)

type mockGitHubGateway struct {
	GetReadmeFunc func(context.Context, string, string) (github.Readme, error)
}

func (m *mockGitHubGateway) GetReadme(ctx context.Context, owner, name string) (github.Readme, error) {
	if m.GetReadmeFunc == nil {
		return github.Readme{}, nil
	}
	return m.GetReadmeFunc(ctx, owner, name)
}
