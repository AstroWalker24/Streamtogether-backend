-- =============================================================================
-- Migration: 000006_create_email_verification_tokens.up
-- Purpose:   Creates the email_verification_tokens table, which holds the
--            server-side record of every verification challenge sent to a user's
--            email address during registration and resend flows.
-- Dependencies:
--   000001_create_users — users table must exist (FK target).
--   pgcrypto extension must exist (loaded by 000001_create_users).
-- Objects created:
--   TABLE  email_verification_tokens
--   INDEX  idx_email_verification_user_id
-- Rollback: 000006_create_email_verification_tokens.down
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Table: email_verification_tokens
-- -----------------------------------------------------------------------------
-- Stores only the SHA-256 hash of the plaintext token. The plaintext is
-- transmitted to the user via email exactly once and never persisted.
-- Lookup is always by hash: WHERE token_hash = SHA256($plaintext).
--
-- Single-active-token invariant: at most one valid (unconsumed, unexpired)
-- token should exist per user at any time. This invariant is enforced by the
-- service layer (invalidating previous tokens before inserting a new one).
-- The database cannot express this with a partial unique index cleanly, so
-- the service-layer contract is the enforcement point; the UNIQUE constraint
-- on token_hash ensures no two tokens share the same hash value.
--
-- Expiry window: 24 hours from creation (enforced by the application;
-- the ck_email_verification_expires constraint only guards against
-- nonsensical records where expires_at ≤ created_at).
--
-- Cascade: ON DELETE CASCADE from users — tokens are meaningless without
-- the owning user and are removed with them on hard deletion.
-- -----------------------------------------------------------------------------
CREATE TABLE email_verification_tokens (
    id         UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL,
    token_hash TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_email_verification_tokens           PRIMARY KEY (id),
    CONSTRAINT uq_email_verification_token_hash       UNIQUE (token_hash),
    CONSTRAINT fk_email_verification_user_id          FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_email_verification_expires          CHECK (expires_at > created_at),
    CONSTRAINT ck_email_verification_hash_not_empty   CHECK (token_hash <> '')
);

-- -----------------------------------------------------------------------------
-- Index: idx_email_verification_user_id
-- -----------------------------------------------------------------------------
-- Supports the "invalidate all active tokens for user" write executed by
-- the service before issuing a new token (InvalidateAllForUser).
-- Partial on WHERE used_at IS NULL: consumed tokens are never looked up
-- by user_id again — only by token_hash via the unique constraint above.
-- Keeping the index partial limits its size to unconsumed tokens only.
-- -----------------------------------------------------------------------------
CREATE INDEX idx_email_verification_user_id
    ON email_verification_tokens (user_id)
    WHERE used_at IS NULL;