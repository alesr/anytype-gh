package state

import "time"

// StateSchemaVersion protects future migrations by making persisted format changes explicit.
const StateSchemaVersion = 1

// RepositoryState stores enough history to make sync idempotent across runs.
type RepositoryState struct {
	ObjectID      string    `json:"object_id"`
	LastReadmeSHA string    `json:"last_readme_sha"`
	LastSyncedAt  time.Time `json:"last_synced_at"`
	SpaceID       string    `json:"space_id"`
}

// AppState captures long-lived user choices and sync mappings that should survive process restarts.
type AppState struct {
	SchemaVersion int                        `json:"schema_version"`
	AnytypeAppKey string                     `json:"anytype_app_key"`
	DefaultSpace  string                     `json:"default_space"`
	Repositories  map[string]RepositoryState `json:"repositories"`
}

// NewAppState guarantees map initialization so callers can mutate state without defensive nil checks.
func NewAppState() *AppState {
	return &AppState{
		SchemaVersion: StateSchemaVersion,
		Repositories:  map[string]RepositoryState{},
	}
}

// EnsureDefaults allows forward-compatible reads of older or partial state payloads.
func (s *AppState) EnsureDefaults() {
	if s.SchemaVersion == 0 {
		s.SchemaVersion = StateSchemaVersion
	}

	if s.Repositories == nil {
		s.Repositories = map[string]RepositoryState{}
	}
}
