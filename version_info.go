package main

// Build-time variables, set via ldflags:
//
//	-X main.pkgName=stdb-gen -X main.version=v0.1.0 -X main.commit=abc1234
var (
	pkgName = "stdb-gen"
	version = "v0.0.0"
	commit  = "local"
)
