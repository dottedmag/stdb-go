package main

//stdb:table name=all_primitives access=public
type AllPrimitives struct {
	Id      uint64  `stdb:"primarykey,autoinc"`
	ABool   bool
	AU8     uint8
	AU16    uint16
	AU32    uint32
	AU64    uint64
	AI8     int8
	AI16    int16
	AI32    int32
	AI64    int64
	AF32    float32
	AF64    float64
	AString string
}
