package db

import (
	"database/sql"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func Open(databaseURL string) *bun.DB {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(databaseURL)))
	bundb := bun.NewDB(sqldb, pgdialect.New())
	if err := bundb.Ping(); err != nil {
		panic("connect to postgres: " + err.Error())
	}
	return bundb
}
