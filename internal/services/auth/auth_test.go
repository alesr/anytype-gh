package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alesr/anytype-gh/internal/repositories/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_StartChallenge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		appName           string
		challengeResponse string
		statusCode        int
		wantErrIs         []error
		wantChallengeID   string
	}{
		{
			name:              "success",
			appName:           "AnytypeReadmeSync",
			challengeResponse: `{"challenge_id":"challenge-123"}`,
			statusCode:        http.StatusOK,
			wantChallengeID:   "challenge-123",
		},
		{
			name:      "validation error",
			appName:   "  ",
			wantErrIs: []error{errAppNameRequired},
		},
		{
			name:              "api failure",
			appName:           "AnytypeReadmeSync",
			challengeResponse: `{"error":"boom"}`,
			statusCode:        http.StatusInternalServerError,
			wantErrIs:         []error{errCreateAnytypeChallenge},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &mockStateStore{
				loadFunc: func(context.Context) (*state.AppState, error) {
					return state.NewAppState(), nil
				},
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/v1/auth/challenges", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")

				if tc.statusCode == 0 {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(tc.statusCode)
				}
				_, _ = w.Write([]byte(tc.challengeResponse))
			}))
			defer server.Close()

			svc := New(server.URL, store)
			challengeID, err := svc.StartChallenge(context.TODO(), tc.appName)

			if len(tc.wantErrIs) > 0 {
				require.Error(t, err)
				for _, expectedErr := range tc.wantErrIs {
					assert.ErrorIs(t, err, expectedErr)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantChallengeID, challengeID)
		})
	}
}

func TestService_ExchangeCodeAndPersist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		challengeID     string
		code            string
		statusCode      int
		apiKeyResponse  string
		loadErr         error
		saveErr         error
		wantErrIs       []error
		wantSavedAppKey string
		wantStateSaves  int
	}{
		{
			name:            "success",
			challengeID:     "challenge-1",
			code:            "1234",
			statusCode:      http.StatusOK,
			apiKeyResponse:  `{"api_key":"app-key-123"}`,
			wantSavedAppKey: "app-key-123",
			wantStateSaves:  1,
		},
		{
			name:        "missing challenge id",
			challengeID: "",
			code:        "1234",
			wantErrIs:   []error{errChallengeIDRequired},
		},
		{
			name:        "missing verification code",
			challengeID: "challenge-1",
			code:        " ",
			wantErrIs:   []error{errVerificationCodeRequired},
		},
		{
			name:           "api failure",
			challengeID:    "challenge-1",
			code:           "1234",
			statusCode:     http.StatusInternalServerError,
			apiKeyResponse: `{"error":"boom"}`,
			wantErrIs:      []error{errCreateAnytypeAppKey},
		},
		{
			name:           "state load error",
			challengeID:    "challenge-1",
			code:           "1234",
			statusCode:     http.StatusOK,
			apiKeyResponse: `{"api_key":"app-key-123"}`,
			loadErr:        errors.New("load failed"),
			wantErrIs:      []error{errLoadState},
		},
		{
			name:           "state save error",
			challengeID:    "challenge-1",
			code:           "1234",
			statusCode:     http.StatusOK,
			apiKeyResponse: `{"api_key":"app-key-123"}`,
			saveErr:        errors.New("save failed"),
			wantErrIs:      []error{errSaveState},
			wantStateSaves: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			currentState := state.NewAppState()
			saveCalls := 0
			store := &mockStateStore{
				loadFunc: func(context.Context) (*state.AppState, error) {
					if tc.loadErr != nil {
						return nil, tc.loadErr
					}
					return currentState, nil
				},
				saveFunc: func(_ context.Context, appState *state.AppState) error {
					saveCalls++
					if tc.saveErr != nil {
						return tc.saveErr
					}
					currentState.AnytypeAppKey = appState.AnytypeAppKey
					return nil
				},
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/v1/auth/api_keys", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")

				if tc.statusCode == 0 {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(tc.statusCode)
				}
				_, _ = w.Write([]byte(tc.apiKeyResponse))
			}))
			defer server.Close()

			svc := New(server.URL, store)
			apiKey, err := svc.ExchangeCodeAndPersist(context.TODO(), tc.challengeID, tc.code)

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

			if tc.wantSavedAppKey != "" {
				assert.Equal(t, tc.wantSavedAppKey, apiKey)
				assert.Equal(t, tc.wantSavedAppKey, currentState.AnytypeAppKey)
			}
			assert.Equal(t, tc.wantStateSaves, saveCalls)
		})
	}
}
