package main

import (
	"github.com/clockworklabs/SpacetimeDB/sdks/go/server/reducer"
	"github.com/clockworklabs/SpacetimeDB/sdks/go/types"
)

//stdb:table name=scheduled_proc_table access=private
//stdb:schedule table=scheduled_proc_table function=scheduled_proc
type ScheduledProcTable struct {
	ScheduledId uint64 `stdb:"primarykey,autoinc"`
	ScheduledAt types.ScheduleAt
}

//stdb:reducer name=scheduled_proc
func ScheduledProc(ctx reducer.ReducerContext) {
}
