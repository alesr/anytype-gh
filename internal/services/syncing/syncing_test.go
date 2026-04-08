package syncing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alesr/anytype-gh/internal/clients/github"
	"github.com/alesr/anytype-gh/internal/repositories/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errGitHubBoom = errors.New("github boom")

func TestService_Sync(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		params           Params
		initialState     *state.AppState
		githubReadme     github.Readme
		githubErr        error
		loadErr          error
		saveErr          error
		listStatus       int
		listResponse     string
		createStatus     int
		createResponse   string
		updateStatus     int
		updateResponse   string
		wantAction       string
		wantErrIs        []error
		wantObjectID     string
		wantListCalls    int
		wantCreateCalls  int
		wantUpdateCalls  int
		wantSaveCalls    int
		wantStateSHA     string
		wantStateSpaceID string
	}{
		{
			name: "creates page on first sync",
			params: Params{
				Owner:   "octo",
				Name:    "private-repo",
				AppKey:  "app-key",
				SpaceID: "space-1",
			},
			initialState: state.NewAppState(),
			githubReadme: github.Readme{
				RepoFullName: "octo/private-repo",
				SHA:          "sha-1",
				Content:      "# readme",
			},
			createResponse:   `{"object":{"id":"obj-1"}}`,
			wantAction:       "created",
			wantObjectID:     "obj-1",
			wantListCalls:    1,
			wantCreateCalls:  1,
			wantUpdateCalls:  0,
			wantSaveCalls:    1,
			wantStateSHA:     "sha-1",
			wantStateSpaceID: "space-1",
		},
		{
			name: "updates existing page discovered by title",
			params: Params{
				Owner:   "octo",
				Name:    "private-repo",
				AppKey:  "app-key",
				SpaceID: "space-1",
			},
			initialState: state.NewAppState(),
			githubReadme: github.Readme{
				RepoFullName: "octo/private-repo",
				SHA:          "sha-new",
				Content:      "# updated",
			},
			listResponse:     `{"data":[{"id":"obj-existing","name":"README - octo/private-repo","archived":false}]}`,
			wantAction:       "updated",
			wantObjectID:     "obj-existing",
			wantListCalls:    1,
			wantCreateCalls:  0,
			wantUpdateCalls:  1,
			wantSaveCalls:    1,
			wantStateSHA:     "sha-new",
			wantStateSpaceID: "space-1",
		},
		{
			name: "skips when sha unchanged",
			params: Params{
				Owner:   "octo",
				Name:    "private-repo",
				AppKey:  "app-key",
				SpaceID: "space-1",
			},
			initialState: withRepoState(
				"octo/private-repo", state.RepositoryState{
					ObjectID:      "obj-1",
					LastReadmeSHA: "sha-1",
				},
			),
			githubReadme: github.Readme{
				RepoFullName: "octo/private-repo",
				SHA:          "sha-1",
				Content:      "# readme",
			},
			listResponse:     `{"data":[{"id":"obj-1","name":"README - octo/private-repo","archived":false}]}`,
			wantAction:       "skipped",
			wantObjectID:     "obj-1",
			wantListCalls:    1,
			wantCreateCalls:  0,
			wantUpdateCalls:  0,
			wantSaveCalls:    0,
			wantStateSHA:     "sha-1",
			wantStateSpaceID: "",
		},
		{
			name: "recreates when stored object was deleted and sha unchanged",
			params: Params{
				Owner:   "octo",
				Name:    "private-repo",
				AppKey:  "app-key",
				SpaceID: "space-1",
			},
			initialState: withRepoState(
				"octo/private-repo", state.RepositoryState{
					ObjectID:      "missing-id",
					LastReadmeSHA: "sha-1",
				},
			),
			githubReadme: github.Readme{
				RepoFullName: "octo/private-repo",
				SHA:          "sha-1",
				Content:      "# readme",
			},
			createResponse:   `{"object":{"id":"obj-recreated"}}`,
			wantAction:       "created",
			wantObjectID:     "obj-recreated",
			wantListCalls:    1,
			wantCreateCalls:  1,
			wantUpdateCalls:  0,
			wantSaveCalls:    1,
			wantStateSHA:     "sha-1",
			wantStateSpaceID: "space-1",
		},
		{
			name: "recreates when update returns not found",
			params: Params{
				Owner:   "octo",
				Name:    "private-repo",
				AppKey:  "app-key",
				SpaceID: "space-1",
			},
			initialState: withRepoState(
				"octo/private-repo", state.RepositoryState{
					ObjectID:      "missing-id",
					LastReadmeSHA: "sha-old",
				},
			),
			githubReadme: github.Readme{
				RepoFullName: "octo/private-repo",
				SHA:          "sha-new",
				Content:      "# readme",
			},
			updateStatus:     http.StatusNotFound,
			updateResponse:   `{"error":"missing"}`,
			createResponse:   `{"object":{"id":"obj-new"}}`,
			wantAction:       "recreated",
			wantObjectID:     "obj-new",
			wantListCalls:    0,
			wantCreateCalls:  1,
			wantUpdateCalls:  1,
			wantSaveCalls:    1,
			wantStateSHA:     "sha-new",
			wantStateSpaceID: "space-1",
		},
		{
			name:         "validates owner and name",
			params:       Params{Name: "repo", AppKey: "app-key", SpaceID: "space-1"},
			initialState: state.NewAppState(),
			wantErrIs:    []error{errRepositoryOwnerNameRequired},
		},
		{
			name:         "validates space id",
			params:       Params{Owner: "octo", Name: "repo", AppKey: "app-key", SpaceID: "  "},
			initialState: state.NewAppState(),
			wantErrIs:    []error{errAnytypeSpaceIDRequired},
		},
		{
			name:         "propagates github readme error",
			params:       Params{Owner: "octo", Name: "repo", AppKey: "app-key", SpaceID: "space-1"},
			initialState: state.NewAppState(),
			githubErr:    errGitHubBoom,
			wantErrIs:    []error{errGitHubBoom},
		},
		{
			name:         "wraps state load error",
			params:       Params{Owner: "octo", Name: "repo", AppKey: "app-key", SpaceID: "space-1"},
			initialState: state.NewAppState(),
			githubReadme: github.Readme{RepoFullName: "octo/repo", SHA: "sha-1", Content: "# readme"},
			loadErr:      errors.New("load failed"),
			wantErrIs:    []error{errLoadState},
		},
		{
			name:            "wraps state save error",
			params:          Params{Owner: "octo", Name: "repo", AppKey: "app-key", SpaceID: "space-1"},
			initialState:    state.NewAppState(),
			githubReadme:    github.Readme{RepoFullName: "octo/repo", SHA: "sha-1", Content: "# readme"},
			saveErr:         errors.New("save failed"),
			wantErrIs:       []error{errSaveState},
			wantListCalls:   1,
			wantCreateCalls: 1,
			wantUpdateCalls: 0,
			wantSaveCalls:   1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.initialState == nil {
				tc.initialState = state.NewAppState()
			}

			serverState := &anytypeServerState{}
			server := newAnytypeTestServer(t, tc, serverState)
			defer server.Close()

			github := &mockGitHubGateway{readme: tc.githubReadme, err: tc.githubErr}
			store := &mockStateStore{state: tc.initialState, loadErr: tc.loadErr, saveErr: tc.saveErr}
			svc := New(github, server.URL, store)
			svc.clock = fixedClock{value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

			result, err := svc.Sync(context.Background(), tc.params)

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
				assert.Equal(t, tc.wantAction, result.Action)
				assert.Equal(t, tc.wantObjectID, result.ObjectID)
			}

			assert.Equal(t, tc.wantListCalls, int(serverState.listCalls.Load()))
			assert.Equal(t, tc.wantCreateCalls, int(serverState.createCalls.Load()))
			assert.Equal(t, tc.wantUpdateCalls, int(serverState.updateCalls.Load()))
			assert.Equal(t, tc.wantSaveCalls, store.saveCalls)

			if tc.wantStateSHA != "" {
				repoState := store.state.Repositories[tc.githubReadme.RepoFullName]
				assert.Equal(t, tc.wantStateSHA, repoState.LastReadmeSHA)
				assert.Equal(t, tc.wantStateSpaceID, repoState.SpaceID)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "plain 404 status text",
			err:  errors.New("request failed with status 404"),
			want: true,
		},
		{
			name: "wrapped 404",
			err:  fmt.Errorf("wrapper: %w", errors.New("status 404 body")),
			want: true,
		},
		{
			name: "different status",
			err:  errors.New("request failed with status 403"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isNotFound(tc.err))
		})
	}
}

