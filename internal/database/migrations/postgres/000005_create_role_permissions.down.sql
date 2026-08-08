-- =============================================================================
-- Migration: 000005_create_role_permissions.down
-- Purpose:   Reverses 000005_create_role_permissions.up.
--            Removes the role_permissions table and its index.
-- Note:      The index is dropped automatically with the table.
-- WARNING:   All role-to-permission assignments will be permanently lost.
--            Seed data (migration 000017) must be re-applied after re-running up.
-- =============================================================================

DROP TABLE IF EXISTS role_permissions;