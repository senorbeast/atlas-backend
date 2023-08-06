// websocket_handler.go - Contains the WebSocket handling logic

package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	multiplayer "github.com/senorbeast/atlas-backend/multiplayer" // Import the generated Go code
)

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
