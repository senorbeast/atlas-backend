package response_type

import (
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
	"google.golang.org/protobuf/proto"
)

// broadcastMessage broadcasts a message to all connected clients.
func BroadcastMessage(gr *game_room.GameRoom, message *protobufs.ServerToClientMessage) {
	gr.PlayersMux.Lock()
	defer gr.PlayersMux.Unlock()

	responseData, err := proto.Marshal(message)
	if err != nil {
		fmt.Println("Error marshaling message:", err)
		return
	}

	for _, pc := range gr.PlayerData {
		err := pc.Conn.WriteMessage(websocket.BinaryMessage, responseData)
		if err != nil {
			fmt.Println("Error broadcasting message:", err)
		}
	}
}
