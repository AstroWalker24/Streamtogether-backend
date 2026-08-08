-- =============================================================================
-- Migration: 000010_create_refresh_tokens.down
-- Purpose:   Reverses 000010_create_refresh_tokens.up.
--            Drops the refresh_tokens table and its indexes.
-- Note:      Indexes are dropped automatically with the table.
--            The self-referencing FK (replaced_by_id) is within this table,
--            so no cross-table dependency ordering is required for the drop.
-- WARNING:   All refresh token records will be permanently lost.
-- =============================================================================

DROP TABLE IF EXISTS refresh_tokens;