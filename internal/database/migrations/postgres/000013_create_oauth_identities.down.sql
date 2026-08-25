-- =============================================================================
-- Migration: 000013_create_oauth_identities.down
-- Rolls back the oauth_identities table and all its supporting objects.
-- =============================================================================
DROP TRIGGER IF EXISTS trg_oauth_identities_updated_at ON oauth_identities;
DROP INDEX   IF EXISTS idx_oauth_identities_user_id;
DROP TABLE   IF EXISTS oauth_identities;
