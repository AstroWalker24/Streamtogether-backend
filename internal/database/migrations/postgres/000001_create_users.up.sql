-- =============================================================================
-- Migration: 000001_create_users.up
-- Purpose:   Creates the foundational infrastructure shared by all auth
--            migrations (extension, enum, trigger function) and the users table,
--            which is the root aggregate of the Authentication domain.
-- Dependencies: None. This is the first migration in the auth domain.
-- Objects created:
--   EXTENSION  pgcrypto
--   TYPE       user_status
--   FUNCTION   fn_set_updated_at()
--   TABLE      users
--   TRIGGER    trg_users_updated_at
--   INDEX      uq_users_email_lower
--   INDEX      uq_users_username_lower
--   INDEX      idx_users_status
--   INDEX      idx_users_deleted_at
-- Rollback:   000001_create_users.down reverses all objects in dependency order.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Extension
-- ---------------------------------------------------------------------------
-- gen_random_uuid() is built-in on PostgreSQL 13+.
-- Enabling pgcrypto defensively ensures compatibility with earlier versions
-- without any cost on 13+.
-- ---------------------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Enum: user_status
-- ---------------------------------------------------------------------------
-- Stored as a 4-byte integer internally. Prevents invalid states at the
-- database level. Adding new values in the future requires only:
--   ALTER TYPE user_status ADD VALUE 'new_state';
-- which is a fast metadata-only operation.
-- ---------------------------------------------------------------------------
CREATE TYPE user_status AS ENUM (
    'pending_verification',
    'active',
    'suspended',
    'deleted'
);

-- ---------------------------------------------------------------------------
-- Trigger function: fn_set_updated_at
-- ---------------------------------------------------------------------------
-- Shared by any table that carries an updated_at column.
-- Defined once here; attached per-table via CREATE TRIGGER below.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION fn_set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

-- ---------------------------------------------------------------------------
-- Table: users
-- ---------------------------------------------------------------------------
-- Root identity aggregate. Every session, device, token, and role assignment
-- in the Authentication domain traces back to a row in this table.
-- Soft-deleted via deleted_at + status = 'deleted' (30-day grace period).
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    -- Identity
    id              UUID          NOT NULL DEFAULT gen_random_uuid(),

    -- Business fields
    email           TEXT          NOT NULL,
    username        TEXT          NOT NULL,

    -- Credential (Argon2id hash only — plaintext is never stored)
    password_hash   TEXT          NOT NULL,

    -- Lifecycle state
    status          user_status   NOT NULL DEFAULT 'pending_verification',
    email_verified  BOOLEAN       NOT NULL DEFAULT FALSE,

    -- Audit columns
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),

    -- Soft delete marker
    deleted_at      TIMESTAMPTZ   NULL,

    -- Primary key
    CONSTRAINT pk_users PRIMARY KEY (id),

    -- Uniqueness invariants (case-sensitive at column level;
    -- case-insensitive uniqueness is enforced by the functional indexes below)
    CONSTRAINT uq_users_email    UNIQUE (email),
    CONSTRAINT uq_users_username UNIQUE (username),

    -- Guard against application bugs sending empty strings
    CONSTRAINT ck_users_email_not_empty    CHECK (email          <> ''),
    CONSTRAINT ck_users_username_not_empty CHECK (username       <> ''),
    CONSTRAINT ck_users_password_not_empty CHECK (password_hash  <> ''),

    -- deleted_at and status must remain in sync
    CONSTRAINT ck_users_deleted_at_status  CHECK (
        deleted_at IS NULL OR status = 'deleted'
    )
);

-- ---------------------------------------------------------------------------
-- Trigger: trg_users_updated_at
-- ---------------------------------------------------------------------------
CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION fn_set_updated_at();

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------

-- Case-insensitive unique email lookup (login by email, registration duplicate check).
-- The column-level UNIQUE constraint is case-sensitive; this functional index is
-- the actual enforcement of the case-insensitive uniqueness invariant.
CREATE UNIQUE INDEX uq_users_email_lower
    ON users (lower(email));

-- Case-insensitive unique username lookup (login by username, registration check).
CREATE UNIQUE INDEX uq_users_username_lower
    ON users (lower(username));

-- Filter by account status (admin queries, suspension checks, cleanup jobs).
CREATE INDEX idx_users_status
    ON users (status);

-- Locate soft-deleted users whose grace period has elapsed (background cleanup job).
-- Partial: excludes the overwhelming majority of active users from the index.
CREATE INDEX idx_users_deleted_at
    ON users (deleted_at)
    WHERE deleted_at IS NOT NULL;