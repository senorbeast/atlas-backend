package games

import (
	"time"

	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

const AtlasWordKind = "atlas-word"

type PlayerAction struct {
	PlayerID   string
	PlayerName string
	Type       string
	CityName   string
	Now        time.Time
}

type ActionResult struct {
	Update *protobufs.GameUpdatePayload
	State  *protobufs.GameStatePayload
}

type Game interface {
	Kind() string
	Snapshot() *protobufs.GameStatePayload
	Apply(PlayerAction) (*ActionResult, *GameError)
	ExpiresAt() time.Time
	IsOver() bool
}

type GameError struct {
	Code    string
	Message string
}

func NewGame(kind string, now time.Time) (Game, *GameError) {
	switch kind {
	case "", AtlasWordKind:
		return NewAtlasWordGame(now)
	default:
		return nil, &GameError{Code: "unsupported_game", Message: "Unsupported game kind"}
	}
}
