package game_room

// internal/game_room/game_room.go - Contains the game room handling logic and WebSocket server management

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

type PlayerConnection struct {
	Player *protobufs.PlayerData
	Conn   *websocket.Conn
}

type GameRoom struct {
	RoomID       string
	PlayerData   map[string]*PlayerConnection
	PlayersMux   sync.Mutex
	LastActivity time.Time
}
