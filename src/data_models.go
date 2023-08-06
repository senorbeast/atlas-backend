// data_models.go - Contains the data models for the game

package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	multiplayer "atlas-backend/protobuf" // Import the generated Go code
)

// CreateGameRoomRequest is used to create a new game room
type CreateGameRoomRequest multiplayer.CreateGameRoomRequest

// JoinGameRoomRequest is used to join an existing game room
type JoinGameRoomRequest multiplayer.JoinGameRoomRequest

// GameEvent is used to handle game events
type GameEvent multiplayer.GameEvent

// ... (Any additional data models you need, like player data or game data)
