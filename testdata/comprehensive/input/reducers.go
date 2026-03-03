package main

import "github.com/clockworklabs/SpacetimeDB/sdks/go/server/reducer"

//stdb:init
func Init(ctx reducer.ReducerContext) {
}

//stdb:reducer
func CreatePlayer(ctx reducer.ReducerContext, name string) {
}

//stdb:reducer
func UpdateScore(ctx reducer.ReducerContext, playerId uint64, delta int64) {
}

//stdb:reducer
func RemovePlayer(ctx reducer.ReducerContext, playerId uint64) error {
	return nil
}
