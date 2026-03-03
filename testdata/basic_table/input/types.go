package main

//stdb:table name=person access=public
type Person struct {
	Id   uint64 `stdb:"primarykey,autoinc"`
	Name string
}
