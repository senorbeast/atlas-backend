package game_room

import (
	mathrand "math/rand"
	"time"

	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

// TurnPolicy describes room-level turn ownership and advancement behaviour.
type TurnPolicy interface {
	CurrentPlayer(room *GameRoom) (string, string)
	Advance(room *GameRoom)
}

// StrictTurnPolicy advances through connected players in stable join order.
type StrictTurnPolicy struct{}

// CurrentPlayer returns the current player under strict turn semantics.
func (StrictTurnPolicy) CurrentPlayer(room *GameRoom) (string, string) {
	room.mu.Lock()
	defer room.mu.Unlock()
	return room.currentTurnPlayerLocked()
}

// Advance moves the active turn to the next connected player.
func (StrictTurnPolicy) Advance(room *GameRoom) {
	room.mu.Lock()
	defer room.mu.Unlock()
	room.advanceTurnLocked()
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
