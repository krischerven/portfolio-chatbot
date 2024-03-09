package main

import (
	"github.com/jackc/pgx/v5"
)

func finishRows(rows pgx.Rows) {
	rows.Close()
	fail(rows.Err())
}
