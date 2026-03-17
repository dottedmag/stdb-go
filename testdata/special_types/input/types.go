package main

import "github.com/clockworklabs/SpacetimeDB/sdks/go/types"

//stdb:table name=special access=public
type Special struct {
	Id         uint64 `stdb:"primarykey"`
	Owner      types.Identity
	ConnId     types.ConnectionId
	CreatedAt  types.Timestamp
	Duration   types.TimeDuration
	ExternalId types.Uuid
	BigNum     types.Uint128
	BigSigned  types.Int128
}
