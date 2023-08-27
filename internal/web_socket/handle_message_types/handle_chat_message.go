package handle_message_types

import (
	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
	rt "github.com/senorbeast/atlas-backend/internal/web_socket/handle_message_types/response_types"
)

func HandleChatMessage(gr *game_room.GameRoom, conn *websocket.Conn, content string) {
	var senderID string
	for id, pc := range gr.PlayerData {
		if pc.Conn == conn {
			senderID = id
			break
		}
	}
	// Process chat message and generate a response
	response := generateChatResponse(senderID, content)
	// rt.SendResponse(conn, response)

	// Broadcast the chat message to all players
	rt.BroadcastMessage(gr, response)
}

// generateChatResponse generates a response for a chat message.
func generateChatResponse(senderID, content string) *protobufs.ServerToClientMessage {
	// Process the content and generate a response
	return &protobufs.ServerToClientMessage{
		MessageType: protobufs.ServerToClientMessageType_BROADCAST_CHAT_MESSAGE,
		Payload: &protobufs.ServerToClientMessage_ChatMessagePayload{
			ChatMessagePayload: &protobufs.ChatMessagePayload{
				SenderId: senderID,
				Content:  content,
			},
		},
	}
}
