-- =============================================================================
-- Migration: 000013_create_oauth_identities.up
-- Purpose:   Creates the oauth_identities table, which links an external
--            OAuth/OIDC provider identity (provider + provider_user_id) to a
--            local user account. Supports §12.1 Social Login.
-- Dependencies:
--   000001_create_users — users table + fn_set_updated_at must exist.
-- Objects created:
--   TABLE   oauth_identities
--   INDEX   idx_oauth_identities_user_id
--   TRIGGER trg_oauth_identities_updated_at
-- Rollback: 000013_create_oauth_identities.down
-- =============================================================================
CREATE TABLE oauth_identities (
    id               UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id          UUID        NOT NULL,
    provider         TEXT        NOT NULL,
    provider_user_id TEXT        NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_oauth_identities              PRIMARY KEY (id),
    CONSTRAINT uq_oauth_provider_user           UNIQUE (provider, provider_user_id),
    CONSTRAINT fk_oauth_identity_user           FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_oauth_provider_not_empty      CHECK (provider <> ''),
    CONSTRAINT ck_oauth_provider_user_not_empty CHECK (provider_user_id <> '')
);

CREATE INDEX idx_oauth_identities_user_id
    ON oauth_identities (user_id);

CREATE TRIGGER trg_oauth_identities_updated_at
    BEFORE UPDATE ON oauth_identities
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
