package web_socket

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
	"google.golang.org/protobuf/proto"
)

func TestWebSocketJoinChatAndCitySubmission(t *testing.T) {
	setupWebSocketCityFixture(t)
	manager := game_room.NewRoomManager()
	if _, err := manager.CreateRoom("room-1", game_room.DefaultGameKind, "", time.Now()); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	server := httptest.NewServer(HandleWebSocketConnections(manager))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/rooms/room-1/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	writeClientMessage(t, conn, &protobufs.ClientToServerMessage{
		MessageType: protobufs.ClientToServerMessageType_JOIN_ROOM,
		Payload: &protobufs.ClientToServerMessage_JoinRoomPayload{
			JoinRoomPayload: &protobufs.JoinRoomPayload{
				DisplayName: "Ada",
				GameKind:    game_room.DefaultGameKind,
			},
		},
	})

	ackMessage := readUntil(t, conn, protobufs.ServerToClientMessageType_SEND_ON_CONNECT_ACK)
	ack := ackMessage.GetOnConnectAckPayload()
	if ack.GetPlayerId() == "" {
		t.Fatal("ACK should include player id")
	}
	if got := ack.GetRoom().GetPlayers()[0].GetName(); got != "Ada" {
		t.Fatalf("ACK player name = %q, want Ada", got)
	}
	adaID := ack.GetPlayerId()

	graceConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial(Grace) error = %v", err)
	}
	defer graceConn.Close()
	writeClientMessage(t, graceConn, &protobufs.ClientToServerMessage{
		MessageType: protobufs.ClientToServerMessageType_JOIN_ROOM,
		Payload: &protobufs.ClientToServerMessage_JoinRoomPayload{
			JoinRoomPayload: &protobufs.JoinRoomPayload{
				DisplayName: "Grace",
				GameKind:    game_room.DefaultGameKind,
			},
		},
	})
	graceAckMessage := readUntil(t, graceConn, protobufs.ServerToClientMessageType_SEND_ON_CONNECT_ACK)
	graceID := graceAckMessage.GetOnConnectAckPayload().GetPlayerId()
	if graceID == "" {
		t.Fatal("Grace ACK should include player id")
	}
	activeID := graceAckMessage.GetOnConnectAckPayload().GetGameState().GetCurrentTurnPlayerId()
	if activeID != adaID && activeID != graceID {
		t.Fatalf("initial current turn = %q, want Ada %q or Grace %q", activeID, adaID, graceID)
	}
	activeConn := conn
	inactiveConn := graceConn
	inactiveID := graceID
	if activeID == graceID {
		activeConn = graceConn
		inactiveConn = conn
		inactiveID = adaID
	}
	readUntil(t, conn, protobufs.ServerToClientMessageType_BROADCAST_ROOM_UPDATE)
	readUntil(t, conn, protobufs.ServerToClientMessageType_RESPOND_GAME_STATE)
	readUntil(t, graceConn, protobufs.ServerToClientMessageType_BROADCAST_ROOM_UPDATE)
	readUntil(t, graceConn, protobufs.ServerToClientMessageType_RESPOND_GAME_STATE)

	writeClientMessage(t, conn, &protobufs.ClientToServerMessage{
		MessageType: protobufs.ClientToServerMessageType_SEND_CHAT_MESSAGE,
		Payload: &protobufs.ClientToServerMessage_ChatMessagePayload{
			ChatMessagePayload: &protobufs.ChatMessagePayload{Content: "hello"},
		},
	})

	chatMessage := readUntil(t, conn, protobufs.ServerToClientMessageType_BROADCAST_CHAT_MESSAGE)
	chat := chatMessage.GetChatMessagePayload()
	if chat.GetSenderName() != "Ada" || chat.GetMessageId() == "" || chat.GetContent() != "hello" {
		t.Fatalf("chat payload = %#v, want sender name, id, content", chat)
	}
	readUntil(t, graceConn, protobufs.ServerToClientMessageType_BROADCAST_CHAT_MESSAGE)

	writeClientMessage(t, inactiveConn, &protobufs.ClientToServerMessage{
		MessageType: protobufs.ClientToServerMessageType_SEND_GAME_UPDATE,
		Payload: &protobufs.ClientToServerMessage_GameUpdatePayload{
			GameUpdatePayload: &protobufs.GameUpdatePayload{
				Type:     "submit_city",
				GameKind: game_room.DefaultGameKind,
				CityName: "Sydney",
			},
		},
	})
	errorMessage := readUntil(t, inactiveConn, protobufs.ServerToClientMessageType_SEND_ERROR)
	if got := errorMessage.GetErrorPayload().GetCode(); got != "not_your_turn" {
		t.Fatalf("inactive submit error = %q, want not_your_turn", got)
	}

	writeClientMessage(t, activeConn, &protobufs.ClientToServerMessage{
		MessageType: protobufs.ClientToServerMessageType_SEND_GAME_UPDATE,
		Payload: &protobufs.ClientToServerMessage_GameUpdatePayload{
			GameUpdatePayload: &protobufs.GameUpdatePayload{
				Type:     "submit_city",
				GameKind: game_room.DefaultGameKind,
				CityName: "Sydney",
			},
		},
	})

	sequence := readSequence(t, conn, []protobufs.ServerToClientMessageType{
		protobufs.ServerToClientMessageType_BROADCAST_GAME_UPDATE,
		protobufs.ServerToClientMessageType_RESPOND_GAME_STATE,
		protobufs.ServerToClientMessageType_BROADCAST_ROOM_UPDATE,
	})
	updateMessage := sequence[0]
	update := updateMessage.GetGameUpdatePayload()
	expectedSubmitter := "Ada"
	if activeID == graceID {
		expectedSubmitter = "Grace"
	}
	if update.GetAcceptedCity().GetName() != "Sydney" ||
		update.GetAcceptedCity().GetCityHash() == "" ||
		update.GetAcceptedCity().GetSubmittedByName() != expectedSubmitter {
		t.Fatalf("game update = %#v, want accepted Sydney by %s", update, expectedSubmitter)
	}

	stateMessage := sequence[1]
	if got := stateMessage.GetGameStatePayload().GetCurrentTurnPlayerId(); got != inactiveID {
		t.Fatalf("next current turn = %q, want inactive player %q", got, inactiveID)
	}
	roomUpdateMessage := sequence[2]
	players := roomUpdateMessage.GetRoomUpdatePayload().GetRoom().GetPlayers()
	if len(players) != 2 || scoreForPlayer(players, activeID) != 1 {
		t.Fatalf("room players after score = %#v, want active player score 1", players)
	}

	lateConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial(late) error = %v", err)
	}
	defer lateConn.Close()
	writeClientMessage(t, lateConn, &protobufs.ClientToServerMessage{
		MessageType: protobufs.ClientToServerMessageType_JOIN_ROOM,
		Payload: &protobufs.ClientToServerMessage_JoinRoomPayload{
			JoinRoomPayload: &protobufs.JoinRoomPayload{
				DisplayName: "Linus",
				GameKind:    game_room.DefaultGameKind,
			},
		},
	})
	lateAck := readUntil(t, lateConn, protobufs.ServerToClientMessageType_SEND_ON_CONNECT_ACK).GetOnConnectAckPayload()
	if len(lateAck.GetRoom().GetPlayers()) != 3 {
		t.Fatalf("late ACK players = %#v, want three players", lateAck.GetRoom().GetPlayers())
	}
	if got := len(lateAck.GetGameState().GetAcceptedCities()); got != 1 {
		t.Fatalf("late ACK accepted cities = %d, want 1", got)
	}
	if got := scoreForPlayer(lateAck.GetRoom().GetPlayers(), activeID); got != 1 {
		t.Fatalf("late ACK active score = %d, want 1", got)
	}
}

