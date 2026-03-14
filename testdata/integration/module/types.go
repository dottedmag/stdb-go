package main

import "go.digitalxero.dev/spacetimedb-client/types"

//stdb:enum variants=Online,Offline,Away
type Status uint8

const (
	StatusOnline  Status = 0
	StatusOffline Status = 1
	StatusAway    Status = 2
)

type Position struct {
	X float64
	Y float64
}

//stdb:table name=player access=public
type Player struct {
	Id       uint64         `stdb:"primarykey,autoinc"`
	Name     string         `stdb:"unique"`
	Owner    types.Identity
	Position Position
	Status   Status
	Score    *uint64
}

//stdb:table name=game_log access=public event=true
type GameLog struct {
	Message string
	Level   uint8
}
