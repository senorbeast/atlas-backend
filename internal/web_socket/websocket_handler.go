package web_socket

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/games"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
	"google.golang.org/protobuf/proto"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var connectionWriteLocks sync.Map

func HandleWebSocketConnections(manager *game_room.RoomManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomID, ok := roomIDFromWebSocketPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("Error upgrading connection:", err)
			return
		}

		playerID, joined := handleJoin(manager, roomID, conn)
		if !joined {
			connectionWriteLocks.Delete(conn)
			_ = conn.Close()
			return
		}

		defer func() {
			defer connectionWriteLocks.Delete(conn)
			update := manager.Disconnect(roomID, playerID, time.Now())
			if update != nil {
				broadcast(manager, roomID, &protobufs.ServerToClientMessage{
					MessageType: protobufs.ServerToClientMessageType_BROADCAST_ROOM_UPDATE,
					Payload: &protobufs.ServerToClientMessage_RoomUpdatePayload{
						RoomUpdatePayload: update,
					},
				})
			}
		}()

		HandleAllMessages(manager, roomID, playerID, conn)
	}
}

func handleJoin(manager *game_room.RoomManager, roomID string, conn *websocket.Conn) (string, bool) {
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	messageType, payload, err := conn.ReadMessage()
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		sendError(conn, "join_timeout", "Join message was not received")
		return "", false
	}
	if messageType != websocket.BinaryMessage {
		sendError(conn, "invalid_payload", "Join message must be protobuf binary")
		return "", false
	}

	var clientMessage protobufs.ClientToServerMessage
	if err := proto.Unmarshal(payload, &clientMessage); err != nil {
		sendError(conn, "invalid_payload", "Unable to decode join message")
		return "", false
	}
	if clientMessage.GetMessageType() != protobufs.ClientToServerMessageType_JOIN_ROOM {
		sendError(conn, "join_required", "Join room before sending other messages")
		return "", false
	}

	joinPayload := clientMessage.GetJoinRoomPayload()
	if joinPayload == nil {
		sendError(conn, "invalid_join", "Join payload is required")
		return "", false
	}

	ack, playerID, gameErr := manager.JoinRoom(roomID, conn, joinPayload.GetDisplayName(), joinPayload.GetGameKind(), joinPayload.GetTurnMode(), time.Now())
	if gameErr != nil {
		sendGameError(conn, gameErr)
		return "", false
	}

	send(conn, &protobufs.ServerToClientMessage{
		MessageType: protobufs.ServerToClientMessageType_SEND_ON_CONNECT_ACK,
		Payload: &protobufs.ServerToClientMessage_OnConnectAckPayload{
			OnConnectAckPayload: ack,
		},
	})

	broadcast(manager, roomID, &protobufs.ServerToClientMessage{
		MessageType: protobufs.ServerToClientMessageType_BROADCAST_ROOM_UPDATE,
		Payload: &protobufs.ServerToClientMessage_RoomUpdatePayload{
			RoomUpdatePayload: &protobufs.RoomUpdatePayload{Room: ack.GetRoom()},
		},
	})
	if ack.GetGameState() != nil {
		broadcast(manager, roomID, &protobufs.ServerToClientMessage{
			MessageType: protobufs.ServerToClientMessageType_RESPOND_GAME_STATE,
			Payload: &protobufs.ServerToClientMessage_GameStatePayload{
				GameStatePayload: ack.GetGameState(),
			},
		})
	}

	fmt.Println("Player:", playerID, "joined room:", roomID, "as:", joinPayload.GetDisplayName())
	return playerID, true
}

func HandleAllMessages(manager *game_room.RoomManager, roomID string, playerID string, conn *websocket.Conn) {
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Error reading message:", err)
			return
		}
		if messageType != websocket.BinaryMessage {
			sendError(conn, "invalid_payload", "Messages must be protobuf binary")
			continue
		}

		var clientMessage protobufs.ClientToServerMessage
		if err := proto.Unmarshal(payload, &clientMessage); err != nil {
			sendError(conn, "invalid_payload", "Unable to decode message")
			continue
		}

		switch clientMessage.GetMessageType() {
		case protobufs.ClientToServerMessageType_SEND_CHAT_MESSAGE:
			handleChatMessage(manager, roomID, playerID, conn, clientMessage.GetChatMessagePayload())
		case protobufs.ClientToServerMessageType_SEND_GAME_UPDATE:
			handleGameUpdate(manager, roomID, playerID, conn, clientMessage.GetGameUpdatePayload())
		case protobufs.ClientToServerMessageType_REQUEST_GAME_STATE:
			handleGameState(manager, roomID, conn)
		case protobufs.ClientToServerMessageType_JOIN_ROOM:
			sendError(conn, "already_joined", "Player is already joined")
		default:
			sendError(conn, "unsupported_message", "Unsupported message type")
		}
	}
}

