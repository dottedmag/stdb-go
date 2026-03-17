package main

//stdb:table name=optional_data access=public
type OptionalData struct {
	Id      uint64 `stdb:"primarykey"`
	OptName *string
	Tags    []string
	Scores  []int32
	RawData []byte
}
