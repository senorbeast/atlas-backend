package response_message_type

import (
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
	"google.golang.org/protobuf/proto"
)

// sendResponse marshals and sends a response message.
func SendMessage(conn *websocket.Conn, response *protobufs.ServerToClientMessage) {
	responseData, err := proto.Marshal(response)
	if err != nil {
		fmt.Println("Error marshaling response:", err)
	}

	err = conn.WriteMessage(websocket.BinaryMessage, responseData)
	if err != nil {
		fmt.Println("Error sending response:", err)
	}
}
