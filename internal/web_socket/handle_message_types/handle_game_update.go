package handle_message_types

import (
	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

func HandleGameUpdate(gr *game_room.GameRoom, conn *websocket.Conn, payload *protobufs.GameUpdatePayload) {
	// Process game action and generate a response
	// response := generateGameUpdateResponse(playerID, payload)
	// sendResponse(conn, response)

	// Handle specific game actions and updates
}
