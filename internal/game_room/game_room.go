package game_room

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/games"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

const (
	DefaultGameKind    = games.AtlasWordKind
	RoomStatusWaiting  = "waiting"
	RoomStatusPlaying  = "playing"
	RoomStatusEnded    = "ended"
	TurnModeStrict     = "strict-turns"
	TurnModeFreeForAll = "free-for-all"

	defaultMaxPlayers = 8
	emptyRoomTTL      = 3 * time.Minute
)

// GameRoom owns all mutable in-memory state for a single multiplayer room.
type GameRoom struct {
	mu           sync.Mutex
	RoomID       string
	GameKind     string
	TurnMode     string
	TurnIndex    int
	TurnOrder    []string
	HasGameMove  bool
	Status       string
	MaxPlayers   int
	CreatedAt    time.Time
	StartedAt    time.Time
	ExpiresAt    time.Time
	LastActivity time.Time
	EmptySince   *time.Time
	Players      map[string]*protobufs.PlayerData
	Connections  map[string]*websocket.Conn
	ChatHistory  []ChatMessage
	Game         games.Game
}

// RoomManager coordinates in-memory rooms and is the public orchestration entrypoint.
type RoomManager struct {
	mu    sync.Mutex
	rooms map[string]*GameRoom
}

// NewRoomManager creates an empty in-memory room registry.
func NewRoomManager() *RoomManager {
	return &RoomManager{rooms: make(map[string]*GameRoom)}
}