func handleChatMessage(manager *game_room.RoomManager, roomID string, playerID string, conn *websocket.Conn, payload *protobufs.ChatMessagePayload) {
	if payload == nil {
		sendError(conn, "invalid_chat", "Chat payload is required")
		return
	}

	chatPayload, gameErr := manager.AddChatMessage(roomID, playerID, payload.GetContent(), time.Now())
	if gameErr != nil {
		sendGameError(conn, gameErr)
		return
	}

	broadcast(manager, roomID, &protobufs.ServerToClientMessage{
		MessageType: protobufs.ServerToClientMessageType_BROADCAST_CHAT_MESSAGE,
		Payload: &protobufs.ServerToClientMessage_ChatMessagePayload{
			ChatMessagePayload: chatPayload,
		},
	})
}

func handleGameUpdate(manager *game_room.RoomManager, roomID string, playerID string, conn *websocket.Conn, payload *protobufs.GameUpdatePayload) {
	if payload == nil {
		sendError(conn, "invalid_game_update", "Game update payload is required")
		return
	}

	result, roomUpdate, gameErr := manager.ApplyGameUpdate(roomID, playerID, payload, time.Now())
	if gameErr != nil {
		sendGameError(conn, gameErr)
		return
	}

	if result.Update != nil {
		broadcast(manager, roomID, &protobufs.ServerToClientMessage{
			MessageType: protobufs.ServerToClientMessageType_BROADCAST_GAME_UPDATE,
			Payload: &protobufs.ServerToClientMessage_GameUpdatePayload{
				GameUpdatePayload: result.Update,
			},
		})
	}

	if result.State != nil {
		broadcast(manager, roomID, &protobufs.ServerToClientMessage{
			MessageType: protobufs.ServerToClientMessageType_RESPOND_GAME_STATE,
			Payload: &protobufs.ServerToClientMessage_GameStatePayload{
				GameStatePayload: result.State,
			},
		})
	}

	if roomUpdate != nil {
		broadcast(manager, roomID, &protobufs.ServerToClientMessage{
			MessageType: protobufs.ServerToClientMessageType_BROADCAST_ROOM_UPDATE,
			Payload: &protobufs.ServerToClientMessage_RoomUpdatePayload{
				RoomUpdatePayload: roomUpdate,
			},
		})
	}
}

func handleGameState(manager *game_room.RoomManager, roomID string, conn *websocket.Conn) {
	state, gameErr := manager.GameState(roomID)
	if gameErr != nil {
		sendGameError(conn, gameErr)
		return
	}

	send(conn, &protobufs.ServerToClientMessage{
		MessageType: protobufs.ServerToClientMessageType_RESPOND_GAME_STATE,
		Payload: &protobufs.ServerToClientMessage_GameStatePayload{
			GameStatePayload: state,
		},
	})
}

func sendGameError(conn *websocket.Conn, gameErr *games.GameError) {
	sendError(conn, gameErr.Code, gameErr.Message)
}

func sendError(conn *websocket.Conn, code string, message string) {
	send(conn, &protobufs.ServerToClientMessage{
		MessageType: protobufs.ServerToClientMessageType_SEND_ERROR,
		Payload: &protobufs.ServerToClientMessage_ErrorPayload{
			ErrorPayload: &protobufs.ServerErrorPayload{
				Code:    code,
				Message: message,
			},
		},
	})
}

func send(conn *websocket.Conn, message *protobufs.ServerToClientMessage) {
	data, err := proto.Marshal(message)
	if err != nil {
		fmt.Println("Error marshaling message:", err)
		return
	}
	lockValue, _ := connectionWriteLocks.LoadOrStore(conn, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		fmt.Println("Error sending message:", err)
	}
}

func broadcast(manager *game_room.RoomManager, roomID string, message *protobufs.ServerToClientMessage) {
	room, ok := manager.GetRoom(roomID)
	if !ok {
		return
	}
	for _, conn := range room.ConnectionsSnapshot() {
		send(conn, message)
	}
}

func roomIDFromWebSocketPath(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "rooms" || parts[2] != "ws" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
