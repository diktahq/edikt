// Package repository is the database access layer. Every method takes
// a tenantID as an explicit parameter — the repository does not read
// context. Tenant scoping is a service-layer responsibility; the
// repository is dumb SQL.
package repository

import "database/sql"

// Repos bundles the per-table repositories.
type Repos struct {
	Orders *OrdersRepo
	Users  *UsersRepo
}

// New constructs the repository bundle.
func New(db *sql.DB) *Repos {
	return &Repos{
		Orders: &OrdersRepo{db: db},
		Users:  &UsersRepo{db: db},
	}
}
