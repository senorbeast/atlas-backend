// websocket_handler.go - Contains the WebSocket handling logic

package main

import (
	"fmt"
	"net/http"
	"time"
)

func handleWebSocketConnections(gr *GameRoom) {
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

	// No need to start the WebSocket server here
	// The server will be started from the main function
}
