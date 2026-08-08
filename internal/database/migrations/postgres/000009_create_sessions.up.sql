-- =============================================================================
-- Migration: 000009_create_sessions.up
-- Purpose:   Creates the sessions table.
--            Sessions are the server-side revocation anchor for stateless JWTs.
--            Each session is scoped to one user on one device within a finite
--            time window, and is the target of all token-refresh operations.
-- Dependencies:
--   000001_create_users  — users table must exist (FK target).
--   000008_create_devices — devices table must exist (FK target).
-- Objects created:
--   TABLE  sessions
--   INDEX  idx_sessions_user_id_active
--   INDEX  idx_sessions_device_id
--   INDEX  idx_sessions_expires_at
-- Rollback: 000009_create_sessions.down
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Table: sessions
-- ---------------------------------------------------------------------------
-- ip_address: recorded at session creation time only — never updated during
--   the session lifetime. The device's last_ip_address carries the current IP.
--
-- user_agent: raw user-agent string at session creation; stored for the device
--   management UI. Defaults to empty string rather than NULL because the value
--   is always present in HTTP requests.
--
-- expires_at: immutable after creation. The ck_sessions_expires_future
--   constraint ensures the expiry is strictly in the future relative to
--   created_at, preventing nonsensical records at the DB level.
--
-- last_active_at: the write-hot column — updated on every successful token
--   refresh. Kept separate from created_at so the refresh path touches a
--   dedicated column rather than a misleading generic updated_at.
--
-- remember_me: controls refresh token expiry duration, recorded at creation,
--   never modified. The service reads this flag when issuing refresh tokens.
--
-- revoked / revoked_at: the ck_sessions_revoked_at constraint ensures the
--   boolean flag and the timestamp are always consistent with each other.
--
-- fk_sessions_device_id uses ON DELETE RESTRICT intentionally. Devices must
--   not be hard-deleted while active sessions exist. Any purge operation must
--   explicitly revoke sessions first, making accidental orphaning impossible.
-- ---------------------------------------------------------------------------
CREATE TABLE sessions (
    id             UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id        UUID        NOT NULL,
    device_id      UUID        NOT NULL,
    ip_address     INET        NOT NULL,
    user_agent     TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at     TIMESTAMPTZ NOT NULL,
    revoked        BOOLEAN     NOT NULL DEFAULT FALSE,
    revoked_at     TIMESTAMPTZ NULL,
    remember_me    BOOLEAN     NOT NULL DEFAULT FALSE,

    CONSTRAINT pk_sessions           PRIMARY KEY (id),
    CONSTRAINT fk_sessions_user_id   FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_sessions_device_id FOREIGN KEY (device_id)
        REFERENCES devices (id) ON DELETE RESTRICT,
    CONSTRAINT ck_sessions_expires_future CHECK (expires_at > created_at),
    CONSTRAINT ck_sessions_revoked_at     CHECK (
        (revoked = FALSE AND revoked_at IS NULL) OR
        (revoked = TRUE  AND revoked_at IS NOT NULL)
    )
);

-- ---------------------------------------------------------------------------
-- Index: idx_sessions_user_id_active
-- ---------------------------------------------------------------------------
-- Supports the device management UI query: list all active sessions for a
-- user, ordered by most recent creation. Partial on WHERE revoked = FALSE
-- keeps the index small — historical revoked sessions are excluded entirely.
-- The created_at DESC ordering matches the natural display order.
-- ---------------------------------------------------------------------------
CREATE INDEX idx_sessions_user_id_active
    ON sessions (user_id, created_at DESC)
    WHERE revoked = FALSE;

-- ---------------------------------------------------------------------------
-- Index: idx_sessions_device_id
-- ---------------------------------------------------------------------------
-- Supports the "revoke all sessions for a device" operation in the device
-- revocation flow. Non-partial because revocation queries target all sessions
-- (active and already-revoked) to verify the device's full session history.
-- ---------------------------------------------------------------------------
CREATE INDEX idx_sessions_device_id
    ON sessions (device_id);

-- ---------------------------------------------------------------------------
-- Index: idx_sessions_expires_at
-- ---------------------------------------------------------------------------
-- Used by background cleanup jobs to find and purge expired sessions.
-- Partial on WHERE revoked = FALSE because already-revoked sessions need no
-- expiry-based cleanup action — they have already been handled.
-- ---------------------------------------------------------------------------
CREATE INDEX idx_sessions_expires_at
    ON sessions (expires_at)
    WHERE revoked = FALSE;