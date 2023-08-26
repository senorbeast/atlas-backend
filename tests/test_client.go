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
)

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
				messageLoop(reader, conn)
			}
		case "q":
			fmt.Println("Exiting...")
			close(done)
			if conn != nil {
				conn.Close()
			}
			select {
			case <-done:
			}
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
			return
		}

		if message != "" {
			if conn != nil {
				err := conn.WriteMessage(websocket.TextMessage, []byte(message))
				if err != nil {
					log.Println("Error sending message:", err)
				}
			}
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
	url := fmt.Sprintf("ws://localhost:8080/ws/%s", roomID)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func printMenu() {
	fmt.Println("===== Menu =====")
	fmt.Println("1. Create Room and Connect")
	fmt.Println("2. Connect to Existing Room")
	fmt.Println("q. Quit")
	fmt.Println("================")
}
