package web_socket

import (
	"log"
	"math/rand"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	"github.com/senorbeast/atlas-backend/internal/game_room"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

func HandleAllMessage(gr *game_room.GameRoom, conn *websocket.Conn, playerID string) {
	defer func() {
		// Handle player disconnection
		playerLeft(gr, playerID)
		conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("error: %v", err)
			break
		}

		var msg protobufs.ClientServerMessage
		if err := proto.Unmarshal(message, &msg); err != nil {
			log.Printf("error unmarshalling message: %v", err)
			continue
		}

		switch msg.MessageType {
		case protobufs.ClientServerMessageType_PLAYER_JOIN_GAME:
			handlePlayerJoinGame(gr, conn, playerID, msg.GetPlayerJoinGame())
		case protobufs.ClientServerMessageType_RECONNECT_PLAYER:
			handleReconnectPlayer(gr, conn, msg.GetReconnectPlayer())
		case protobufs.ClientServerMessageType_TOGGLE_READY_STATE:
			handleToggleReadyState(gr, playerID, msg.GetToggleReadyState())
		case protobufs.ClientServerMessageType_START_GAME:
			handleStartGame(gr, playerID, msg.GetStartGame())
		case protobufs.ClientServerMessageType_SUBMIT_WORD:
			// Placeholder for SUBMIT_WORD handler
		case protobufs.ClientServerMessageType_USE_POWER_UP:
			// Placeholder for USE_POWER_UP handler
		case protobufs.ClientServerMessageType_SEND_CHAT_MESSAGE:
			handleChatMessage(gr, playerID, msg.GetChatMessage())
		default:
			log.Printf("Unknown message type: %v", msg.MessageType)
		}
	}
}

func handleChatMessage(gr *game_room.GameRoom, playerID string, payload *protobufs.ChatMessagePayload) {
	chatMessage := &protobufs.BroadcastChatMessagePayload{
		SenderId: playerID,
		Content:  payload.Content,
	}
	broadcastMessage(gr, protobufs.ServerClientMessageType_BROADCAST_CHAT_MESSAGE, chatMessage, "")
}

func handlePlayerJoinGame(gr *game_room.GameRoom, conn *websocket.Conn, playerID string, payload *protobufs.PlayerJoinGamePayload) {
	player := &protobufs.PlayerData{
		PlayerId:    playerID,
		Name:        payload.PlayerName,
		Hearts:      3, // Default value
		IsReady:     false,
		IsConnected: true,
	}
	gr.AddPlayer(player, conn)

	// Notify other players
	playerJoinedPayload := &protobufs.PlayerJoinedPayload{PlayerData: player}
	broadcastMessage(gr, protobufs.ServerClientMessageType_PLAYER_JOINED, playerJoinedPayload, playerID)

	// Send full game state to the new player
	syncPayload := gr.GetGameStateSyncPayload()
	sendToPlayer(gr, playerID, protobufs.ServerClientMessageType_GAME_STATE_SYNC, syncPayload)
}

func handleReconnectPlayer(gr *game_room.GameRoom, conn *websocket.Conn, payload *protobufs.ReconnectPlayerPayload) {
	playerID := payload.PlayerId
	if p, ok := gr.GetPlayer(playerID); ok {
		p.Conn = conn
		p.Player.IsConnected = true
		gr.Players[playerID] = p

		// Notify other players
		playerDataUpdatePayload := &protobufs.PlayerDataUpdatePayload{PlayerData: p.Player}
		broadcastMessage(gr, protobufs.ServerClientMessageType_PLAYER_DATA_UPDATE, playerDataUpdatePayload, "")

		// Send full game state to the reconnecting player
		syncPayload := gr.GetGameStateSyncPayload()
		sendToPlayer(gr, playerID, protobufs.ServerClientMessageType_GAME_STATE_SYNC, syncPayload)
	} else {
		// Handle error: player not found
		log.Printf("Player not found for reconnect: %s", playerID)
	}
}

func handleToggleReadyState(gr *game_room.GameRoom, playerID string, payload *protobufs.ToggleReadyStatePayload) {
	if p, ok := gr.GetPlayer(playerID); ok {
		p.Player.IsReady = payload.IsReady

		playerDataUpdatePayload := &protobufs.PlayerDataUpdatePayload{PlayerData: p.Player}
		broadcastMessage(gr, protobufs.ServerClientMessageType_PLAYER_DATA_UPDATE, playerDataUpdatePayload, "")
	}
}

