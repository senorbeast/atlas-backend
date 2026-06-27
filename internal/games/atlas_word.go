package games

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/senorbeast/atlas-backend/internal/protobufs"
)

const (
	atlasMaxCities    = 250
	atlasGameDuration = 30 * time.Minute
)

type cityRecord struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
	Lat     string `json:"lat"`
	Lon     string `json:"lon"`
}

type cityEntry struct {
	ID      string
	Name    string
	Country string
	Lat     float64
	Lng     float64
}

type cityIndex struct {
	ByName map[string]cityEntry
}

var (
	citiesOnce sync.Once
	citiesData *cityIndex
	citiesErr  error
)

type AtlasWordGame struct {
	mu             sync.Mutex
	startedAt      time.Time
	expiresAt      time.Time
	currentLetter  string
	acceptedCities []*protobufs.AcceptedCity
	usedCities     map[string]struct{}
	isOver         bool
	endReason      string
}

// NewAtlasWordGame creates an authoritative Atlas word-chain game with city validation.
func NewAtlasWordGame(now time.Time) (*AtlasWordGame, *GameError) {
	if _, err := loadCities(); err != nil {
		return nil, &GameError{Code: "city_data_unavailable", Message: "City data is unavailable"}
	}

	return &AtlasWordGame{
		startedAt:     now,
		expiresAt:     now.Add(atlasGameDuration),
		currentLetter: "s",
		usedCities:    make(map[string]struct{}),
	}, nil
}

// Kind returns the protocol game kind handled by this implementation.
func (g *AtlasWordGame) Kind() string {
	return AtlasWordKind
}

// ExpiresAt returns the hard room/game expiry time.
func (g *AtlasWordGame) ExpiresAt() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.expiresAt
}

// IsOver reports whether the city limit or time limit has ended the game.
func (g *AtlasWordGame) IsOver() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.isOver
}

// Snapshot returns the accepted-city history and current Atlas metadata.
func (g *AtlasWordGame) Snapshot() *protobufs.GameStatePayload {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.snapshotLocked()
}

// Apply validates and applies a player city submission.
func (g *AtlasWordGame) Apply(action PlayerAction) (*ActionResult, *GameError) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := action.Now
	if now.IsZero() {
		now = time.Now()
	}

	if g.isExpiredLocked(now) {
		return nil, &GameError{Code: "game_expired", Message: "The game has ended"}
	}

	if action.Type != "submit_city" {
		return nil, &GameError{Code: "invalid_game_action", Message: "Unsupported game action"}
	}

	rawCityName := strings.TrimSpace(action.CityName)
	if rawCityName == "" {
		return nil, &GameError{Code: "empty_city", Message: "Enter a city name"}
	}

	cities, err := loadCities()
	if err != nil {
		return nil, &GameError{Code: "city_data_unavailable", Message: "City data is unavailable"}
	}

	normalizedName := normalizeName(rawCityName)
	city, ok := cities.ByName[normalizedName]
	if !ok {
		return nil, &GameError{Code: "unknown_city", Message: "That city is not in the Atlas city list"}
	}

	if _, exists := g.usedCities[normalizedName]; exists {
		return nil, &GameError{Code: "duplicate_city", Message: "That city has already been used"}
	}

	if !startsWithLetter(city.Name, g.currentLetter) {
		return nil, &GameError{Code: "wrong_letter", Message: "City must start with the current required letter"}
	}

	accepted := &protobufs.AcceptedCity{
		CityHash:            cityHash(normalizedName),
		Name:                city.Name,
		SubmittedByPlayerId: action.PlayerID,
		SubmittedByName:     action.PlayerName,
		SubmittedAt:         now.UTC().Format(time.RFC3339Nano),
	}

	g.acceptedCities = append(g.acceptedCities, accepted)
	g.usedCities[normalizedName] = struct{}{}
	g.currentLetter = lastLetter(city.Name)

	if len(g.acceptedCities) >= atlasMaxCities {
		g.isOver = true
		g.endReason = "city_limit"
	}

	update := &protobufs.GameUpdatePayload{
		Type:         "city_accepted",
		GameKind:     AtlasWordKind,
		CityName:     city.Name,
		AcceptedCity: accepted,
	}

	return &ActionResult{
		Update: update,
		State:  g.snapshotLocked(),
	}, nil
}

func (g *AtlasWordGame) snapshotLocked() *protobufs.GameStatePayload {
	cities := make([]*protobufs.AcceptedCity, len(g.acceptedCities))
	copy(cities, g.acceptedCities)

	return &protobufs.GameStatePayload{
		IsStarted:      true,
		IsOver:         g.isOver,
		CurrentLetter:  g.currentLetter,
		GameKind:       AtlasWordKind,
		AcceptedCities: cities,
		StartedAt:      g.startedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:      g.expiresAt.UTC().Format(time.RFC3339Nano),
		EndReason:      g.endReason,
	}
}

func (g *AtlasWordGame) isExpiredLocked(now time.Time) bool {
	if g.isOver {
		return true
	}
	if now.Before(g.expiresAt) {
		return false
	}
	g.isOver = true
	g.endReason = "time_limit"
	return true
}

func loadCities() (*cityIndex, error) {
	citiesOnce.Do(func() {
		citiesData, citiesErr = readCities()
	})
	return citiesData, citiesErr
}

func readCities() (*cityIndex, error) {
	path := os.Getenv("ATLAS_CITIES_PATH")
	candidates := []string{}
	if path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates,
		filepath.Clean("../cities500.json"),
		filepath.Clean("../atlas-mutli-game/public/cities.json"),
		filepath.Clean("cities500.json"),
	)

	var data []byte
	for _, candidate := range candidates {
		bytes, err := os.ReadFile(candidate)
		if err == nil {
			data = bytes
			break
		}
	}
	if len(data) == 0 {
		return nil, errors.New("city dataset not found")
	}

	var records []cityRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}

	index := &cityIndex{ByName: make(map[string]cityEntry, len(records))}
	for _, record := range records {
		key := normalizeName(record.Name)
		if key == "" {
			continue
		}
		if _, exists := index.ByName[key]; exists {
			continue
		}
		lat, err := strconv.ParseFloat(record.Lat, 64)
		if err != nil {
			continue
		}
		lng, err := strconv.ParseFloat(record.Lon, 64)
		if err != nil {
			continue
		}
		index.ByName[key] = cityEntry{
			ID:      strconv.Itoa(record.ID),
			Name:    strings.TrimSpace(record.Name),
			Country: strings.TrimSpace(record.Country),
			Lat:     lat,
			Lng:     lng,
		}
	}

	if len(index.ByName) == 0 {
		return nil, errors.New("city dataset is empty")
	}
	return index, nil
}

func normalizeName(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func cityHash(normalizedName string) string {
	sum := sha256.Sum256([]byte(normalizedName))
	return hex.EncodeToString(sum[:])
}

func startsWithLetter(value string, letter string) bool {
	runes := []rune(strings.ToLower(strings.TrimSpace(value)))
	if len(runes) == 0 {
		return false
	}
	return string(runes[0]) == letter
}

func lastLetter(value string) string {
	runes := []rune(strings.ToLower(strings.TrimSpace(value)))
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] >= 'a' && runes[i] <= 'z' {
			return string(runes[i])
		}
	}
	return ""
}
