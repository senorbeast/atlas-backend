package game_room

// internal/game_room/data_models.go - Contains the data models for the game

import (
	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

// CreateGameRoomRequest is used to create a new game room
type CreateGameRoomRequest protobufs.CreateGameRoomRequest

// JoinGameRoomRequest is used to join an existing game room
type JoinGameRoomRequest protobufs.JoinGameRoomRequest

// GameEvent is used to handle game events
type GameEvent protobufs.GameEvent

// ... (Any additional data models you need, like player data or game data)
