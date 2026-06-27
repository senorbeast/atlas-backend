package game_room

import (
	"errors"
	"strings"
	"time"

	"github.com/senorbeast/atlas-backend/internal/games"
	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

// ChatMessage is the room-scoped persisted chat shape returned by history pages.
type ChatMessage struct {
	MessageID  string `json:"messageId"`
	SenderID   string `json:"senderId"`
	SenderName string `json:"senderName"`
	Content    string `json:"content"`
	CreatedAt  string `json:"createdAt"`
}

// ChatPage returns latest-first chat messages and an older-page cursor.
type ChatPage struct {
	Messages   []ChatMessage `json:"messages"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

// AddChatMessage validates content, derives sender identity from the room player, and appends history.
func (m *RoomManager) AddChatMessage(roomID string, playerID string, content string, now time.Time) (*protobufs.ChatMessagePayload, *games.GameError) {
	room, ok := m.GetRoom(roomID)
	if !ok {
		return nil, &games.GameError{Code: "room_not_found", Message: "Room not found"}
	}

	trimmed, gameErr := validateChatContent(content)
	if gameErr != nil {
		return nil, gameErr
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	player, ok := room.Players[playerID]
	if !ok {
		return nil, &games.GameError{Code: "unknown_player", Message: "Player is not in the room"}
	}

	id := generateID("msg")
	createdAt := now.UTC().Format(time.RFC3339Nano)
	message := ChatMessage{
		MessageID:  id,
		SenderID:   playerID,
		SenderName: player.Name,
		Content:    trimmed,
		CreatedAt:  createdAt,
	}
	room.ChatHistory = append(room.ChatHistory, message)
	room.LastActivity = now

	return &protobufs.ChatMessagePayload{
		MessageId:  id,
		SenderId:   playerID,
		SenderName: player.Name,
		Content:    trimmed,
		CreatedAt:  createdAt,
	}, nil
}

// ChatPage returns a latest-first slice of room-scoped chat history.
func (m *RoomManager) ChatPage(roomID string, limit int, before string) (ChatPage, error) {
	room, ok := m.GetRoom(roomID)
	if !ok {
		return ChatPage{}, errors.New("room not found")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	end := len(room.ChatHistory)
	if before != "" {
		for index, message := range room.ChatHistory {
			if message.MessageID == before {
				end = index
				break
			}
		}
	}

	start := end - limit
	if start < 0 {
		start = 0
	}

	messages := make([]ChatMessage, 0, end-start)
	for i := end - 1; i >= start; i-- {
		messages = append(messages, room.ChatHistory[i])
	}

	nextCursor := ""
	if start > 0 && len(messages) > 0 {
		nextCursor = room.ChatHistory[start].MessageID
	}

	return ChatPage{Messages: messages, NextCursor: nextCursor}, nil
}

func validateChatContent(content string) (string, *games.GameError) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", &games.GameError{Code: "empty_chat", Message: "Enter a message first"}
	}
	if len([]rune(trimmed)) > 280 {
		return "", &games.GameError{Code: "chat_too_long", Message: "Messages must be 280 characters or fewer"}
	}
	return trimmed, nil
}
