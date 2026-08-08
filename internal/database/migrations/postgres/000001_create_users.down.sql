-- =============================================================================
-- Migration: 000001_create_users.down
-- Purpose:   Reverses 000001_create_users.up in strict dependency order.
--            Removes all objects created by the up migration.
-- Rollback behavior:
--   - Indexes are dropped automatically with their table (CASCADE).
--   - The trigger is dropped automatically with its table (CASCADE).
--   - The trigger function, enum type, and extension are dropped explicitly.
-- WARNING:   Running this migration permanently destroys all user data.
-- =============================================================================

-- Indexes are owned by the table and are dropped implicitly via DROP TABLE CASCADE.
-- Listed here for documentation; no explicit DROP INDEX is needed.

-- Drop table (also drops the trigger and all associated indexes)
DROP TABLE IF EXISTS users;

-- Drop trigger function (no longer referenced after table is gone)
DROP FUNCTION IF EXISTS fn_set_updated_at();

-- Drop enum type (no longer referenced after table is gone)
DROP TYPE IF EXISTS user_status;

-- Drop extension
DROP EXTENSION IF EXISTS pgcrypto;