func writeClientMessage(t *testing.T, conn *websocket.Conn, message *protobufs.ClientToServerMessage) {
	t.Helper()
	data, err := proto.Marshal(message)
	if err != nil {
		t.Fatalf("marshal client message: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("write client message: %v", err)
	}
}

func readUntil(t *testing.T, conn *websocket.Conn, messageType protobufs.ServerToClientMessageType) *protobufs.ServerToClientMessage {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read message: %v", err)
		}
		var serverMessage protobufs.ServerToClientMessage
		if err := proto.Unmarshal(payload, &serverMessage); err != nil {
			t.Fatalf("unmarshal server message: %v", err)
		}
		if serverMessage.GetMessageType() == messageType {
			return &serverMessage
		}
	}
	t.Fatalf("timed out waiting for %v", messageType)
	return nil
}

func readSequence(t *testing.T, conn *websocket.Conn, messageTypes []protobufs.ServerToClientMessageType) []*protobufs.ServerToClientMessage {
	t.Helper()
	messages := make([]*protobufs.ServerToClientMessage, 0, len(messageTypes))
	for _, messageType := range messageTypes {
		message := readNext(t, conn)
		if message.GetMessageType() != messageType {
			t.Fatalf("next message type = %v, want %v", message.GetMessageType(), messageType)
		}
		messages = append(messages, message)
	}
	return messages
}

func readNext(t *testing.T, conn *websocket.Conn) *protobufs.ServerToClientMessage {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var serverMessage protobufs.ServerToClientMessage
	if err := proto.Unmarshal(payload, &serverMessage); err != nil {
		t.Fatalf("unmarshal server message: %v", err)
	}
	return &serverMessage
}

func setupWebSocketCityFixture(t *testing.T) {
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
