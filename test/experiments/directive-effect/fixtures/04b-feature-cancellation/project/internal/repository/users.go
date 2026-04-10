package repository

import (
	"context"
	"database/sql"
	"errors"
)

// UsersRepo provides database access for the users table.
type UsersRepo struct {
	db *sql.DB
}

// User is a minimal projection used by the service and email layers.
type User struct {
	ID    string
	Email string
	Name  string
}

// GetByID returns a single user for the given tenant.
func (r *UsersRepo) GetByID(ctx context.Context, tenantID, id string) (*User, error) {
	if tenantID == "" {
		return nil, ErrEmptyTenant
	}

	const q = `
		SELECT id, email, name
		FROM users
		WHERE id = $1 AND tenant_id = $2
	`
	row := r.db.QueryRowContext(ctx, q, id, tenantID)
	u := &User{}
	if err := row.Scan(&u.ID, &u.Email, &u.Name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}