func handleStartGame(gr *game_room.GameRoom, playerID string, payload *protobufs.StartGamePayload) {
	if gr.HostPlayerID != playerID {
		sendError(gr, playerID, "Only the host can start the game.")
		return
	}

	// Check if all players are ready
	gr.PlayersMux.Lock()
	allReady := true
	for _, p := range gr.Players {
		if !p.Player.IsReady {
			allReady = false
			break
		}
	}
	gr.PlayersMux.Unlock()

	if !allReady {
		sendError(gr, playerID, "Not all players are ready.")
		return
	}

	gr.IsStarted = true
	gr.Category = payload.Category
	gr.CurrentLetter = randomLetter()
	gr.TurnPlayerID = gr.HostPlayerID // Host starts
	gr.TurnStartTime = time.Now().Unix()
	gr.TurnDurationSeconds = 30 // Example duration

	gameStartedPayload := &protobufs.GameStartedPayload{
		Category:            gr.Category,
		StartingLetter:      gr.CurrentLetter,
		TurnPlayerId:        gr.TurnPlayerID,
		TurnDurationSeconds: gr.TurnDurationSeconds,
	}
	broadcastMessage(gr, protobufs.ServerClientMessageType_GAME_STARTED, gameStartedPayload, "")
}

func playerLeft(gr *game_room.GameRoom, playerID string) {
	if p, ok := gr.GetPlayer(playerID); ok {
		p.Player.IsConnected = false

		playerLeftPayload := &protobufs.PlayerLeftPayload{PlayerId: playerID}
		broadcastMessage(gr, protobufs.ServerClientMessageType_PLAYER_LEFT, playerLeftPayload, "")

		if playerID == gr.HostPlayerID {
			// Handle host leaving - for now, just log it
			log.Printf("Host %s left the game.", playerID)
		}
	}
}

// Helper functions to send messages
func sendToPlayer(gr *game_room.GameRoom, playerID string, msgType protobufs.ServerClientMessageType, payload proto.Message) {
	msg := &protobufs.ServerClientMessage{
		MessageType: msgType,
	}

	// This is tedious, but necessary with the oneof structure
	switch p := payload.(type) {
	case *protobufs.GameStateSyncPayload:
		msg.Payload = &protobufs.ServerClientMessage_GameStateSync{GameStateSync: p}
	// Add other payload types here
	default:
		log.Printf("Unknown payload type for sendToPlayer")
		return
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		log.Printf("error marshalling message: %v", err)
		return
	}
	gr.SendToPlayer(playerID, data)
}

func broadcastMessage(gr *game_room.GameRoom, msgType protobufs.ServerClientMessageType, payload proto.Message, exceptPlayerID string) {
	msg := &protobufs.ServerClientMessage{
		MessageType: msgType,
	}

	// This is tedious, but necessary with the oneof structure
	switch p := payload.(type) {
	case *protobufs.PlayerJoinedPayload:
		msg.Payload = &protobufs.ServerClientMessage_PlayerJoined{PlayerJoined: p}
	case *protobufs.PlayerLeftPayload:
		msg.Payload = &protobufs.ServerClientMessage_PlayerLeft{PlayerLeft: p}
	case *protobufs.PlayerDataUpdatePayload:
		msg.Payload = &protobufs.ServerClientMessage_PlayerDataUpdate{PlayerDataUpdate: p}
	case *protobufs.GameStartedPayload:
		msg.Payload = &protobufs.ServerClientMessage_GameStarted{GameStarted: p}
	case *protobufs.BroadcastChatMessagePayload:
		msg.Payload = &protobufs.ServerClientMessage_BroadcastChatMessage{BroadcastChatMessage: p}
	// Add other payload types here
	default:
		log.Printf("Unknown payload type for broadcast")
		return
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		log.Printf("error marshalling message: %v", err)
		return
	}

	if exceptPlayerID != "" {
		gr.BroadcastExcept(data, exceptPlayerID)
	} else {
		gr.Broadcast(data)
	}
}

func sendError(gr *game_room.GameRoom, playerID string, message string) {
	errorPayload := &protobufs.ErrorMessagePayload{Message: message}
	sendToPlayer(gr, playerID, protobufs.ServerClientMessageType_ERROR_MESSAGE, errorPayload)
}

func randomLetter() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	return string(letters[rand.Intn(len(letters))])
}