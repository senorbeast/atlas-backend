package game_room

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

func TestRoomManagerChatPageLatestFirstAndCursor(t *testing.T) {
	setupRoomCityFixture(t)
	manager := NewRoomManager()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	room, err := manager.CreateRoom("room-1", DefaultGameKind, "", now)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	room.mu.Lock()
	room.Players["p1"] = &protobufs.PlayerData{PlayerId: "p1", Name: "Ada", Connected: true}
	room.mu.Unlock()

	for i := 1; i <= 4; i++ {
		if _, gameErr := manager.AddChatMessage("room-1", "p1", fmt.Sprintf("message %d", i), now.Add(time.Duration(i)*time.Second)); gameErr != nil {
			t.Fatalf("AddChatMessage(%d) error = %v", i, gameErr)
		}
	}

	firstPage, err := manager.ChatPage("room-1", 2, "")
	if err != nil {
		t.Fatalf("ChatPage() error = %v", err)
	}
	if got := []string{firstPage.Messages[0].Content, firstPage.Messages[1].Content}; got[0] != "message 4" || got[1] != "message 3" {
		t.Fatalf("first page = %#v, want latest first messages 4,3", got)
	}
	if firstPage.NextCursor == "" {
		t.Fatal("first page should include next cursor")
	}
	encoded, err := json.Marshal(firstPage.Messages[0])
	if err != nil {
		t.Fatalf("marshal chat message: %v", err)
	}
	if string(encoded) != `{"messageId":"`+firstPage.Messages[0].MessageID+`","senderId":"p1","senderName":"Ada","content":"message 4","createdAt":"`+firstPage.Messages[0].CreatedAt+`"}` {
		t.Fatalf("chat message json = %s, should not include roomId", encoded)
	}

	secondPage, err := manager.ChatPage("room-1", 2, firstPage.NextCursor)
	if err != nil {
		t.Fatalf("ChatPage(before) error = %v", err)
	}
	if got := []string{secondPage.Messages[0].Content, secondPage.Messages[1].Content}; got[0] != "message 2" || got[1] != "message 1" {
		t.Fatalf("second page = %#v, want messages 2,1", got)
	}
	if secondPage.NextCursor != "" {
		t.Fatalf("second page next cursor = %q, want empty", secondPage.NextCursor)
	}
}

func TestRoomManagerCleanupExpiredAndEmptyRooms(t *testing.T) {
	setupRoomCityFixture(t)
	manager := NewRoomManager()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	room, err := manager.CreateRoom("room-1", DefaultGameKind, "", now)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	emptySince := now.Add(-4 * time.Minute)
	room.EmptySince = &emptySince
	removed := manager.CleanupExpired(now)
	if len(removed) != 1 || removed[0] != "room-1" {
		t.Fatalf("removed empty rooms = %#v, want room-1", removed)
	}

	room, err = manager.CreateRoom("room-2", DefaultGameKind, "", now)
	if err != nil {
		t.Fatalf("CreateRoom(room-2) error = %v", err)
	}
	room.ExpiresAt = now.Add(-time.Second)
	removed = manager.CleanupExpired(now)
	if len(removed) != 1 || removed[0] != "room-2" {
		t.Fatalf("removed expired rooms = %#v, want room-2", removed)
	}
}

func TestRoomManagerRejectsInvalidPlayerName(t *testing.T) {
	setupRoomCityFixture(t)
	manager := NewRoomManager()
	if _, err := manager.CreateRoom("room-1", DefaultGameKind, "", time.Now()); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	_, _, gameErr := manager.JoinRoom("room-1", nil, "   ", DefaultGameKind, "", time.Now())
	if gameErr == nil || gameErr.Code != "invalid_player_name" {
		t.Fatalf("JoinRoom invalid name error = %#v, want invalid_player_name", gameErr)
	}
}

func TestRoomManagerRejectsInvalidChatContent(t *testing.T) {
	setupRoomCityFixture(t)
	manager := NewRoomManager()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	if _, err := manager.CreateRoom("room-1", DefaultGameKind, "", now); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	_, playerID, gameErr := manager.JoinRoom("room-1", nil, "Ada", DefaultGameKind, "", now)
	if gameErr != nil {
		t.Fatalf("JoinRoom() error = %v", gameErr)
	}

	_, gameErr = manager.AddChatMessage("room-1", playerID, "   ", now)
	if gameErr == nil || gameErr.Code != "empty_chat" {
		t.Fatalf("empty chat error = %#v, want empty_chat", gameErr)
	}

	_, gameErr = manager.AddChatMessage("room-1", playerID, strings.Repeat("x", 281), now)
	if gameErr == nil || gameErr.Code != "chat_too_long" {
		t.Fatalf("long chat error = %#v, want chat_too_long", gameErr)
	}
}

