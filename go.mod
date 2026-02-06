module github.com/i5heu/ouroboros-db

go 1.24.6

require github.com/i5heu/ouroboros-crypt v1.1.2

require (
	github.com/cloudflare/circl v1.6.0 // indirect
	golang.org/x/sys v0.10.0 // indirect
	pgregory.net/rapid v1.2.0
)

replace pgregory.net/rapid => github.com/flyingmutant/rapid v1.2.0
