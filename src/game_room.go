// game_room.go - Contains the game room handling logic and WebSocket server management

package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/src/protobuf/game_proto"
	// Import the generated Go code
)

type GameRoom struct {
	RoomID       string
	players      map[string]*game_proto.PlayerData
	playersMux   sync.Mutex
	lastActivity time.Time // Track the last activity time
}

func (gr *GameRoom) checkWebSocketActivity() {
	// Define the inactivity threshold (e.g., 5 minutes)
	const inactivityThreshold = 5 * time.Minute

	for {
		time.Sleep(time.Minute) // Check activity periodically

		gr.playersMux.Lock()
		// Check if there are any players in the room
		if len(gr.players) == 0 {
			gr.playersMux.Unlock()
			break // Close the WebSocket server if there are no players
		}

		// Check if there has been any activity in the last 'inactivityThreshold' duration
		if time.Since(gr.lastActivity) >= inactivityThreshold {
			// Close the WebSocket server due to inactivity
			gr.playersMux.Unlock()
			gr.closeWebSocketServer()
			break
		}
		gr.playersMux.Unlock()
	}
}

func (gr *GameRoom) closeWebSocketServer() {
	// ... (Logic to gracefully close the WebSocket server, including closing connections)
}

func (gr *GameRoom) handleWebSocketConnection(conn *websocket.Conn) {
	// ... (Logic to handle a single WebSocket connection)
	gr.playersMux.Lock()
	gr.lastActivity = time.Now() // Update the last activity time
	gr.playersMux.Unlock()
	// ... (Rest of the logic to handle the WebSocket connection)
}

func (gr *GameRoom) handleWebSocketConnections() {
	http.HandleFunc("/ws/"+gr.RoomID, func(w http.ResponseWriter, r *http.Request) {
		// Upgrade HTTP connection to WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("Error upgrading connection:", err)
			return
		}
		defer conn.Close()

		gr.playersMux.Lock()
		gr.lastActivity = time.Now() // Update the last activity time
		gr.playersMux.Unlock()

		// ... (Rest of the logic to handle WebSocket connections)
	})

	// Start the WebSocket server for the game room
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Error starting WebSocket server for room", gr.RoomID, ":", err)
	}
}
