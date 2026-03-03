package main

//stdb:table name=game_event access=public event=true
type GameEvent struct {
	EventType uint8
	Data      string
}
