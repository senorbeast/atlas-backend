// main.go - Contains the HTTP API handler for creating game rooms

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/src/protobuf/game_proto"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all connections. You may want to add additional security checks here.
		return true
	},
}

var (
	gameRooms    = make(map[string]*GameRoom)
	gameRoomsMux sync.Mutex
)

func generateRoomID() (string, error) {
	const roomIDLength = 6
	randomBytes := make([]byte, roomIDLength)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(randomBytes)
	return base64.URLEncoding.EncodeToString(hash[:roomIDLength]), nil
}

func createGameRoomHandler(w http.ResponseWriter, r *http.Request) {
	roomID, err := generateRoomID()
	if err != nil {
		http.Error(w, "Failed to generate RoomID", http.StatusInternalServerError)
		return
	}

	gameRoom := &GameRoom{
		RoomID:  roomID,
		players: make(map[string]*game_proto.PlayerData),
	}

	gameRoomsMux.Lock()
	gameRooms[roomID] = gameRoom
	gameRoomsMux.Unlock()

	// Start a separate goroutine to handle WebSocket connections for this room
	go func() {
		gameRoom.handleWebSocketConnections()
	}()

	// Respond with the game room ID to the frontend
	fmt.Fprintf(w, "{\"roomId\": \"%s\"}", gameRoom.RoomID)
}

func main() {
	http.HandleFunc("/create", createGameRoomHandler)

	// Start the HTTP server
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Error starting HTTP server:", err)
	}
}
