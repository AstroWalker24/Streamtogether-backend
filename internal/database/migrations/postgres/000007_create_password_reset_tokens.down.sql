-- =============================================================================
-- Migration: 000007_create_password_reset_tokens.down
-- Purpose:   Reverses 000007_create_password_reset_tokens.up.
--            Removes the password_reset_tokens table and its index.
-- Note:      The index is dropped automatically with the table.
-- WARNING:   All password reset token records will be permanently lost.
--            Any user currently mid-way through a password reset flow will
--            need to restart the process after re-running the up migration.
-- =============================================================================

DROP TABLE IF EXISTS password_reset_tokens;