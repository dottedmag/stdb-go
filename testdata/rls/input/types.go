package main

//stdb:table name=secret access=public
type Secret struct {
	Id    uint64 `stdb:"primarykey"`
	Value string
}

//stdb:rls SELECT * FROM secret WHERE owner = @sender
func RlsFilter() {}
