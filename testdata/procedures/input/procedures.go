package main

import "github.com/clockworklabs/SpacetimeDB/sdks/go/server/reducer"

type Player struct {
	Id   uint64
	Name string
}

//stdb:procedure
func GetPlayerCount(ctx reducer.ProcedureContext) uint64 {
	return 0
}

//stdb:procedure
func FindPlayer(ctx reducer.ProcedureContext, name string) *Player {
	return nil
}
