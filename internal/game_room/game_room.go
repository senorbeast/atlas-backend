package game_room

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

// AddPlayer adds a player to the game room
func (gr *GameRoom) AddPlayer(player *protobufs.PlayerData, conn *websocket.Conn) {
	gr.PlayersMux.Lock()
	defer gr.PlayersMux.Unlock()

	if len(gr.Players) == 0 {
		gr.HostPlayerID = player.PlayerId
	}

	gr.Players[player.PlayerId] = &PlayerConnection{
		Player: player,
		Conn:   conn,
	}
	gr.LastActivity = time.Now()
}

// RemovePlayer removes a player from the game room
func (gr *GameRoom) RemovePlayer(playerID string) {
	gr.PlayersMux.Lock()
	defer gr.PlayersMux.Unlock()

	delete(gr.Players, playerID)
	gr.LastActivity = time.Now()
}

// GetPlayer retrieves a player from the game room
func (gr *GameRoom) GetPlayer(playerID string) (*PlayerConnection, bool) {
	gr.PlayersMux.Lock()
	defer gr.PlayersMux.Unlock()

	player, ok := gr.Players[playerID]
	return player, ok
}

// Broadcast sends a message to all players in the game room
func (gr *GameRoom) Broadcast(message []byte) {
	gr.PlayersMux.Lock()
	defer gr.PlayersMux.Unlock()

	for _, player := range gr.Players {
		if player.Conn != nil {
			player.Conn.WriteMessage(websocket.BinaryMessage, message)
		}
	}
}

// BroadcastExcept sends a message to all players in the game room except one
func (gr *GameRoom) BroadcastExcept(message []byte, exceptPlayerID string) {
	gr.PlayersMux.Lock()
	defer gr.PlayersMux.Unlock()

	for playerID, player := range gr.Players {
		if playerID != exceptPlayerID {
			if player.Conn != nil {
				player.Conn.WriteMessage(websocket.BinaryMessage, message)
			}
		}
	}
}

// SendToPlayer sends a message to a specific player in the game room
func (gr *GameRoom) SendToPlayer(playerID string, message []byte) {
	gr.PlayersMux.Lock()
	defer gr.PlayersMux.Unlock()

	if player, ok := gr.Players[playerID]; ok {
		if player.Conn != nil {
			player.Conn.WriteMessage(websocket.BinaryMessage, message)
		}
	}
}

func (gr *GameRoom) GetGameStateSyncPayload() *protobufs.GameStateSyncPayload {
	gr.PlayersMux.Lock()
	defer gr.PlayersMux.Unlock()

	players := make([]*protobufs.PlayerData, 0, len(gr.Players))
	for _, p := range gr.Players {
		players = append(players, p.Player)
	}

	return &protobufs.GameStateSyncPayload{
		RoomId:             gr.RoomID,
		Category:           gr.Category,
		IsStarted:          gr.IsStarted,
		IsOver:             gr.IsOver,
		TurnPlayerId:       gr.TurnPlayerID,
		CurrentLetter:      gr.CurrentLetter,
		LastWord:           gr.LastWord,
		Players:            players,
		TurnStartTimestamp: gr.TurnStartTime,
	}
}