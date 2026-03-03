package main

//stdb:enum variants=Red,Green,Blue
type Color uint8

const (
	ColorRed   Color = 0
	ColorGreen Color = 1
	ColorBlue  Color = 2
)

//stdb:enum variants=Up,Down,Left,Right scope=Game
type Direction uint8

const (
	DirectionUp    Direction = 0
	DirectionDown  Direction = 1
	DirectionLeft  Direction = 2
	DirectionRight Direction = 3
)
