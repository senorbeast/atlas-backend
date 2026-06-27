package games

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

func TestAtlasWordGameValidatesCitiesAndDuplicates(t *testing.T) {
	setupCityFixture(t)
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	game, gameErr := NewAtlasWordGame(now)
	if gameErr != nil {
		t.Fatalf("NewAtlasWordGame() error = %v", gameErr)
	}

	result, gameErr := game.Apply(PlayerAction{
		PlayerID:   "p1",
		PlayerName: "Ada",
		Type:       "submit_city",
		CityName:   "Sydney",
		Now:        now,
	})
	if gameErr != nil {
		t.Fatalf("Apply(Sydney) error = %v", gameErr)
	}
	if got := result.State.GetCurrentLetter(); got != "y" {
		t.Fatalf("current letter = %q, want y", got)
	}
	if got := result.Update.GetAcceptedCity().GetSubmittedByName(); got != "Ada" {
		t.Fatalf("submitted name = %q, want Ada", got)
	}
	if got := result.Update.GetAcceptedCity().GetCityHash(); got == "" {
		t.Fatal("accepted city should include city hash")
	}

	_, gameErr = game.Apply(PlayerAction{PlayerID: "p1", PlayerName: "Ada", Type: "submit_city", CityName: "Sydney", Now: now})
	if gameErr == nil || gameErr.Code != "duplicate_city" {
		t.Fatalf("duplicate error = %#v, want duplicate_city", gameErr)
	}

	_, gameErr = game.Apply(PlayerAction{PlayerID: "p1", PlayerName: "Ada", Type: "submit_city", CityName: "Kyoto", Now: now})
	if gameErr == nil || gameErr.Code != "wrong_letter" {
		t.Fatalf("wrong-letter error = %#v, want wrong_letter", gameErr)
	}
}

func TestAtlasWordGameRejectsUnknownCity(t *testing.T) {
	setupCityFixture(t)
	game, gameErr := NewAtlasWordGame(time.Now())
	if gameErr != nil {
		t.Fatalf("NewAtlasWordGame() error = %v", gameErr)
	}

	_, gameErr = game.Apply(PlayerAction{PlayerID: "p1", PlayerName: "Ada", Type: "submit_city", CityName: "Snotacity"})
	if gameErr == nil || gameErr.Code != "unknown_city" {
		t.Fatalf("unknown city error = %#v, want unknown_city", gameErr)
	}
}

func TestAtlasWordGameRejectsEmptyAndUnsupportedActions(t *testing.T) {
	setupCityFixture(t)
	game, gameErr := NewAtlasWordGame(time.Now())
	if gameErr != nil {
		t.Fatalf("NewAtlasWordGame() error = %v", gameErr)
	}

	_, gameErr = game.Apply(PlayerAction{PlayerID: "p1", PlayerName: "Ada", Type: "dance", CityName: "Sydney"})
	if gameErr == nil || gameErr.Code != "invalid_game_action" {
		t.Fatalf("unsupported action error = %#v, want invalid_game_action", gameErr)
	}

	_, gameErr = game.Apply(PlayerAction{PlayerID: "p1", PlayerName: "Ada", Type: "submit_city", CityName: "   "})
	if gameErr == nil || gameErr.Code != "empty_city" {
		t.Fatalf("empty city error = %#v, want empty_city", gameErr)
	}
}

func TestAtlasWordGameNormalizesCityNames(t *testing.T) {
	setupCityFixture(t)
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	game, gameErr := NewAtlasWordGame(now)
	if gameErr != nil {
		t.Fatalf("NewAtlasWordGame() error = %v", gameErr)
	}

	result, gameErr := game.Apply(PlayerAction{
		PlayerID:   "p1",
		PlayerName: "Ada",
		Type:       "submit_city",
		CityName:   "  sydney  ",
		Now:        now,
	})
	if gameErr != nil {
		t.Fatalf("Apply(normalized Sydney) error = %v", gameErr)
	}
	if got := result.Update.GetAcceptedCity().GetName(); got != "Sydney" {
		t.Fatalf("accepted city name = %q, want canonical Sydney", got)
	}
}

func TestAtlasWordGameEndsAtCityLimitAndExpiry(t *testing.T) {
	setupCityFixture(t)
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	game, gameErr := NewAtlasWordGame(now)
	if gameErr != nil {
		t.Fatalf("NewAtlasWordGame() error = %v", gameErr)
	}

	game.acceptedCities = make([]*protobufs.AcceptedCity, atlasMaxCities-1)
	game.currentLetter = "s"

	_, gameErr = game.Apply(PlayerAction{PlayerID: "p1", PlayerName: "Ada", Type: "submit_city", CityName: "Sydney", Now: now})
	if gameErr != nil {
		t.Fatalf("Apply at city limit error = %v", gameErr)
	}
	if !game.IsOver() {
		t.Fatal("game should be over at city limit")
	}

	expiredGame, gameErr := NewAtlasWordGame(now)
	if gameErr != nil {
		t.Fatalf("NewAtlasWordGame() error = %v", gameErr)
	}
	_, gameErr = expiredGame.Apply(PlayerAction{
		PlayerID:   "p1",
		PlayerName: "Ada",
		Type:       "submit_city",
		CityName:   "Sydney",
		Now:        now.Add(atlasGameDuration + time.Second),
	})
	if gameErr == nil || gameErr.Code != "game_expired" {
		t.Fatalf("expired game error = %#v, want game_expired", gameErr)
	}
}

func setupCityFixture(t *testing.T) {
	t.Helper()
	citiesOnce = sync.Once{}
	citiesData = nil
	citiesErr = nil

	path := filepath.Join(t.TempDir(), "cities.json")
	data := `[
		{"id": 1, "name": "Sydney", "country": "AU", "lat": "-33.8678500", "lon": "151.2073200"},
		{"id": 2, "name": "York", "country": "GB", "lat": "53.9599650", "lon": "-1.0872980"},
		{"id": 3, "name": "Kyoto", "country": "JP", "lat": "35.0116000", "lon": "135.7681000"}
	]`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write city fixture: %v", err)
	}
	t.Setenv("ATLAS_CITIES_PATH", path)
}
