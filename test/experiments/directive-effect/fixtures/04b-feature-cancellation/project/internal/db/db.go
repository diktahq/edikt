// Package db wraps the raw database connection used by the repository layer.
// Nothing outside internal/repository or internal/audit should depend on it.
package db

import "database/sql"

// Open returns a connected *sql.DB.
func Open(dsn string) (*sql.DB, error) {
	if dsn == "" {
		dsn = "postgres://localhost/orders?sslmode=disable"
	}
	return sql.Open("pgx", dsn)
}
