// data_models.go - Contains the data models for the game

package main

import (
	"github.com/senorbeast/atlas-backend/src/protobuf/game_proto"
)

// CreateGameRoomRequest is used to create a new game room
type CreateGameRoomRequest game_proto.CreateGameRoomRequest

// JoinGameRoomRequest is used to join an existing game room
type JoinGameRoomRequest game_proto.JoinGameRoomRequest

// GameEvent is used to handle game events
type GameEvent game_proto.GameEvent

// ... (Any additional data models you need, like player data or game data)
