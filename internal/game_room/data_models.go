package game_room

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
	RoomID              string
	Players             map[string]*PlayerConnection
	PlayersMux          sync.Mutex
	LastActivity        time.Time
	Category            string
	IsStarted           bool
	IsOver              bool
	TurnPlayerID        string
	CurrentLetter       string
	LastWord            string
	TurnStartTime       int64
	HostPlayerID        string
	TurnDurationSeconds int32
}

// NewGameRoom creates a new game room
func NewGameRoom(roomID string) *GameRoom {
	return &GameRoom{
		RoomID:       roomID,
		Players:      make(map[string]*PlayerConnection),
		LastActivity: time.Now(),
	}
}