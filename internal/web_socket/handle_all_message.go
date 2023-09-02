package web_socket

/* Message is recieved from a client, response to client(s) accordingly
Response can be to the sender, or to any/all clients
*/

import (
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
	hm "github.com/senorbeast/atlas-backend/internal/web_socket/handle_messages"
	"google.golang.org/protobuf/proto"
)

func HandleAllMessage(gr *game_room.GameRoom, conn *websocket.Conn) {
	// Find the player ID based on the connection
	// var playerID string
	// for id, pc := range gr.PlayerData {
	// 	if pc.Conn == conn {
	// 		playerID = id
	// 		break
	// 	}
	// }

	// Keep listening for messages
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Error reading message:", err)
			break
		}

		if messageType == websocket.BinaryMessage {
			var clientMessage protobufs.ClientToServerMessage
			if err := proto.Unmarshal(p, &clientMessage); err != nil {
				fmt.Println("Error unmarshaling message:", err)
				continue
			}

			// Process the message based on its type
			switch clientMessage.GetMessageType() {
			// ? No need to unmarshal chat message
			case protobufs.ClientToServerMessageType_SEND_CHAT_MESSAGE:
				content := clientMessage.GetChatMessagePayload().GetContent()
				hm.HandleChatMessage(gr, conn, content)
			case protobufs.ClientToServerMessageType_SEND_GAME_UPDATE:
				payload := clientMessage.GetGameUpdatePayload()
				hm.HandleGameUpdate(gr, conn, payload)
			case protobufs.ClientToServerMessageType_REQUEST_GAME_STATE:
				hm.HandleGameState(gr, conn)
				// Add more cases for other message types

			}
		}
	}
}