type anytypeServerState struct {
	listCalls   atomic.Int32
	createCalls atomic.Int32
	updateCalls atomic.Int32
}

func newAnytypeTestServer(t *testing.T, tc struct {
	name             string
	params           Params
	initialState     *state.AppState
	githubReadme     github.Readme
	githubErr        error
	loadErr          error
	saveErr          error
	listStatus       int
	listResponse     string
	createStatus     int
	createResponse   string
	updateStatus     int
	updateResponse   string
	wantAction       string
	wantErrIs        []error
	wantObjectID     string
	wantListCalls    int
	wantCreateCalls  int
	wantUpdateCalls  int
	wantSaveCalls    int
	wantStateSHA     string
	wantStateSpaceID string
}, serverState *anytypeServerState,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tc.params.AppKey != "" {
			assert.Equal(t, "Bearer "+tc.params.AppKey, r.Header.Get("Authorization"))
		}

		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/objects") {
			serverState.listCalls.Add(1)
			writeJSONResponse(w, defaultStatus(tc.listStatus, http.StatusOK), defaultListResponse(tc.listResponse))
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/objects") {
			serverState.createCalls.Add(1)
			writeJSONResponse(w, defaultStatus(tc.createStatus, http.StatusOK), defaultCreateResponse(tc.createResponse))
			return
		}
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/objects/") {
			serverState.updateCalls.Add(1)
			writeJSONResponse(w, defaultStatus(tc.updateStatus, http.StatusOK), defaultUpdateResponse(r.URL.Path, tc.updateResponse))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

