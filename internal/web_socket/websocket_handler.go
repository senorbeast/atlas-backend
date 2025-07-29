package web_socket

import (
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func HandleWebSocketConnections(gr *game_room.GameRoom) {
	http.HandleFunc("/ws/"+gr.RoomID, func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("Error upgrading connection:", err)
			return
		}

		playerID := generatePlayerID()

		// Create payload with room and player information
		onConnectAckPayload := &protobufs.OnConnectAckPayload{
			RoomId:   gr.RoomID,
			PlayerId: playerID,
			IsHost:   len(gr.Players) == 0,
		}

		// Create the ServerToClientMessage
		ackMessage := &protobufs.ServerClientMessage{
			MessageType: protobufs.ServerClientMessageType_ON_CONNECT_ACK,
			Payload: &protobufs.ServerClientMessage_OnConnectAck{
				OnConnectAck: onConnectAckPayload,
			},
		}

		data, err := proto.Marshal(ackMessage)
		if err != nil {
			log.Printf("error marshalling ack message: %v", err)
			conn.Close()
			return
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			log.Printf("error sending ack message: %v", err)
			conn.Close()
			return
		}

		fmt.Println("Player:", playerID, "Connected to:", gr.RoomID)

		// Pass handling to the message handler
		HandleAllMessage(gr, conn, playerID)
	})
}

func generatePlayerID() string {
	randomBytes := make([]byte, 8)
	_, err := rand.Read(randomBytes)
	if err != nil {
		// Fallback to a less random source if crypto/rand fails
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", randomBytes)
}