-- =============================================================================
-- Migration: 000014_create_profiles.down
-- Purpose:   Reverses 000014_create_profiles.up in dependency order.
-- Objects dropped:
--   TRIGGER trg_profiles_updated_at  (on profiles)
--   TABLE   profiles                 (also drops the uq/fk/ck constraints)
-- Note: fn_set_updated_at() is shared and must NOT be dropped here.
-- =============================================================================
DROP TRIGGER IF EXISTS trg_profiles_updated_at ON profiles;
DROP TABLE   IF EXISTS profiles;
