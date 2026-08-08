-- =============================================================================
-- Migration: 000008_create_devices.down
-- Purpose:   Reverses 000008_create_devices.up in dependency order.
--            Removes the devices table, its indexes, and the device_platform
--            enum type introduced by this migration.
-- Note:      Indexes are dropped automatically with the table.
--            The enum is dropped explicitly after the table is gone.
-- WARNING:   All device records will be permanently lost.
--            Sessions and refresh_tokens reference devices with ON DELETE
--            RESTRICT, so those tables must be dropped first if they exist.
--            golang-migrate handles this automatically by running downs in
--            reverse order (sessions and refresh_tokens are later migrations).
-- =============================================================================

DROP TABLE IF EXISTS devices;

DROP TYPE IF EXISTS device_platform;