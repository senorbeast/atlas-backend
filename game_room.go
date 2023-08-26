// game_room.go - Contains the game room handling logic and WebSocket server management

package main

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/protobuf/game_proto"
)

type PlayerConnection struct {
	Player *game_proto.PlayerData
	Conn   *websocket.Conn
}

type GameRoom struct {
	RoomID       string
	playerData   map[string]*PlayerConnection
	playersMux   sync.Mutex
	lastActivity time.Time
}
