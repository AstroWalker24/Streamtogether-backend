-- =============================================================================
-- Migration: 000010_create_refresh_tokens.up
-- Purpose:   Creates the refresh_tokens table.
--            Provides the server-side record for every refresh token issued,
--            enabling validation, rotation, replay detection, and revocation.
-- Dependencies:
--   000001_create_users   — users table must exist (FK target).
--   000008_create_devices — devices table must exist (FK target).
--   000009_create_sessions — sessions table must exist (FK target).
-- Objects created:
--   TABLE  refresh_tokens
--   INDEX  idx_refresh_tokens_session_id
--   INDEX  idx_refresh_tokens_user_id
--   INDEX  idx_refresh_tokens_expires_consumed
-- Rollback: 000010_create_refresh_tokens.down
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Table: refresh_tokens
-- ---------------------------------------------------------------------------
-- token_hash: SHA-256 hex digest of the plaintext token. Never store the raw
--   token value. The UNIQUE constraint also creates the B-tree index used by
--   the refresh hot path — no separate index is needed.
--
-- user_id, device_id: intentionally denormalized from sessions. This removes
--   a join from the two high-urgency write operations:
--     1. "Revoke all tokens for user" — triggered by password change / logout-all.
--     2. "Revoke all tokens for device" — triggered by device revocation.
--
-- issued_at: serves as created_at for this table. Adding a separate
--   created_at would be redundant.
--
-- consumed_at: non-NULL signals the token has been used in a rotation.
--   A presented token with consumed_at IS NOT NULL indicates a replay
--   attack — the service must revoke the entire session immediately.
--
-- replaced_by_id: self-referencing FK forming the rotation chain.
--   ON DELETE SET NULL: if a successor token is purged by a cleanup job
--   before its predecessor, the chain breaks gracefully rather than
--   cascading a deletion through the predecessor.
--
-- revoked / revoked_at: the ck_refresh_tokens_revoked_at constraint ensures
--   the boolean flag and the timestamp are always consistent with each other.
--
-- fk_refresh_tokens_session_id uses ON DELETE CASCADE — refresh tokens are
--   meaningless without their session and must be removed with it.
--
-- fk_refresh_tokens_device_id uses ON DELETE RESTRICT — mirrors the sessions
--   table policy: devices must not be hard-deleted while tokens exist.
--
-- No updated_at: this table is append-heavy with only three mutable columns
--   (consumed_at, revoked, revoked_at). A generic updated_at adds noise.
-- ---------------------------------------------------------------------------
CREATE TABLE refresh_tokens (
    id             UUID        NOT NULL DEFAULT gen_random_uuid(),
    session_id     UUID        NOT NULL,
    user_id        UUID        NOT NULL,
    device_id      UUID        NOT NULL,
    token_hash     TEXT        NOT NULL,
    issued_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at     TIMESTAMPTZ NOT NULL,
    consumed_at    TIMESTAMPTZ NULL,
    revoked        BOOLEAN     NOT NULL DEFAULT FALSE,
    revoked_at     TIMESTAMPTZ NULL,
    replaced_by_id UUID        NULL,

    CONSTRAINT pk_refresh_tokens             PRIMARY KEY (id),
    CONSTRAINT uq_refresh_tokens_token_hash  UNIQUE (token_hash),
    CONSTRAINT fk_refresh_tokens_session_id  FOREIGN KEY (session_id)
        REFERENCES sessions (id) ON DELETE CASCADE,
    CONSTRAINT fk_refresh_tokens_user_id     FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_refresh_tokens_device_id   FOREIGN KEY (device_id)
        REFERENCES devices (id) ON DELETE RESTRICT,
    CONSTRAINT fk_refresh_tokens_replaced_by FOREIGN KEY (replaced_by_id)
        REFERENCES refresh_tokens (id) ON DELETE SET NULL,
    CONSTRAINT ck_refresh_tokens_expires_future CHECK (expires_at > issued_at),
    CONSTRAINT ck_refresh_tokens_revoked_at     CHECK (
        (revoked = FALSE AND revoked_at IS NULL) OR
        (revoked = TRUE  AND revoked_at IS NOT NULL)
    ),
    CONSTRAINT ck_refresh_tokens_hash_not_empty CHECK (token_hash <> '')
);

-- ---------------------------------------------------------------------------
-- Index: idx_refresh_tokens_session_id
-- ---------------------------------------------------------------------------
-- Supports "revoke all tokens for a session" — the logout single-session
-- flow. Non-partial: revocation queries must find all tokens for the session,
-- including already-consumed ones (to detect and log replay chains).
-- ---------------------------------------------------------------------------
CREATE INDEX idx_refresh_tokens_session_id
    ON refresh_tokens (session_id);

-- ---------------------------------------------------------------------------
-- Index: idx_refresh_tokens_user_id
-- ---------------------------------------------------------------------------
-- Supports the high-urgency "revoke all tokens for user" write path,
-- triggered by password change and logout-all. A full table scan here
-- would be unacceptable — this is a write-path index, not a read-path index.
-- ---------------------------------------------------------------------------
CREATE INDEX idx_refresh_tokens_user_id
    ON refresh_tokens (user_id);

-- ---------------------------------------------------------------------------
-- Index: idx_refresh_tokens_expires_consumed
-- ---------------------------------------------------------------------------
-- Used by background cleanup jobs to find expired or consumed tokens eligible
-- for purging. Partial on the condition that makes tokens candidates for
-- deletion, keeping the index small and the scan targeted.
-- ---------------------------------------------------------------------------
CREATE INDEX idx_refresh_tokens_expires_consumed
    ON refresh_tokens (expires_at)
    WHERE consumed_at IS NOT NULL OR revoked = TRUE;