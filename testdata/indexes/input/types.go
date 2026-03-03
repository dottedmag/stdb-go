package main

//stdb:table name=indexed access=public index=multi_idx:0,1
type Indexed struct {
	Id     uint64 `stdb:"primarykey"`
	Name   string `stdb:"unique"`
	Age    uint32 `stdb:"index=btree"`
	Status uint8  `stdb:"index=direct"`
}
