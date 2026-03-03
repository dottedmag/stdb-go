package main

import "github.com/clockworklabs/SpacetimeDB/sdks/go/server/reducer"

type Player struct {
	Id   uint64
	Name string
}

//stdb:view
func GetPlayers(ctx reducer.ViewContext) []Player {
	return nil
}

//stdb:view
func GetPlayerByName(ctx reducer.AnonymousViewContext, name string) *Player {
	return nil
}
