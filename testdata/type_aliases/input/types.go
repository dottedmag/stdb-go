package main

import "github.com/clockworklabs/SpacetimeDB/sdks/go/server/reducer"

type Score struct {
	Value uint64
	Label string
}

type ScoreAlias = Score

//stdb:table name=scores access=public
type ScoreTable struct {
	Id uint64 `stdb:"primarykey"`
}

//stdb:reducer
func AddScore(ctx reducer.ReducerContext, score ScoreAlias) {
}
