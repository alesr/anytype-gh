package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alesr/anytype-gh/internal/anytypefactory"
	"github.com/alesr/anytype-gh/internal/repositories/state"
)

var (
	errAppNameRequired          = errors.New("app name is required")
	errChallengeIDRequired      = errors.New("challenge id is required")
	errVerificationCodeRequired = errors.New("verification code is required")
	errCreateAnytypeChallenge   = errors.New("could not create anytype challenge")
	errCreateAnytypeAppKey      = errors.New("could not create anytype app key")
	errLoadState                = errors.New("load state")
	errSaveState                = errors.New("save state")
)

type stateStore interface {
	Load(ctx context.Context) (*state.AppState, error)
	Save(ctx context.Context, state *state.AppState) error
}

// Service isolates Anytype auth policy so callers do not need to reason
// about challenge/app-key protocol details.
type Service struct {
	anytype *anytypefactory.Factory
	state   stateStore
}

// New binds auth logic to a concrete API endpoint and local state store.
func New(baseURL string, store stateStore) *Service {
	return &Service{anytype: anytypefactory.New(baseURL), state: store}
}

func (s *Service) StartChallenge(ctx context.Context, appName string) (string, error) {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		return "", errAppNameRequired
	}

	client := s.anytype.UnauthedClient()

	response, err := client.Auth().CreateChallenge(ctx, appName)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errCreateAnytypeChallenge, err)
	}

	if response == nil || strings.TrimSpace(response.ChallengeID) == "" {
		return "", errors.New("anytype challenge response did not include a challenge id")
	}
	return response.ChallengeID, nil
}

func (s *Service) ExchangeCodeAndPersist(ctx context.Context, challengeID, code string) (string, error) {
	if strings.TrimSpace(challengeID) == "" {
		return "", errChallengeIDRequired
	}

	if strings.TrimSpace(code) == "" {
		return "", errVerificationCodeRequired
	}

	client := s.anytype.UnauthedClient()

	response, err := client.Auth().CreateApiKey(ctx, challengeID, code)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errCreateAnytypeAppKey, err)
	}

	if response == nil || strings.TrimSpace(response.ApiKey) == "" {
		return "", errors.New("anytype api key response did not include an app key")
	}

	currentState, err := s.state.Load(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errLoadState, err)
	}

	currentState.AnytypeAppKey = response.ApiKey
	if err := s.state.Save(ctx, currentState); err != nil {
		return "", fmt.Errorf("%w: %w", errSaveState, err)
	}
	return response.ApiKey, nil
}
