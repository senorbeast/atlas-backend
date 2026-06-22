package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/web_socket"
)

var roomManager = game_room.NewRoomManager()

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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	roomID, err := generateRoomID()
	if err != nil {
		http.Error(w, "Failed to generate RoomID", http.StatusInternalServerError)
		return
	}

	gameKind := r.URL.Query().Get("gameKind")
	turnMode := r.URL.Query().Get("turnMode")
	room, err := roomManager.CreateRoom(roomID, gameKind, turnMode, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"roomId":   room.RoomID,
		"gameKind": room.GameKind,
		"status":   room.Status,
		"turnMode": room.TurnMode,
	})
}

func roomRoutesHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] != "rooms" {
		http.NotFound(w, r)
		return
	}

	switch parts[2] {
	case "ws":
		web_socket.HandleWebSocketConnections(roomManager)(w, r)
	case "chat":
		chatHistoryHandler(parts[1], w, r)
	default:
		http.NotFound(w, r)
	}
}

func chatHistoryHandler(roomID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			http.Error(w, "Invalid limit", http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	page, err := roomManager.ChatPage(roomID, limit, r.URL.Query().Get("before"))
	if err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		fmt.Println("Error writing JSON:", err)
	}
}

func startCleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for now := range ticker.C {
			removed := roomManager.CleanupExpired(now)
			for _, roomID := range removed {
				fmt.Println("Cleaned up room:", roomID)
			}
		}
	}()
}

func main() {
	http.HandleFunc("/create", createGameRoomHandler)
	http.HandleFunc("/rooms/", roomRoutesHandler)
	startCleanupLoop()

	port := os.Getenv("ATLAS_PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Running atlas-backend")
	fmt.Printf("Create rooms at http://localhost:%s/create\n", port)
	fmt.Printf("Join rooms at ws://localhost:%s/rooms/{roomId}/ws\n", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Println("Error starting HTTP server:", err)
	}
}
