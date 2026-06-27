package game_room

import "github.com/senorbeast/atlas-backend/internal/protobufs"

// ScorePolicy defines how accepted actions mutate player score metadata.
type ScorePolicy interface {
	ApplyAcceptedMove(player *protobufs.PlayerData)
}

// AcceptedCityScorePolicy awards one point for each accepted Atlas city.
type AcceptedCityScorePolicy struct{}

// ApplyAcceptedMove increments the submitting player's score by one.
func (AcceptedCityScorePolicy) ApplyAcceptedMove(player *protobufs.PlayerData) {
	player.Score++
}

var defaultScorePolicy ScorePolicy = AcceptedCityScorePolicy{}
