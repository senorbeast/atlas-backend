package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
	"google.golang.org/protobuf/proto"
)

var playerId string

func main() {
	reader := bufio.NewReader(os.Stdin)
	done := make(chan struct{})
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	var conn *websocket.Conn
	var roomID string

	for {
		printMenu()
		fmt.Print("Select an option: ")
		option, _ := reader.ReadString('\n')
		option = strings.TrimSpace(option)

		switch option {
		case "1":
			newRoomID, err := createGameRoom()
			if err != nil {
				log.Println("Error creating game room:", err)
			} else {
				fmt.Printf("Room created with ID: %s\n", newRoomID)
				roomID = newRoomID
				conn, err = connectToWebSocket(roomID)
				if err != nil {
					log.Println("Error connecting to WebSocket:", err)
				} else {
					fmt.Println("Connected to WebSocket")
					// Start the goroutine to listen to the connection
					go listenToConnection(conn)
					// Set the global playerId variable with the value received from the server
					messageLoop(reader, conn)
				}
			}
		case "2":
			fmt.Print("Enter existing room ID: ")
			roomID, _ = reader.ReadString('\n')
			roomID = strings.TrimSpace(roomID)
			conn, err := connectToWebSocket(roomID)
			if err != nil {
				log.Println("Error connecting to WebSocket:", err)
				fmt.Println("Invalid room ID.")
				roomID = ""
			} else {
				fmt.Println("Connected to WebSocket")
				// Start the goroutine to listen to the connection
				go listenToConnection(conn)
				// Set the global playerId variable with the value received from the server
				messageLoop(reader, conn)
			}
		case "q":
			fmt.Println("Exiting...")
			close(done)
			if conn != nil {
				conn.Close()
			}
			<-done // Wait for the channel to be closed
			return
		default:
			fmt.Println("Invalid option. Please select a valid option.")
		}
	}
}

func messageLoop(reader *bufio.Reader, conn *websocket.Conn) {
	for {
		fmt.Print("Enter message: ")
		message, _ := reader.ReadString('\n')
		message = strings.TrimSpace(message)

		if message == "q" {
			fmt.Println("Exiting Message mode...")
			conn.Close()

		}

		if message != "" {
			if conn != nil {
				// Send the typed message as a chat message
				sendChatMessage(conn, playerId, message)
			}
		}

	}
}

func listenToConnection(conn *websocket.Conn) {
	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.Println("Error reading message:", err)
			}
			return
		}

		// Unmarshal the received message into ServerToClientMessage
		var serverMessage protobufs.ServerToClientMessage
		if err := proto.Unmarshal(p, &serverMessage); err != nil {
			log.Println("Error unmarshaling message:", err)
			return
		}

		// Process the message based on its type
		switch serverMessage.MessageType {
		case protobufs.ServerToClientMessageType_BROADCAST_CHAT_MESSAGE:
			// Handle chat message
			chatMessage := serverMessage.GetChatMessagePayload()
			fmt.Printf("[%s]: %s\n", chatMessage.SenderId, chatMessage.Content)
		case protobufs.ServerToClientMessageType_SEND_ON_CONNECT_ACK:
			// Handle connect ack message and save sender ID
			ackPayload := serverMessage.GetOnConnectAckPayload()
			fmt.Printf("Connected to room %s. Your Player ID is: %s\n", ackPayload.RoomId, ackPayload.PlayerId)
			playerId = ackPayload.PlayerId // Update the playerId directly
			// Add cases for other message types as needed
		}
	}
}

func createGameRoom() (string, error) {
	resp, err := http.Get("http://localhost:8080/create")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var responseData struct {
		RoomID string `json:"roomId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return "", err
	}

	return responseData.RoomID, nil
}

func connectToWebSocket(roomID string) (*websocket.Conn, error) {
	url := fmt.Sprintf("ws://localhost:8080/%s", roomID)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func sendChatMessage(conn *websocket.Conn, senderID, messageContent string) {
	chatMessage := &protobufs.ChatMessagePayload{
		SenderId: playerId, // Use the global playerId variable as the sender ID
		Content:  messageContent,
	}

	clientMessage := &protobufs.ClientToServerMessage{
		MessageType: protobufs.ClientToServerMessageType_SEND_CHAT_MESSAGE,
		Payload: &protobufs.ClientToServerMessage_ChatMessagePayload{
			ChatMessagePayload: chatMessage,
		},
	}

	// Marshal the client message and send it to the server
	messageData, err := proto.Marshal(clientMessage)
	if err != nil {
		log.Println("Error marshaling chat message:", err)
		return
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, messageData); err != nil {
		log.Println("Error sending chat message:", err)
	}
}

func printMenu() {
	fmt.Println("===== Menu =====")
	fmt.Println("1. Create Room and Connect")
	fmt.Println("2. Connect to Existing Room")
	fmt.Println("q. Quit")
	fmt.Println("================")
}
