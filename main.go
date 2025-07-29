package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/web_socket"
)

var (
	gameRooms    = make(map[string]*game_room.GameRoom)
	gameRoomsMux sync.Mutex
)

func createGameRoomHandler(w http.ResponseWriter, r *http.Request) {
	roomID := uuid.New().String()[:8] // Short UUID for room ID
	gr := game_room.NewGameRoom(roomID)

	gameRoomsMux.Lock()
	gameRooms[roomID] = gr
	gameRoomsMux.Unlock()

	go web_socket.HandleWebSocketConnections(gr)

	log.Printf("Room created: %s", roomID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"roomId": roomID})
}

func listRoomsHandler(w http.ResponseWriter, r *http.Request) {
	gameRoomsMux.Lock()
	defer gameRoomsMux.Unlock()

	type roomInfo struct {
		RoomID      string `json:"roomId"`
		PlayerCount int    `json:"playerCount"`
		IsStarted   bool   `json:"isStarted"`
	}

	rooms := make([]roomInfo, 0, len(gameRooms))
	for _, gr := range gameRooms {
		rooms = append(rooms, roomInfo{
			RoomID:      gr.RoomID,
			PlayerCount: len(gr.Players),
			IsStarted:   gr.IsStarted,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func main() {
	http.HandleFunc("/create", createGameRoomHandler)
	http.HandleFunc("/rooms", listRoomsHandler)
	http.HandleFunc("/health", healthCheckHandler)

	// Cleanup inactive rooms periodically
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			gameRoomsMux.Lock()
			for id, gr := range gameRooms {
				if time.Since(gr.LastActivity) > 30*time.Minute {
					delete(gameRooms, id)
					log.Printf("Cleaned up inactive room: %s", id)
				}
			}
			gameRoomsMux.Unlock()
		}
	}()

	fmt.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}