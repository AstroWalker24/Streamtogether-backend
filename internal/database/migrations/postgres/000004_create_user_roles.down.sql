-- =============================================================================
-- Migration: 000004_create_user_roles.down
-- Purpose:   Reverses 000004_create_user_roles.up.
--            Removes the user_roles table and its index.
-- Note:      The index is dropped automatically with the table.
-- WARNING:   All role assignment records will be permanently lost.
-- =============================================================================

DROP TABLE IF EXISTS user_roles;