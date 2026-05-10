package db

import (
	"database/sql"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func Open(databaseURL string) (*bun.DB, error) {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(databaseURL)))
	bundb := bun.NewDB(sqldb, pgdialect.New())
	if err := bundb.Ping(); err != nil {
		bundb.Close()
		return nil, err
	}
	return bundb, nil
}
