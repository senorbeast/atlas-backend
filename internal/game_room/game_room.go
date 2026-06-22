package game_room

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	mathrand "math/rand"
	"sort"
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

type ChatMessage struct {
	MessageID  string `json:"messageId"`
	SenderID   string `json:"senderId"`
	SenderName string `json:"senderName"`
	Content    string `json:"content"`
	CreatedAt  string `json:"createdAt"`
}

type ChatPage struct {
	Messages   []ChatMessage `json:"messages"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

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

type RoomManager struct {
	mu    sync.Mutex
	rooms map[string]*GameRoom
}

func NewRoomManager() *RoomManager {
	return &RoomManager{rooms: make(map[string]*GameRoom)}
}

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

func (m *RoomManager) GetRoom(roomID string) (*GameRoom, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	room, ok := m.rooms[roomID]
	return room, ok
}

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

func (m *RoomManager) AddChatMessage(roomID string, playerID string, content string, now time.Time) (*protobufs.ChatMessagePayload, *games.GameError) {
	room, ok := m.GetRoom(roomID)
	if !ok {
		return nil, &games.GameError{Code: "room_not_found", Message: "Room not found"}
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, &games.GameError{Code: "empty_chat", Message: "Enter a message first"}
	}
	if len([]rune(trimmed)) > 280 {
		return nil, &games.GameError{Code: "chat_too_long", Message: "Messages must be 280 characters or fewer"}
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	player, ok := room.Players[playerID]
	if !ok {
		return nil, &games.GameError{Code: "unknown_player", Message: "Player is not in the room"}
	}

	id := generateID("msg")
	createdAt := now.UTC().Format(time.RFC3339Nano)
	message := ChatMessage{
		MessageID:  id,
		SenderID:   playerID,
		SenderName: player.Name,
		Content:    trimmed,
		CreatedAt:  createdAt,
	}
	room.ChatHistory = append(room.ChatHistory, message)
	room.LastActivity = now

	return &protobufs.ChatMessagePayload{
		MessageId:  id,
		SenderId:   playerID,
		SenderName: player.Name,
		Content:    trimmed,
		CreatedAt:  createdAt,
	}, nil
}

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
		player.Score++
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

func (m *RoomManager) GameState(roomID string) (*protobufs.GameStatePayload, *games.GameError) {
	room, ok := m.GetRoom(roomID)
	if !ok {
		return nil, &games.GameError{Code: "room_not_found", Message: "Room not found"}
	}
	room.mu.Lock()
	defer room.mu.Unlock()
	return room.gameStateLocked(), nil
}

func (m *RoomManager) ChatPage(roomID string, limit int, before string) (ChatPage, error) {
	room, ok := m.GetRoom(roomID)
	if !ok {
		return ChatPage{}, errors.New("room not found")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	end := len(room.ChatHistory)
	if before != "" {
		for index, message := range room.ChatHistory {
			if message.MessageID == before {
				end = index
				break
			}
		}
	}

	start := end - limit
	if start < 0 {
		start = 0
	}

	messages := make([]ChatMessage, 0, end-start)
	for i := end - 1; i >= start; i-- {
		messages = append(messages, room.ChatHistory[i])
	}

	nextCursor := ""
	if start > 0 && len(messages) > 0 {
		nextCursor = room.ChatHistory[start].MessageID
	}

	return ChatPage{Messages: messages, NextCursor: nextCursor}, nil
}

func (r *GameRoom) ConnectionsSnapshot() []*websocket.Conn {
	r.mu.Lock()
	defer r.mu.Unlock()
	conns := make([]*websocket.Conn, 0, len(r.Connections))
	for _, conn := range r.Connections {
		if conn != nil {
			conns = append(conns, conn)
		}
	}
	return conns
}

func (r *GameRoom) Snapshot() *protobufs.RoomSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

func (r *GameRoom) snapshotLocked() *protobufs.RoomSnapshot {
	r.ensureCurrentTurnLocked()

	players := make([]*protobufs.PlayerData, 0, len(r.Players))
	for _, player := range r.Players {
		copyPlayer := *player
		players = append(players, &copyPlayer)
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].JoinedAt < players[j].JoinedAt
	})
	currentTurnPlayerID, currentTurnPlayerName := r.currentTurnPlayerLocked()

	return &protobufs.RoomSnapshot{
		RoomId:                r.RoomID,
		GameKind:              r.GameKind,
		Status:                r.Status,
		MaxPlayers:            int32(r.MaxPlayers),
		IsStarted:             r.Status == RoomStatusPlaying,
		CreatedAt:             r.CreatedAt.UTC().Format(time.RFC3339Nano),
		StartedAt:             r.StartedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:             r.ExpiresAt.UTC().Format(time.RFC3339Nano),
		Players:               players,
		TurnMode:              r.TurnMode,
		CurrentTurnPlayerId:   currentTurnPlayerID,
		CurrentTurnPlayerName: currentTurnPlayerName,
		TurnIndex:             int32(r.TurnIndex),
	}
}

func (r *GameRoom) gameStateLocked() *protobufs.GameStatePayload {
	r.ensureCurrentTurnLocked()

	state := r.Game.Snapshot()
	state.CurrentTurnPlayerId = ""
	state.CurrentTurnPlayerName = ""
	state.TurnIndex = int32(r.TurnIndex)
	if r.TurnMode != TurnModeStrict {
		return state
	}

	currentTurnPlayerID, currentTurnPlayerName := r.currentTurnPlayerLocked()
	state.CurrentTurnPlayerId = currentTurnPlayerID
	state.CurrentTurnPlayerName = currentTurnPlayerName
	state.TurnIndex = int32(r.TurnIndex)
	return state
}

func (r *GameRoom) currentTurnPlayerLocked() (string, string) {
	if r.TurnMode != TurnModeStrict {
		return "", ""
	}
	r.ensureCurrentTurnLocked()
	players := r.connectedPlayersLocked()
	if len(players) == 0 {
		r.TurnIndex = 0
		return "", ""
	}
	r.clampTurnIndexLocked()
	current := players[r.TurnIndex]
	return current.GetPlayerId(), current.GetName()
}

func (r *GameRoom) connectedPlayersLocked() []*protobufs.PlayerData {
	players := make([]*protobufs.PlayerData, 0, len(r.Players))
	seen := make(map[string]struct{}, len(r.TurnOrder))
	for _, playerID := range r.TurnOrder {
		player, ok := r.Players[playerID]
		if ok && player.GetConnected() {
			players = append(players, player)
		}
		seen[playerID] = struct{}{}
	}
	for playerID := range r.Connections {
		if _, exists := seen[playerID]; exists {
			continue
		}
		player, ok := r.Players[playerID]
		if ok && player.GetConnected() {
			players = append(players, player)
		}
	}
	return players
}

func (r *GameRoom) currentTurnPlayerIDLocked() string {
	if r.TurnMode != TurnModeStrict {
		return ""
	}
	r.ensureCurrentTurnLocked()
	players := r.connectedPlayersLocked()
	if len(players) == 0 {
		return ""
	}
	if r.TurnIndex < 0 || r.TurnIndex >= len(players) {
		r.TurnIndex = 0
	}
	return players[r.TurnIndex].GetPlayerId()
}

func (r *GameRoom) advanceTurnLocked() {
	r.ensureCurrentTurnLocked()
	players := r.connectedPlayersLocked()
	if len(players) == 0 {
		r.TurnIndex = 0
		return
	}
	r.TurnIndex = (r.TurnIndex + 1) % len(players)
}

func (r *GameRoom) randomizeCurrentTurnLocked(now time.Time) {
	players := r.connectedPlayersLocked()
	if len(players) == 0 {
		r.TurnIndex = 0
		return
	}
	if len(players) == 1 {
		r.TurnIndex = 0
		return
	}
	r.TurnIndex = mathrand.New(mathrand.NewSource(now.UnixNano())).Intn(len(players))
}

func (r *GameRoom) ensureCurrentTurnLocked() {
	if r.TurnMode != TurnModeStrict {
		return
	}
	players := r.connectedPlayersLocked()
	if len(players) == 0 {
		r.TurnIndex = 0
		return
	}
	if len(r.TurnOrder) == 0 {
		r.TurnOrder = make([]string, 0, len(players))
		for _, player := range players {
			r.TurnOrder = append(r.TurnOrder, player.GetPlayerId())
		}
	}
	r.clampTurnIndexLocked()
}

func (r *GameRoom) clampTurnIndexLocked() {
	players := r.connectedPlayersLocked()
	if len(players) == 0 {
		r.TurnIndex = 0
		return
	}
	if r.TurnIndex < 0 || r.TurnIndex >= len(players) {
		r.TurnIndex = r.TurnIndex % len(players)
		if r.TurnIndex < 0 {
			r.TurnIndex = 0
		}
	}
}

func (r *GameRoom) setTurnIndexForPlayerLocked(playerID string) {
	players := r.connectedPlayersLocked()
	for index, player := range players {
		if player.GetPlayerId() == playerID {
			r.TurnIndex = index
			return
		}
	}
	r.clampTurnIndexLocked()
}

func (r *GameRoom) turnOrderIndexLocked(playerID string) int {
	for index, item := range r.TurnOrder {
		if item == playerID {
			return index
		}
	}
	return -1
}

func (r *GameRoom) removeFromTurnOrderLocked(playerID string) {
	next := r.TurnOrder[:0]
	for _, item := range r.TurnOrder {
		if item != playerID {
			next = append(next, item)
		}
	}
	r.TurnOrder = next
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

func normalizeDisplayName(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func normalizeTurnMode(value string) (string, error) {
	mode := strings.TrimSpace(value)
	if mode == "" {
		return TurnModeStrict, nil
	}
	switch mode {
	case TurnModeStrict, TurnModeFreeForAll:
		return mode, nil
	default:
		return "", errors.New("invalid turn mode")
	}
}

func generateID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(bytes))
}
