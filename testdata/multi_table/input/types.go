package main

//stdb:table name=active_entity access=public
//stdb:table name=inactive_entity access=private
type Entity struct {
	Id   uint64 `stdb:"primarykey"`
	Name string
}
