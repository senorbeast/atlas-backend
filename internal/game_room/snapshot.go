package game_room

import (
	"sort"
	"time"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

// ConnectionsSnapshot returns currently connected sockets for safe fan-out.
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

// Snapshot returns a copy of current room/player/turn metadata.
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
