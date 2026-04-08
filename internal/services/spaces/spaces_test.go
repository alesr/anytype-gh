package spaces

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alesr/anytype-gh/internal/anytypefactory"
	"github.com/alesr/anytype-gh/internal/repositories/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_List(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		appKey     string
		statusCode int
		response   string
		wantErrIs  []error
		wantCount  int
	}{
		{
			name:       "success",
			appKey:     "app-key",
			statusCode: http.StatusOK,
			response:   `{"data":[{"id":"space-1","name":"Main","description":"Primary"}],"pagination":{"total":1,"limit":100,"offset":0,"has_more":false}}`,
			wantCount:  1,
		},
		{
			name:      "missing app key",
			appKey:    " ",
			wantErrIs: []error{anytypefactory.ErrAppKeyRequired},
		},
		{
			name:       "api failure",
			appKey:     "app-key",
			statusCode: http.StatusInternalServerError,
			response:   `{"error":"boom"}`,
			wantErrIs:  []error{errCouldNotListAnytypeSpaces},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &mockStateStore{state: state.NewAppState()}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/v1/spaces", r.URL.Path)
				assert.Equal(t, "Bearer app-key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				if tc.statusCode == 0 {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(tc.statusCode)
				}
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			svc := New(server.URL, store)
			spaceList, err := svc.List(context.Background(), tc.appKey)
			if len(tc.wantErrIs) > 0 {
				require.Error(t, err)
				for _, expectedErr := range tc.wantErrIs {
					assert.ErrorIs(t, err, expectedErr)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, spaceList, tc.wantCount)
		})
	}
}

func TestService_SetDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		spaceID       string
		loadErr       error
		saveErr       error
		wantErrIs     []error
		wantSaveCalls int
	}{
		{name: "success", spaceID: "space-1", wantSaveCalls: 1},
		{name: "missing space id", spaceID: "  ", wantErrIs: []error{errSpaceIDRequired}},
		{name: "load error", spaceID: "space-1", loadErr: errors.New("load failed"), wantErrIs: []error{errLoadState}},
		{name: "save error", spaceID: "space-1", saveErr: errors.New("save failed"), wantErrIs: []error{errSaveState}, wantSaveCalls: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &mockStateStore{state: state.NewAppState(), loadErr: tc.loadErr, saveErr: tc.saveErr}
			svc := New("http://localhost:31009", store)

			err := svc.SetDefault(context.Background(), tc.spaceID)
			if len(tc.wantErrIs) > 0 {
				require.Error(t, err)
				for _, expectedErr := range tc.wantErrIs {
					assert.ErrorIs(t, err, expectedErr)
				}
				if tc.loadErr != nil {
					assert.ErrorIs(t, err, tc.loadErr)
				}
				if tc.saveErr != nil {
					assert.ErrorIs(t, err, tc.saveErr)
				}
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.wantSaveCalls, store.saveCalls)
			if len(tc.wantErrIs) == 0 {
				assert.Equal(t, tc.spaceID, store.state.DefaultSpace)
			}
		})
	}
}

type mockStateStore struct {
	state     *state.AppState
	loadErr   error
	saveErr   error
	saveCalls int
}

var _ stateStore = (*mockStateStore)(nil)

func (m *mockStateStore) Load(context.Context) (*state.AppState, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.state == nil {
		m.state = state.NewAppState()
	}
	return m.state, nil
}

func (m *mockStateStore) Save(_ context.Context, appState *state.AppState) error {
	m.saveCalls++
	if m.saveErr != nil {
		return m.saveErr
	}
	m.state = appState
	return nil
}