// CreateRoom creates a room with the requested game kind and turn mode.
func (m *RoomManager) CreateRoom(roomID string, gameKind string, turnMode string, now time.Time) (*GameRoom, error) {
	kind := strings.TrimSpace(gameKind)
	if kind == "" {
		kind = DefaultGameKind
	}
	mode, err := normalizeTurnMode(turnMode)
	if err != nil {
		return nil, err
	}
	game, gameErr := games.NewGame(kind, now)
	if gameErr != nil {
		return nil, errors.New(gameErr.Message)
	}

	room := &GameRoom{
		RoomID:       roomID,
		GameKind:     game.Kind(),
		TurnMode:     mode,
		Status:       RoomStatusWaiting,
		MaxPlayers:   defaultMaxPlayers,
		CreatedAt:    now,
		StartedAt:    now,
		ExpiresAt:    game.ExpiresAt(),
		LastActivity: now,
		Players:      make(map[string]*protobufs.PlayerData),
		Connections:  make(map[string]*websocket.Conn),
		ChatHistory:  []ChatMessage{},
		Game:         game,
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.rooms[roomID] = room
	return room, nil
}

// GetRoom looks up a room by id.
func (m *RoomManager) GetRoom(roomID string) (*GameRoom, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	room, ok := m.rooms[roomID]
	return room, ok
}

// RemoveRoom deletes a room and closes active websocket connections.
func (m *RoomManager) RemoveRoom(roomID string) {
	m.mu.Lock()
	room, ok := m.rooms[roomID]
	if ok {
		delete(m.rooms, roomID)
	}
	m.mu.Unlock()

	if ok {
		room.closeConnections()
	}
}

// CleanupExpired removes rooms that are past expiry or have been empty for the cleanup TTL.
func (m *RoomManager) CleanupExpired(now time.Time) []string {
	m.mu.Lock()
	var expired []*GameRoom
	for roomID, room := range m.rooms {
		if room.shouldCleanup(now) {
			delete(m.rooms, roomID)
			expired = append(expired, room)
		}
	}
	m.mu.Unlock()

	removed := make([]string, 0, len(expired))
	for _, room := range expired {
		removed = append(removed, room.RoomID)
		room.closeConnections()
	}
	return removed
}

// JoinRoom validates player identity, registers the connection, and returns an initial snapshot ACK.
func (m *RoomManager) JoinRoom(roomID string, conn *websocket.Conn, displayName string, gameKind string, turnMode string, now time.Time) (*protobufs.OnConnectAckPayload, string, *games.GameError) {
	room, ok := m.GetRoom(roomID)
	if !ok {
		return nil, "", &games.GameError{Code: "room_not_found", Message: "Room not found"}
	}

	name := normalizeDisplayName(displayName)
	if name == "" {
		return nil, "", &games.GameError{Code: "invalid_player_name", Message: "Player name is required"}
	}
	if len([]rune(name)) > 32 {
		return nil, "", &games.GameError{Code: "invalid_player_name", Message: "Player name must be 32 characters or fewer"}
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	if room.Status == RoomStatusEnded || room.Game.IsOver() || now.After(room.ExpiresAt) {
		room.Status = RoomStatusEnded
		return nil, "", &games.GameError{Code: "room_closed", Message: "Room is closed"}
	}
	if gameKind != "" && gameKind != room.GameKind {
		return nil, "", &games.GameError{Code: "game_kind_mismatch", Message: "Room uses a different game kind"}
	}
	if turnMode != "" && turnMode != room.TurnMode {
		return nil, "", &games.GameError{Code: "turn_mode_mismatch", Message: "Room uses a different turn mode"}
	}
	if len(room.Connections) >= room.MaxPlayers {
		return nil, "", &games.GameError{Code: "room_full", Message: "Room is full"}
	}

	playerID := generateID("player")
	player := &protobufs.PlayerData{
		PlayerId:   playerID,
		Name:       name,
		Hearts:     1,
		Score:      0,
		Connected:  true,
		JoinedAt:   now.UTC().Format(time.RFC3339Nano),
		LastSeenAt: now.UTC().Format(time.RFC3339Nano),
	}
	room.Players[playerID] = player
	room.Connections[playerID] = conn
	room.TurnOrder = append(room.TurnOrder, playerID)
	room.LastActivity = now
	room.EmptySince = nil
	if room.TurnMode == TurnModeStrict && !room.HasGameMove {
		room.randomizeCurrentTurnLocked(now)
		room.ensureCurrentTurnLocked()
	}
	if room.Status == RoomStatusWaiting {
		room.Status = RoomStatusPlaying
	}

	ack := &protobufs.OnConnectAckPayload{
		RoomId:    room.RoomID,
		PlayerId:  playerID,
		GameKind:  room.GameKind,
		TurnMode:  room.TurnMode,
		Room:      room.snapshotLocked(),
		GameState: room.gameStateLocked(),
	}
	return ack, playerID, nil
}

// Disconnect removes the player connection and returns a room snapshot for broadcast.
func (m *RoomManager) Disconnect(roomID string, playerID string, now time.Time) *protobufs.RoomUpdatePayload {
	room, ok := m.GetRoom(roomID)
	if !ok {
		return nil
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	currentTurnPlayerID := room.currentTurnPlayerIDLocked()
	disconnectedIndex := room.turnOrderIndexLocked(playerID)
	if conn, exists := room.Connections[playerID]; exists {
		if conn != nil {
			_ = conn.Close()
		}
		delete(room.Connections, playerID)
	}
	delete(room.Players, playerID)
	room.removeFromTurnOrderLocked(playerID)
	if room.TurnMode == TurnModeStrict {
		if currentTurnPlayerID == playerID {
			room.clampTurnIndexLocked()
		} else if currentTurnPlayerID != "" && disconnectedIndex >= 0 && disconnectedIndex < room.TurnIndex {
			room.TurnIndex--
			room.clampTurnIndexLocked()
		} else if currentTurnPlayerID != "" {
			room.setTurnIndexForPlayerLocked(currentTurnPlayerID)
		}
		room.ensureCurrentTurnLocked()
	}
	room.LastActivity = now
	if len(room.Connections) == 0 {
		room.EmptySince = &now
	}
	return &protobufs.RoomUpdatePayload{Room: room.snapshotLocked()}
}

// ApplyGameUpdate validates and applies a player game action, returning game and room broadcasts.
func (m *RoomManager) ApplyGameUpdate(roomID string, playerID string, payload *protobufs.GameUpdatePayload, now time.Time) (*games.ActionResult, *protobufs.RoomUpdatePayload, *games.GameError) {
	room, ok := m.GetRoom(roomID)
	if !ok {
		return nil, nil, &games.GameError{Code: "room_not_found", Message: "Room not found"}
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	player, ok := room.Players[playerID]
	if !ok {
		return nil, nil, &games.GameError{Code: "unknown_player", Message: "Player is not in the room"}
	}
	if room.TurnMode == TurnModeStrict {
		currentTurnPlayerID := room.currentTurnPlayerIDLocked()
		if currentTurnPlayerID == "" {
			return nil, nil, &games.GameError{Code: "no_connected_players", Message: "No connected players are available"}
		}
		if currentTurnPlayerID != playerID {
			return nil, nil, &games.GameError{Code: "not_your_turn", Message: "It is not your turn"}
		}
	}
	room.LastActivity = now

	result, gameErr := room.Game.Apply(games.PlayerAction{
		PlayerID:   playerID,
		PlayerName: player.Name,
		Type:       payload.GetType(),
		CityName:   payload.GetCityName(),
		Now:        now,
	})
	if gameErr != nil {
		return nil, nil, gameErr
	}

	if result.Update != nil && result.Update.GetType() == "city_accepted" {
		defaultScorePolicy.ApplyAcceptedMove(player)
		room.HasGameMove = true
		if room.TurnMode == TurnModeStrict {
			room.advanceTurnLocked()
		}
	}
	if room.Game.IsOver() {
		room.Status = RoomStatusEnded
	}

	result.State = room.gameStateLocked()
	return result, &protobufs.RoomUpdatePayload{Room: room.snapshotLocked()}, nil
}

// GameState returns the current authoritative game snapshot for a room.
func (m *RoomManager) GameState(roomID string) (*protobufs.GameStatePayload, *games.GameError) {
	room, ok := m.GetRoom(roomID)
	if !ok {
		return nil, &games.GameError{Code: "room_not_found", Message: "Room not found"}
	}
	room.mu.Lock()
	defer room.mu.Unlock()
	return room.gameStateLocked(), nil
}

func (r *GameRoom) shouldCleanup(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now.After(r.ExpiresAt) || r.Game.IsOver() {
		r.Status = RoomStatusEnded
		return true
	}
	return r.EmptySince != nil && now.Sub(*r.EmptySince) >= emptyRoomTTL
}

func (r *GameRoom) closeConnections() {
	r.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(r.Connections))
	for _, conn := range r.Connections {
		if conn != nil {
			conns = append(conns, conn)
		}
	}
	r.Connections = make(map[string]*websocket.Conn)
	r.Players = make(map[string]*protobufs.PlayerData)
	r.Status = RoomStatusEnded
	r.mu.Unlock()

	for _, conn := range conns {
		_ = conn.Close()
	}
}
