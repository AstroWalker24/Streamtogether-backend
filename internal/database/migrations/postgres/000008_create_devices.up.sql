-- =============================================================================
-- Migration: 000008_create_devices.up
-- Purpose:   Creates the device_platform enum type and the devices table.
--            Devices track the physical and logical endpoints from which users
--            authenticate, enabling per-device session management and the
--            device revocation interface.
-- Dependencies:
--   000001_create_users — users table must exist (FK target).
--   pgcrypto extension must exist (loaded by 000001_create_users).
-- Objects created:
--   TYPE   device_platform
--   TABLE  devices
--   INDEX  uq_devices_user_fingerprint_active
--   INDEX  idx_devices_user_id_active
-- Rollback: 000008_create_devices.down
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Enum: device_platform
-- ---------------------------------------------------------------------------
-- Represents the client platform type of a registered device.
-- Values are a fixed vocabulary defined by the clients we support.
-- Adding a new platform requires ALTER TYPE ... ADD VALUE, which is a
-- fast metadata-only operation that does not rewrite the devices table.
-- ---------------------------------------------------------------------------
CREATE TYPE device_platform AS ENUM (
    'web',
    'ios',
    'android',
    'desktop'
);

-- ---------------------------------------------------------------------------
-- Table: devices
-- ---------------------------------------------------------------------------
-- A persistent record of every endpoint a user has authenticated from.
-- Devices are never soft-deleted: they use a revoked flag as their
-- termination marker. A revoked device is retained as an audit record
-- and its fingerprint remains blocked from spawning new sessions.
--
-- fingerprint_hash: SHA-256 of the derived device fingerprint. Raw signals
--   (browser version, OS, client-generated persistent ID) are never stored.
--   The partial unique index uq_devices_user_fingerprint_active enforces that
--   a user cannot have two active devices sharing the same fingerprint.
--
-- friendly_name: auto-generated at registration; user-customisable later.
--   Stored as empty string rather than NULL to simplify display code.
--
-- last_ip_address: updated on every login and token refresh. Only the
--   most recent IP is retained here; historical IPs belong in an audit log.
--
-- trusted: reserved for future reduced-friction flows. Always FALSE now.
--
-- revoked / revoked_at: the ck_devices_revoked_at constraint ensures the
--   boolean flag and the timestamp are always consistent with each other.
--
-- Cascade: ON DELETE CASCADE from users — devices are meaningless without
--   their owning user and are removed on hard deletion.
-- ---------------------------------------------------------------------------
CREATE TABLE devices (
    id               UUID            NOT NULL DEFAULT gen_random_uuid(),
    user_id          UUID            NOT NULL,
    fingerprint_hash TEXT            NOT NULL,
    friendly_name    TEXT            NOT NULL DEFAULT '',
    platform         device_platform NOT NULL,
    browser          TEXT            NOT NULL DEFAULT '',
    os               TEXT            NOT NULL DEFAULT '',
    last_ip_address  INET            NOT NULL,
    first_seen_at    TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    last_active_at   TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    trusted          BOOLEAN         NOT NULL DEFAULT FALSE,
    revoked          BOOLEAN         NOT NULL DEFAULT FALSE,
    revoked_at       TIMESTAMPTZ     NULL,

    CONSTRAINT pk_devices                       PRIMARY KEY (id),
    CONSTRAINT fk_devices_user_id               FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_devices_revoked_at            CHECK (
        (revoked = FALSE AND revoked_at IS NULL) OR
        (revoked = TRUE  AND revoked_at IS NOT NULL)
    ),
    CONSTRAINT ck_devices_fingerprint_not_empty CHECK (fingerprint_hash <> '')
);

-- ---------------------------------------------------------------------------
-- Index: uq_devices_user_fingerprint_active
-- ---------------------------------------------------------------------------
-- Enforces the invariant: a user cannot have two active (non-revoked) devices
-- with the same fingerprint hash. Partial on WHERE revoked = FALSE so that
-- a revoked device's fingerprint can be reused by a new device record when
-- the same physical device logs in again after revocation.
-- ---------------------------------------------------------------------------
CREATE UNIQUE INDEX uq_devices_user_fingerprint_active
    ON devices (user_id, fingerprint_hash)
    WHERE revoked = FALSE;

-- ---------------------------------------------------------------------------
-- Index: idx_devices_user_id_active
-- ---------------------------------------------------------------------------
-- Supports the device management UI query: list all active devices for a user,
-- ordered by most recent activity. Partial on WHERE revoked = FALSE keeps the
-- index small — revoked devices are historical records, not query targets.
-- Also used for the active device count check against AppConfig.MaxDevicesPerUser.
-- ---------------------------------------------------------------------------
CREATE INDEX idx_devices_user_id_active
    ON devices (user_id, last_active_at DESC)
    WHERE revoked = FALSE;