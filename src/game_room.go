// game_room.go - Contains the game room handling logic and WebSocket server management

package main

import (
	"sync"
	"time"

	"github.com/senorbeast/atlas-backend/src/protobuf/game_proto"
)

type GameRoom struct {
	RoomID       string
	players      map[string]*game_proto.PlayerData
	playersMux   sync.Mutex
	lastActivity time.Time // Track the last activity time
}

