package main

// main.go

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"

	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/web_socket"
)

var (
	gameRooms    = make(map[string]*game_room.GameRoom)
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

	gameRoom := &game_room.GameRoom{
		RoomID:     roomID,
		PlayerData: make(map[string]*game_room.PlayerConnection),
	}

	gameRoomsMux.Lock()
	gameRooms[roomID] = gameRoom
	gameRoomsMux.Unlock()

	// Start WebSocket handling for the created game room
	go func() {
		web_socket.HandleWebSocketConnections(gameRoom)
	}()

	// Respond with the game room ID to the frontend
	fmt.Fprintf(w, "{\"roomId\": \"%s\"}", gameRoom.RoomID)
}

func main() {
	http.HandleFunc("/create", createGameRoomHandler)

	// Start the HTTP server
	fmt.Println("Running atlas-backend")
	fmt.Println("Visit http://localhost:8080/create")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Error starting HTTP server:", err)
	}
}