func TestRoomManagerStrictTurnsScoresAndAdvances(t *testing.T) {
	setupRoomCityFixture(t)
	manager := NewRoomManager()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	if _, err := manager.CreateRoom("room-1", DefaultGameKind, "", now); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	ackAda, adaID, gameErr := manager.JoinRoom("room-1", nil, "Ada", DefaultGameKind, "", now)
	if gameErr != nil {
		t.Fatalf("JoinRoom(Ada) error = %v", gameErr)
	}
	if got := ackAda.GetGameState().GetCurrentTurnPlayerId(); got != adaID {
		t.Fatalf("first player game state turn = %q, want Ada %q", got, adaID)
	}
	if got := ackAda.GetRoom().GetCurrentTurnPlayerId(); got != adaID {
		t.Fatalf("first player room snapshot turn = %q, want Ada %q", got, adaID)
	}
	ackGrace, graceID, gameErr := manager.JoinRoom("room-1", nil, "Grace", DefaultGameKind, "", now.Add(time.Second))
	if gameErr != nil {
		t.Fatalf("JoinRoom(Grace) error = %v", gameErr)
	}
	activeID := ackGrace.GetGameState().GetCurrentTurnPlayerId()
	if activeID != adaID && activeID != graceID {
		t.Fatalf("initial turn = %q, want Ada %q or Grace %q", activeID, adaID, graceID)
	}
	if got := ackGrace.GetRoom().GetCurrentTurnPlayerId(); got != activeID {
		t.Fatalf("initial room snapshot turn = %q, want active %q", got, activeID)
	}
	inactiveID := adaID
	if activeID == adaID {
		inactiveID = graceID
	}

	_, _, gameErr = manager.ApplyGameUpdate("room-1", inactiveID, &protobufs.GameUpdatePayload{
		Type:     "submit_city",
		GameKind: DefaultGameKind,
		CityName: "Sydney",
	}, now.Add(2*time.Second))
	if gameErr == nil || gameErr.Code != "not_your_turn" {
		t.Fatalf("inactive move error = %#v, want not_your_turn", gameErr)
	}

	result, roomUpdate, gameErr := manager.ApplyGameUpdate("room-1", activeID, &protobufs.GameUpdatePayload{
		Type:     "submit_city",
		GameKind: DefaultGameKind,
		CityName: "Sydney",
	}, now.Add(3*time.Second))
	if gameErr != nil {
		t.Fatalf("ApplyGameUpdate(active player) error = %v", gameErr)
	}
	if got := scoreForPlayer(roomUpdate.GetRoom().GetPlayers(), activeID); got != 1 {
		t.Fatalf("active player score = %d, want 1", got)
	}
	if got := result.State.GetCurrentTurnPlayerId(); got != inactiveID {
		t.Fatalf("next turn = %q, want inactive player %q", got, inactiveID)
	}
	if got := roomUpdate.GetRoom().GetCurrentTurnPlayerId(); got != inactiveID {
		t.Fatalf("room update turn = %q, want inactive player %q", got, inactiveID)
	}
}

func TestRoomManagerLateJoinReceivesCurrentSnapshot(t *testing.T) {
	setupRoomCityFixture(t)
	manager := NewRoomManager()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	if _, err := manager.CreateRoom("room-1", DefaultGameKind, "", now); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	_, adaID, gameErr := manager.JoinRoom("room-1", nil, "Ada", DefaultGameKind, "", now)
	if gameErr != nil {
		t.Fatalf("JoinRoom(Ada) error = %v", gameErr)
	}
	_, graceID, gameErr := manager.JoinRoom("room-1", nil, "Grace", DefaultGameKind, "", now.Add(time.Second))
	if gameErr != nil {
		t.Fatalf("JoinRoom(Grace) error = %v", gameErr)
	}
	state, gameErr := manager.GameState("room-1")
	if gameErr != nil {
		t.Fatalf("GameState() error = %v", gameErr)
	}
	activeID := state.GetCurrentTurnPlayerId()
	if activeID == "" {
		t.Fatal("active player should be assigned before first move")
	}

	_, _, gameErr = manager.ApplyGameUpdate("room-1", activeID, &protobufs.GameUpdatePayload{
		Type:     "submit_city",
		GameKind: DefaultGameKind,
		CityName: "Sydney",
	}, now.Add(2*time.Second))
	if gameErr != nil {
		t.Fatalf("ApplyGameUpdate(active) error = %v", gameErr)
	}

	ack, lateID, gameErr := manager.JoinRoom("room-1", nil, "Linus", DefaultGameKind, "", now.Add(3*time.Second))
	if gameErr != nil {
		t.Fatalf("JoinRoom(late) error = %v", gameErr)
	}
	if lateID == "" || len(ack.GetRoom().GetPlayers()) != 3 {
		t.Fatalf("late ACK players = %#v, lateID = %q; want three players and id", ack.GetRoom().GetPlayers(), lateID)
	}
	if got := len(ack.GetGameState().GetAcceptedCities()); got != 1 {
		t.Fatalf("late ACK accepted cities = %d, want 1", got)
	}
	if got := scoreForPlayer(ack.GetRoom().GetPlayers(), activeID); got != 1 {
		t.Fatalf("late ACK active score = %d, want 1", got)
	}
	nextTurn := ack.GetGameState().GetCurrentTurnPlayerId()
	if nextTurn == "" || nextTurn == activeID {
		t.Fatalf("late ACK next turn = %q, want another connected player", nextTurn)
	}
	if nextTurn != adaID && nextTurn != graceID {
		t.Fatalf("late ACK next turn = %q, want Ada or Grace", nextTurn)
	}
}

