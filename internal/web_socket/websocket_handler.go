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
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all connections. You may want to add additional security checks here.
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
		defer conn.Close()

		// Respond with the game room ID to the frontend
		fmt.Fprintf(w, "{\"Created Websocket\": \"Welcome to game: %s\"}", gr.RoomID)

		gr.PlayersMux.Lock()
		// Associate the player's connection with their player ID
		playerID := generatePlayerID() // You need a way to generate player IDs

		player := &protobufs.PlayerData{
			PlayerId: playerID,
		}

		gr.PlayerData[playerID] = &game_room.PlayerConnection{
			Player: player,
			Conn:   conn,
		}
		gr.LastActivity = time.Now()
		gr.PlayersMux.Unlock()

		// Start handling messages for the player's connection
		handleMessage(gr, conn)
	})

	// No need to start the WebSocket server here
	// The server will be started from the main function
}

func handleMessage(gr *game_room.GameRoom, conn *websocket.Conn) {
	// Find the player ID based on the connection
	var playerID string
	for id, pc := range gr.PlayerData {
		if pc.Conn == conn {
			playerID = id
			break
		}
	}

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Error reading message:", err)
			break
		}

		// Handle the received message based on its type
		if messageType == websocket.TextMessage {
			content := string(p)
			// Process the chat message or game event here
			// You can use the playerID to identify the sender
			// and the content of the message for the message itself
			chatMessage := &protobufs.ChatMessage{
				SenderId: playerID,
				Content:  content,
			}

			// Broadcast the chat message to all players in the game room
			gr.PlayersMux.Lock()
			defer gr.PlayersMux.Unlock()

			for id, pc := range gr.PlayerData {
				if id != playerID {
					err := pc.Conn.WriteJSON(chatMessage)
					if err != nil {
						fmt.Println("Error sending chat message:", err)
					}
				}
			}
		}
	}
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
