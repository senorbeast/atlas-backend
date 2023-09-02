package web_socket

// internal/websocket_handler.go - Contains the WebSocket handling logic

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
	rmt "github.com/senorbeast/atlas-backend/internal/web_socket/handle_messages/response_message_types"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all connections. You may want to add additional security checks here.
		return true
	},
}

func HandleWebSocketConnections(gr *game_room.GameRoom) {
	http.HandleFunc("/"+gr.RoomID, func(w http.ResponseWriter, r *http.Request) {
		// Initial Handshake, with client/player
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("Error upgrading connection:", err)
			return
		}
		defer conn.Close()

		gr.PlayersMux.Lock()
		// Associate the player's connection with their player ID
		playerID := generatePlayerID()

		player := &protobufs.PlayerData{
			PlayerId: playerID,
		}

		// Create payload with room and player information
		onConnectAckPayload := &protobufs.OnConnectAckPayload{
			RoomId:   gr.RoomID,
			PlayerId: playerID,
		}

		// Create the ServerToClientMessage
		ackMessage := &protobufs.ServerToClientMessage{
			MessageType: protobufs.ServerToClientMessageType_SEND_ON_CONNECT_ACK,
			Payload: &protobufs.ServerToClientMessage_OnConnectAckPayload{
				OnConnectAckPayload: onConnectAckPayload,
			},
		}

		fmt.Println("Player:", playerID, "Connected to:", gr.RoomID)
		rmt.SendMessage(conn, ackMessage)

		// TODO: Check for unique ids
		gr.PlayerData[playerID] = &game_room.PlayerConnection{
			Player: player,
			Conn:   conn,
		}
		gr.LastActivity = time.Now()
		gr.PlayersMux.Unlock()

		// Start handling messages from the player's connection
		HandleAllMessage(gr, conn)
	})

	// No need to start the WebSocket server here
	// The server will be started from the main function
}

func generatePlayerID() string {
	// Generate a random player ID (you might want to make this more robust)
	randomBytes := make([]byte, 8)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", randomBytes)
}

// func (gr *GameRoom) removePlayer(playerID string) {
// 	gr.playersMux.Lock()
// 	defer gr.playersMux.Unlock()

// 	// Close the player's connection if it exists
// 	if pc, exists := gr.playerData[playerID]; exists {
// 		pc.Conn.Close()
// 	}

// 	// Remove the player's data from the map
// 	delete(gr.playerData, playerID)
// }
