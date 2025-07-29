package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
	"google.golang.org/protobuf/proto"
)

var playerID string
var playerNames = make(map[string]string)
var namesMux sync.Mutex

func main() {
	reader := bufio.NewReader(os.Stdin)
	var conn *websocket.Conn

	for {
		printMenu()
		fmt.Print("Select an option: ")
		option, _ := reader.ReadString('\n')
		option = strings.TrimSpace(option)

		var err error
		var roomID string

		switch option {
		case "1":
			roomID, err = createGameRoom()
			if err != nil {
				log.Println("Error creating game room:", err)
				continue
			}
			fmt.Printf("Room created with ID: %s\n", roomID)
			fallthrough
		case "2":
			if roomID == "" {
				fmt.Print("Enter existing room ID: ")
				roomID, _ = reader.ReadString('\n')
				roomID = strings.TrimSpace(roomID)
			}
			conn, err = connectToWebSocket(roomID)
			if err != nil {
				log.Println("Error connecting to WebSocket:", err)
				continue
			}

			connected := make(chan bool)
			go listenToConnection(conn, connected)

			// Wait for connection to be acknowledged
			<-connected

			messageLoop(reader, conn)
		case "q":
			fmt.Println("Exiting...")
			if conn != nil {
				conn.Close()
			}
			return
		default:
			fmt.Println("Invalid option.")
		}
	}
}

func messageLoop(reader *bufio.Reader, conn *websocket.Conn) {
	fmt.Print("Enter your name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	sendJoinGame(conn, name)

	fmt.Println("You can now start chatting. Type 'q' to disconnect.")

	for {
		message, _ := reader.ReadString('\n')
		message = strings.TrimSpace(message)

		if message == "q" {
			fmt.Println("Disconnecting...")
			return
		}

		if message != "" {
			sendChatMessage(conn, message)
		}
	}
}

func listenToConnection(conn *websocket.Conn, connected chan bool) {
	defer conn.Close()
	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Println("Error reading message:", err)
			}
			return
		}

		var serverMessage protobufs.ServerClientMessage
		if err := proto.Unmarshal(p, &serverMessage); err != nil {
			log.Println("Error unmarshaling message:", err)
			continue
		}

		namesMux.Lock()
		switch serverMessage.MessageType {
		case protobufs.ServerClientMessageType_BROADCAST_CHAT_MESSAGE:
			chatMessage := serverMessage.GetBroadcastChatMessage()
			senderName, ok := playerNames[chatMessage.SenderId]
			if !ok {
				senderName = "Unknown"
			}
			if playerID != chatMessage.SenderId {
				fmt.Printf("\n[%s]: %s\n", senderName, chatMessage.Content)
			}
		case protobufs.ServerClientMessageType_ON_CONNECT_ACK:
			ackPayload := serverMessage.GetOnConnectAck()
			fmt.Printf("\nConnected to room %s. Your Player ID is: %s\n", ackPayload.RoomId, ackPayload.PlayerId)
			playerID = ackPayload.PlayerId
			connected <- true
		case protobufs.ServerClientMessageType_GAME_STATE_SYNC:
			for _, p := range serverMessage.GetGameStateSync().Players {
				playerNames[p.PlayerId] = p.Name
			}
		case protobufs.ServerClientMessageType_PLAYER_JOINED:
			playerData := serverMessage.GetPlayerJoined().PlayerData
			playerNames[playerData.PlayerId] = playerData.Name
			fmt.Printf("\nPlayer %s joined the room.\n", playerData.Name)
		case protobufs.ServerClientMessageType_PLAYER_LEFT:
			leftPlayerID := serverMessage.GetPlayerLeft().PlayerId
			leftPlayerName, ok := playerNames[leftPlayerID]
			if !ok {
				leftPlayerName = leftPlayerID
			}
			fmt.Printf("\nPlayer %s left the room.\n", leftPlayerName)
			delete(playerNames, leftPlayerID)
		}
		namesMux.Unlock()
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
	url := fmt.Sprintf("ws://localhost:8080/ws/%s", roomID)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func sendJoinGame(conn *websocket.Conn, name string) {
	msg := &protobufs.ClientServerMessage{
		MessageType: protobufs.ClientServerMessageType_PLAYER_JOIN_GAME,
		Payload: &protobufs.ClientServerMessage_PlayerJoinGame{
			PlayerJoinGame: &protobufs.PlayerJoinGamePayload{PlayerName: name},
		},
	}
	data, _ := proto.Marshal(msg)
	conn.WriteMessage(websocket.BinaryMessage, data)
}

func sendChatMessage(conn *websocket.Conn, messageContent string) {
	chatMessage := &protobufs.ChatMessagePayload{
		Content: messageContent,
	}

	clientMessage := &protobufs.ClientServerMessage{
		MessageType: protobufs.ClientServerMessageType_SEND_CHAT_MESSAGE,
		Payload: &protobufs.ClientServerMessage_ChatMessage{
			ChatMessage: chatMessage,
		},
	}

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
	fmt.Println("\n===== Atlas CLI Client =====")
	fmt.Println("1. Create Room and Connect")
	fmt.Println("2. Connect to Existing Room")
	fmt.Println("q. Quit")
	fmt.Println("==========================")
}
