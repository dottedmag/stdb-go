package main

import (
	"fmt"

	"go.digitalxero.dev/spacetimedb-server/reducer"
)

//stdb:init
func Init(ctx reducer.ReducerContext) {
	// Log module initialization.
	GameLogTable.Insert(GameLog{
		Message: "Module initialized",
		Level:   0,
	})
}

//stdb:reducer
func CreatePlayer(ctx reducer.ReducerContext, name string) {
	var score uint64
	player := Player{
		Name:     name,
		Owner:    ctx.Sender(),
		Position: Position{X: 0.0, Y: 0.0},
		Status:   StatusOnline,
		Score:    &score,
	}
	PlayerTable.Insert(player)
}

//stdb:reducer
func UpdateScore(ctx reducer.ReducerContext, playerId uint64, delta int64) {
	player, found, err := PlayerTable.FindById(playerId)
	if err != nil || !found {
		return
	}

	var newScore uint64
	if player.Score != nil {
		if delta >= 0 {
			newScore = *player.Score + uint64(delta)
		} else {
			abs := uint64(-delta)
			if abs > *player.Score {
				newScore = 0
			} else {
				newScore = *player.Score - abs
			}
		}
	} else {
		if delta >= 0 {
			newScore = uint64(delta)
		} else {
			newScore = 0
		}
	}

	player.Score = &newScore
	PlayerTable.UpdateById(player)
}

//stdb:reducer
func RemovePlayer(ctx reducer.ReducerContext, playerId uint64) error {
	_, found, err := PlayerTable.FindById(playerId)
	if err != nil {
		return fmt.Errorf("looking up player: %w", err)
	}
	if !found {
		return fmt.Errorf("player %d not found", playerId)
	}

	PlayerTable.DeleteById(playerId)
	return nil
}
