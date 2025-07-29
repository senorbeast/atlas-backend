package main

import (
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
	"github.com/senorbeast/atlas-backend/internal/web_socket"
	"google.golang.org/protobuf/proto"
)

func TestGameFlow(t *testing.T) {
	roomID := "test-room"
	gr := game_room.NewGameRoom(roomID)
	http.DefaultServeMux = new(http.ServeMux)
	web_socket.HandleWebSocketConnections(gr)

	server := httptest.NewServer(http.DefaultServeMux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/" + roomID
	log.Printf("Test server WS URL: %s", wsURL)

	client1MsgChan := make(chan *protobufs.ServerClientMessage, 10)
	client2MsgChan := make(chan *protobufs.ServerClientMessage, 10)

	// Client 1 (Host)
	conn1, err := connectToTestWS(wsURL)
	if err != nil {
		t.Fatalf("Client 1 failed to connect: %v", err)
	}
	defer conn1.Close()
	go listenForMessages(conn1, client1MsgChan, t)

	// Client 1 joins
	sendJoinGame(conn1, "PlayerOne")

	// Verify Client 1 connection
	msg := <-client1MsgChan
	if msg.GetMessageType() != protobufs.ServerClientMessageType_ON_CONNECT_ACK {
		t.Errorf("Expected ON_CONNECT_ACK, got %v", msg.GetMessageType())
	}

	msg = <-client1MsgChan
	if msg.GetMessageType() != protobufs.ServerClientMessageType_GAME_STATE_SYNC {
		t.Errorf("Expected GAME_STATE_SYNC, got %v", msg.GetMessageType())
	}

	// Client 2
	conn2, err := connectToTestWS(wsURL)
	if err != nil {
		t.Fatalf("Client 2 failed to connect: %v", err)
	}
	defer conn2.Close()
	go listenForMessages(conn2, client2MsgChan, t)

	// Client 2 joins
	sendJoinGame(conn2, "PlayerTwo")

	// Verify Client 2 connection
	msg = <-client2MsgChan
	if msg.GetMessageType() != protobufs.ServerClientMessageType_ON_CONNECT_ACK {
		t.Errorf("Expected ON_CONNECT_ACK, got %v", msg.GetMessageType())
	}
	client2ID := msg.GetOnConnectAck().PlayerId

	msg = <-client2MsgChan
	if msg.GetMessageType() != protobufs.ServerClientMessageType_GAME_STATE_SYNC {
		t.Errorf("Expected GAME_STATE_SYNC, got %v", msg.GetMessageType())
	}

	// Verify Client 1 receives PLAYER_JOINED for Client 2
	msg = <-client1MsgChan
	if msg.GetMessageType() != protobufs.ServerClientMessageType_PLAYER_JOINED {
		t.Fatalf("Expected PLAYER_JOINED, got %v", msg.GetMessageType())
	}
	if msg.GetPlayerJoined().PlayerData.PlayerId != client2ID {
		t.Errorf("Wrong player joined ID")
	}

	// Test Ready State
	sendToggleReady(conn1, true)
	msg = <-client1MsgChan // self update
	if msg.GetMessageType() != protobufs.ServerClientMessageType_PLAYER_DATA_UPDATE {
		t.Errorf("Expected PLAYER_DATA_UPDATE, got %v", msg.GetMessageType())
	}
	msg = <-client2MsgChan // broadcast
	if msg.GetMessageType() != protobufs.ServerClientMessageType_PLAYER_DATA_UPDATE {
		t.Errorf("Expected PLAYER_DATA_UPDATE, got %v", msg.GetMessageType())
	}

	sendToggleReady(conn2, true)
	<-client1MsgChan // discard client 1 broadcast
	<-client2MsgChan // discard client 2 self update

	// Test Start Game
	sendStartGame(conn1, "Countries")
	msg = <-client1MsgChan
	if msg.GetMessageType() != protobufs.ServerClientMessageType_GAME_STARTED {
		t.Errorf("Expected GAME_STARTED, got %v", msg.GetMessageType())
	}
	msg = <-client2MsgChan
	if msg.GetMessageType() != protobufs.ServerClientMessageType_GAME_STARTED {
		t.Errorf("Expected GAME_STARTED, got %v", msg.GetMessageType())
	}

	// Test Player Left
	conn2.Close() // Client 2 leaves
	msg = <-client1MsgChan
	if msg.GetMessageType() != protobufs.ServerClientMessageType_PLAYER_LEFT {
		t.Errorf("Expected PLAYER_LEFT, got %v", msg.GetMessageType())
	}
	if msg.GetPlayerLeft().PlayerId != client2ID {
		t.Errorf("Wrong player left ID, expected %s, got %s", client2ID, msg.GetPlayerLeft().PlayerId)
	}
}

func connectToTestWS(url string) (*websocket.Conn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func listenForMessages(conn *websocket.Conn, msgChan chan<- *protobufs.ServerClientMessage, t *testing.T) {
	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var serverMessage protobufs.ServerClientMessage
		if err := proto.Unmarshal(p, &serverMessage); err != nil {
			t.Errorf("Error unmarshaling: %v", err)
			continue
		}
		msgChan <- &serverMessage
	}
}

func sendJoinGame(conn *websocket.Conn, name string) {
	msg := &protobufs.ClientServerMessage{
		MessageType: protobufs.ClientServerMessageType_PLAYER_JOIN_GAME,
		Payload: &protobufs.ClientServerMessage_PlayerJoinGame{
			PlayerJoinGame: &protobufs.PlayerJoinGamePayload{PlayerName: name},
		},
	}
	data, _ := proto.Marshal(msg)
	conn.WriteMessage(websocket.BinaryMessage, data)
}

func sendToggleReady(conn *websocket.Conn, isReady bool) {
	msg := &protobufs.ClientServerMessage{
		MessageType: protobufs.ClientServerMessageType_TOGGLE_READY_STATE,
		Payload: &protobufs.ClientServerMessage_ToggleReadyState{
			ToggleReadyState: &protobufs.ToggleReadyStatePayload{IsReady: isReady},
		},
	}
	data, _ := proto.Marshal(msg)
	conn.WriteMessage(websocket.BinaryMessage, data)
}

func sendStartGame(conn *websocket.Conn, category string) {
	msg := &protobufs.ClientServerMessage{
		MessageType: protobufs.ClientServerMessageType_START_GAME,
		Payload: &protobufs.ClientServerMessage_StartGame{
			StartGame: &protobufs.StartGamePayload{Category: category},
		},
	}
	data, _ := proto.Marshal(msg)
	conn.WriteMessage(websocket.BinaryMessage, data)
}
