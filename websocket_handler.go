// websocket_handler.go - Contains the WebSocket handling logic

package main

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/protobuf/game_proto"
)

func HandleWebSocketConnections(gr *GameRoom) {
	http.HandleFunc("/ws/"+gr.RoomID, func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("Error upgrading connection:", err)
			return
		}
		defer conn.Close()

		gr.playersMux.Lock()
		// Associate the player's connection with their player ID
		playerID := generatePlayerID() // You need a way to generate player IDs

		player := &game_proto.PlayerData{
			PlayerId: playerID,
		}

		gr.playerData[playerID] = &PlayerConnection{
			Player: player,
			Conn:   conn,
		}
		gr.lastActivity = time.Now()
		gr.playersMux.Unlock()

		// Start handling messages for the player's connection
		handleMessage(gr, conn)
	})

	// No need to start the WebSocket server here
	// The server will be started from the main function
}

func handleMessage(gr *GameRoom, conn *websocket.Conn) {
	// Find the player ID based on the connection
	var playerID string
	for id, pc := range gr.playerData {
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
			chatMessage := &game_proto.ChatMessage{
				SenderId: playerID,
				Content:  content,
			}

			// Broadcast the chat message to all players in the game room
			gr.playersMux.Lock()
			defer gr.playersMux.Unlock()

			for id, pc := range gr.playerData {
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

func (gr *GameRoom) removePlayer(playerID string) {
	gr.playersMux.Lock()
	defer gr.playersMux.Unlock()

	// Close the player's connection if it exists
	if pc, exists := gr.playerData[playerID]; exists {
		pc.Conn.Close()
	}

	// Remove the player's data from the map
	delete(gr.playerData, playerID)
}
