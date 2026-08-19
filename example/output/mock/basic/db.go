package main

import (
	"log"
	"sync/atomic"
)

var version atomic.Uint32

type dbClient struct {
	version uint32
	dsn     string
}

func newDb(dsn string) *dbClient {
	return &dbClient{
		version: version.Add(1),
		dsn:     dsn,
	}
}

func (dbc *dbClient) Version() uint32 {
	return dbc.version
}

func (dbc *dbClient) DSN() string {
	return dbc.dsn
}

func (dbc *dbClient) Close() {
	log.Println("Closing db connection")
}
