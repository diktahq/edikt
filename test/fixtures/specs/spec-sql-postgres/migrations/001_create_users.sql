-- Design blueprint — implement in your stack's native format. This artifact defines intent, not implementation.
-- edikt:artifact type=migration spec=SPEC-TEST status=draft reviewed_by=dba
-- created_at=2026-03-27T00:00:00Z
-- migration: 001_create_users
-- description: Create users table with unique email constraint

-- === UP ===
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_users_email UNIQUE (email)
);

-- === DOWN ===
DROP TABLE IF EXISTS users;

-- === BACKFILL ===
-- none required

-- === RISK ===
-- Lock duration: minimal (new table, no existing rows)
-- Data volume: 0 rows affected
-- Deployment notes: safe for zero-downtime deploy
