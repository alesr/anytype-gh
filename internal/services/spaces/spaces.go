package spaces

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alesr/anytype-gh/internal/anytypefactory"
	"github.com/alesr/anytype-gh/internal/repositories/state"
)

type stateStore interface {
	Load(ctx context.Context) (*state.AppState, error)
	Save(ctx context.Context, state *state.AppState) error
}

type Space struct {
	ID          string
	Name        string
	Description string
}

type Service struct {
	anytype *anytypefactory.Factory
	state   stateStore
}

var (
	errSpaceIDRequired           = errors.New("space id is required")
	errCouldNotListAnytypeSpaces = errors.New("could not list anytype spaces")
	errLoadState                 = errors.New("load state")
	errSaveState                 = errors.New("save state")
)

func New(baseURL string, store stateStore) *Service {
	return &Service{anytype: anytypefactory.New(baseURL), state: store}
}

func (s *Service) List(ctx context.Context, appKey string) ([]Space, error) {
	client, err := s.anytype.AuthedClient(appKey)
	if err != nil {
		return nil, err
	}

	response, err := client.Spaces().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errCouldNotListAnytypeSpaces, err)
	}

	result := make([]Space, 0, len(response.Data))
	for _, item := range response.Data {
		result = append(result, Space{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
		})
	}
	return result, nil
}

func (s *Service) SetDefault(ctx context.Context, spaceID string) error {
	spaceID = strings.TrimSpace(spaceID)
	if spaceID == "" {
		return errSpaceIDRequired
	}
	currentState, err := s.state.Load(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", errLoadState, err)
	}
	currentState.DefaultSpace = spaceID
	if err := s.state.Save(ctx, currentState); err != nil {
		return fmt.Errorf("%w: %w", errSaveState, err)
	}
	return nil
}
