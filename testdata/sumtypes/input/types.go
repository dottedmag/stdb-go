package main

//stdb:sumtype
type Shape interface{ isShape() }

//stdb:variant of=Shape name=Circle
type ShapeCircle struct {
	Radius float64
}

//stdb:variant of=Shape name=Rectangle
type ShapeRectangle struct {
	Width  float64
	Height float64
}

//stdb:variant of=Shape name=Point
type ShapePoint struct{}
