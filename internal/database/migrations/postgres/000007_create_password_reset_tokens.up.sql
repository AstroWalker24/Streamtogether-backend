-- =============================================================================
-- Migration: 000007_create_password_reset_tokens.up
-- Purpose:   Creates the password_reset_tokens table, which holds the
--            server-side record of every account recovery challenge issued
--            via the forgot-password flow.
-- Dependencies:
--   000001_create_users — users table must exist (FK target).
--   pgcrypto extension must exist (loaded by 000001_create_users).
-- Objects created:
--   TABLE  password_reset_tokens
--   INDEX  idx_password_reset_user_id
-- Rollback: 000007_create_password_reset_tokens.down
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Table: password_reset_tokens
-- -----------------------------------------------------------------------------
-- Structurally identical to email_verification_tokens but semantically
-- distinct: these tokens grant the right to set a new password, not to
-- verify an email address. They are kept in a separate table because:
--   1. They have a shorter expiry window (1 hour vs 24 hours).
--   2. Their retention, cleanup, and audit requirements may diverge over time.
--   3. A generic tokens table would require a discriminator column that adds
--      complexity to every query and constraint expression.
--
-- Token storage: SHA-256 hash of the plaintext only. The plaintext is sent
-- to the user's registered email address once and is never stored here.
--
-- Single-active-token invariant: enforced by the service layer, not the DB.
-- The service calls InvalidateAllForUser before issuing a new token.
-- The UNIQUE constraint on token_hash is the database's contribution.
--
-- This token does NOT create a session. It grants only the right to update
-- the password hash. Session creation requires a full login after reset.
--
-- Cascade: ON DELETE CASCADE from users — meaningless without the user.
-- -----------------------------------------------------------------------------
CREATE TABLE password_reset_tokens (
    id         UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL,
    token_hash TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_password_reset_tokens             PRIMARY KEY (id),
    CONSTRAINT uq_password_reset_token_hash         UNIQUE (token_hash),
    CONSTRAINT fk_password_reset_user_id            FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_password_reset_expires            CHECK (expires_at > created_at),
    CONSTRAINT ck_password_reset_hash_not_empty     CHECK (token_hash <> '')
);

-- -----------------------------------------------------------------------------
-- Index: idx_password_reset_user_id
-- -----------------------------------------------------------------------------
-- Supports the "invalidate all active reset tokens for user" write before
-- a new token is issued, and the "find existing unexpired token" check.
-- Partial on WHERE used_at IS NULL for the same reason as the verification
-- token index: consumed tokens are only ever accessed by token_hash.
-- -----------------------------------------------------------------------------
CREATE INDEX idx_password_reset_user_id
    ON password_reset_tokens (user_id)
    WHERE used_at IS NULL;