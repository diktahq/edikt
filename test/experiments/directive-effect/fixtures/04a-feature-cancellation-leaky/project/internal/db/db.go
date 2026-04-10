// Package db wraps the database connection used by the repository layer.
// Nothing outside internal/repository should depend on this package directly.
package db

import (
	"database/sql"
)

// Open returns a connected *sql.DB. The driver registration is omitted from
// this fixture; in the real service we use github.com/jackc/pgx/v5/stdlib.
func Open(dsn string) (*sql.DB, error) {
	if dsn == "" {
		dsn = "postgres://localhost/orders?sslmode=disable"
	}
	return sql.Open("pgx", dsn)
}
