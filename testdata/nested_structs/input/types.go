package main

type Vector2 struct {
	X float64
	Y float64
}

type Transform struct {
	Position Vector2
	Rotation float32
	Scale    float32
}

//stdb:table name=entity access=public
type Entity struct {
	Id        uint64 `stdb:"primarykey"`
	Transform Transform
}
