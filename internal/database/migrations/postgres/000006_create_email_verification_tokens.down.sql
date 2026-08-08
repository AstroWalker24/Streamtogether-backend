-- =============================================================================
-- Migration: 000006_create_email_verification_tokens.down
-- Purpose:   Reverses 000006_create_email_verification_tokens.up.
--            Removes the email_verification_tokens table and its index.
-- Note:      The index is dropped automatically with the table.
-- WARNING:   All email verification token records will be permanently lost.
--            Any user currently in the pending_verification state will be
--            unable to complete verification until new tokens are issued.
-- =============================================================================

DROP TABLE IF EXISTS email_verification_tokens;