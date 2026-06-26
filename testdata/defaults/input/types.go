package main

//stdb:enum variants=Online,Offline,Away
type Status uint8

const (
	StatusOnline  Status = 0
	StatusOffline Status = 1
	StatusAway    Status = 2
)

type Point struct {
	X float64
	Y float64
}

//stdb:table name=widget access=public
type Widget struct {
	Id     uint64  `stdb:"primarykey,autoinc"`
	Count  uint32  `stdb:"default=5"`
	Score  int64   `stdb:"default=-3"`
	Ratio  float64 `stdb:"default=1.5"`
	Active bool    `stdb:"default=true"`
	Name   string  `stdb:"index=btree,default='Unknown'"`
	Note   string  `stdb:"default=''"`
	State  Status  `stdb:"default=Offline"`
	Maybe  *uint64 `stdb:"default=null"`
	Some   *uint32 `stdb:"default=7"`
	Blob   []byte  `stdb:"default=0x01020304"`
	Origin Point   `stdb:"default=raw:0x00000000000000000000000000000000"`
}
