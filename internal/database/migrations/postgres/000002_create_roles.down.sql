-- =============================================================================
-- Migration: 000002_create_roles.down
-- Purpose:   Reverses 000002_create_roles.up.
--            Removes the roles table.
-- Dependencies:
--   user_roles and role_permissions must be dropped first (migrations 000004
--   and 000005). golang-migrate runs downs in reverse order automatically.
-- WARNING:   All role data will be permanently lost.
-- =============================================================================

DROP TABLE IF EXISTS roles;