func TestRoomManagerFreeForAllAllowsAnyPlayer(t *testing.T) {
	setupRoomCityFixture(t)
	manager := NewRoomManager()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	if _, err := manager.CreateRoom("room-1", DefaultGameKind, TurnModeFreeForAll, now); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	_, _, gameErr := manager.JoinRoom("room-1", nil, "Ada", DefaultGameKind, TurnModeFreeForAll, now)
	if gameErr != nil {
		t.Fatalf("JoinRoom(Ada) error = %v", gameErr)
	}
	_, graceID, gameErr := manager.JoinRoom("room-1", nil, "Grace", DefaultGameKind, TurnModeFreeForAll, now.Add(time.Second))
	if gameErr != nil {
		t.Fatalf("JoinRoom(Grace) error = %v", gameErr)
	}

	result, _, gameErr := manager.ApplyGameUpdate("room-1", graceID, &protobufs.GameUpdatePayload{
		Type:     "submit_city",
		GameKind: DefaultGameKind,
		CityName: "Sydney",
	}, now.Add(2*time.Second))
	if gameErr != nil {
		t.Fatalf("free-for-all ApplyGameUpdate() error = %v", gameErr)
	}
	if got := result.State.GetCurrentTurnPlayerId(); got != "" {
		t.Fatalf("free-for-all current turn = %q, want empty", got)
	}
}

func TestRoomManagerRejectsExpiredJoin(t *testing.T) {
	setupRoomCityFixture(t)
	manager := NewRoomManager()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	room, err := manager.CreateRoom("room-1", DefaultGameKind, "", now)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	room.ExpiresAt = now.Add(-time.Second)

	_, _, gameErr := manager.JoinRoom("room-1", nil, "Ada", DefaultGameKind, "", now)
	if gameErr == nil || gameErr.Code != "room_closed" {
		t.Fatalf("expired join error = %#v, want room_closed", gameErr)
	}
}

func TestRoomManagerDisconnectReassignsActiveTurn(t *testing.T) {
	setupRoomCityFixture(t)
	manager := NewRoomManager()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	if _, err := manager.CreateRoom("room-1", DefaultGameKind, "", now); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	_, adaID, gameErr := manager.JoinRoom("room-1", nil, "Ada", DefaultGameKind, "", now)
	if gameErr != nil {
		t.Fatalf("JoinRoom(Ada) error = %v", gameErr)
	}
	_, graceID, gameErr := manager.JoinRoom("room-1", nil, "Grace", DefaultGameKind, "", now.Add(time.Second))
	if gameErr != nil {
		t.Fatalf("JoinRoom(Grace) error = %v", gameErr)
	}

	update := manager.Disconnect("room-1", adaID, now.Add(2*time.Second))
	if update == nil || len(update.GetRoom().GetPlayers()) != 1 {
		t.Fatalf("disconnect update = %#v, want one remaining player", update)
	}
	state, gameErr := manager.GameState("room-1")
	if gameErr != nil {
		t.Fatalf("GameState() error = %v", gameErr)
	}
	if got := state.GetCurrentTurnPlayerId(); got != graceID {
		t.Fatalf("turn after active disconnect = %q, want Grace %q", got, graceID)
	}
}

func setupRoomCityFixture(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cities.json")
	data := `[
		{"id": 1, "name": "Sydney", "country": "AU", "lat": "-33.8678500", "lon": "151.2073200"},
		{"id": 2, "name": "York", "country": "GB", "lat": "53.9599650", "lon": "-1.0872980"}
	]`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write city fixture: %v", err)
	}
	t.Setenv("ATLAS_CITIES_PATH", path)
}

func scoreForPlayer(players []*protobufs.PlayerData, playerID string) int32 {
	for _, player := range players {
		if player.GetPlayerId() == playerID {
			return player.GetScore()
		}
	}
	return -1
}