func defaultListResponse(response string) string {
	if response != "" {
		return response
	}
	return `{"data":[],"pagination":{"total":0,"limit":100,"offset":0,"has_more":false}}`
}

func defaultCreateResponse(response string) string {
	if response != "" {
		return response
	}
	return `{"object":{"id":"created-id"}}`
}

func defaultUpdateResponse(path string, response string) string {
	if response != "" {
		return response
	}
	parts := strings.Split(path, "/")
	objectID := parts[len(parts)-1]
	return fmt.Sprintf(`{"object":{"id":"%s"}}`, objectID)
}

func defaultStatus(statusCode int, fallback int) int {
	if statusCode == 0 {
		return fallback
	}
	return statusCode
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(body))
}

func withRepoState(repo string, repoState state.RepositoryState) *state.AppState {
	appState := state.NewAppState()
	appState.Repositories[repo] = repoState
	return appState
}

type mockGitHubGateway struct {
	readme github.Readme
	err    error
}

var _ gitHubGateway = (*mockGitHubGateway)(nil)

func (m *mockGitHubGateway) GetReadme(context.Context, string, string) (github.Readme, error) {
	if m.err != nil {
		return github.Readme{}, m.err
	}
	return m.readme, nil
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

	copyState := *m.state
	copyState.Repositories = make(map[string]state.RepositoryState, len(m.state.Repositories))
	for key, value := range m.state.Repositories {
		copyState.Repositories[key] = value
	}
	return &copyState, nil
}

func (m *mockStateStore) Save(_ context.Context, appState *state.AppState) error {
	m.saveCalls++
	if m.saveErr != nil {
		return m.saveErr
	}

	copyState := *appState
	copyState.Repositories = make(map[string]state.RepositoryState, len(appState.Repositories))
	for key, value := range appState.Repositories {
		copyState.Repositories[key] = value
	}
	m.state = &copyState
	return nil
}

type fixedClock struct {
	value time.Time
}

func (f fixedClock) Now() time.Time {
	return f.value
}
