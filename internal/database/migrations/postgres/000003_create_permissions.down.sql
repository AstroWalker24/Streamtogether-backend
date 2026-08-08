-- =============================================================================
-- Migration: 000003_create_permissions.down
-- Purpose:   Reverses 000003_create_permissions.up.
--            Removes the permissions table.
-- Dependencies:
--   role_permissions must be dropped first (migration 000005).
--   golang-migrate runs downs in reverse order automatically.
-- WARNING:   All permission data will be permanently lost.
-- =============================================================================

DROP TABLE IF EXISTS permissions;