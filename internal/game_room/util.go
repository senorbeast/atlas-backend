package game_room

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

func normalizeDisplayName(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func normalizeTurnMode(value string) (string, error) {
	mode := strings.TrimSpace(value)
	if mode == "" {
		return TurnModeStrict, nil
	}
	switch mode {
	case TurnModeStrict, TurnModeFreeForAll:
		return mode, nil
	default:
		return "", errors.New("invalid turn mode")
	}
}

func generateID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(bytes))
}
