package main

import "github.com/clockworklabs/SpacetimeDB/sdks/go/server/reducer"

//stdb:reducer
func AddPlayer(ctx reducer.ReducerContext, name string, score uint64) {
}

//stdb:reducer name=custom_remove
func RemovePlayer(ctx reducer.ReducerContext, id uint64) error {
	return nil
}

//stdb:reducer
func ResetAll(ctx reducer.ReducerContext) {
}
