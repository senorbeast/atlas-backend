package games

import (
	"time"

	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

// AtlasWordKind is the first registered game implementation.
const AtlasWordKind = "atlas-word"

// PlayerAction is the domain-level action shape passed from rooms into games.
type PlayerAction struct {
	PlayerID   string
	PlayerName string
	Type       string
	CityName   string
	Now        time.Time
}

// ActionResult contains the event and latest snapshot produced by a valid action.
type ActionResult struct {
	Update *protobufs.GameUpdatePayload
	State  *protobufs.GameStatePayload
}

// Game is the extension point every multiplayer game must implement.
type Game interface {
	Kind() string
	Snapshot() *protobufs.GameStatePayload
	Apply(PlayerAction) (*ActionResult, *GameError)
	ExpiresAt() time.Time
	IsOver() bool
}

// MoveValidator can be implemented by games that split validation from mutation.
type MoveValidator interface {
	Validate(PlayerAction) *GameError
}

// GameError is a typed, client-safe validation or lifecycle error.
type GameError struct {
	Code    string
	Message string
}

// NewGame constructs a game implementation by kind.
func NewGame(kind string, now time.Time) (Game, *GameError) {
	switch kind {
	case "", AtlasWordKind:
		return NewAtlasWordGame(now)
	default:
		return nil, &GameError{Code: "unsupported_game", Message: "Unsupported game kind"}
	}
}
