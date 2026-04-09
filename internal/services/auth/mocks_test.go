package auth

import (
	"context"

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
