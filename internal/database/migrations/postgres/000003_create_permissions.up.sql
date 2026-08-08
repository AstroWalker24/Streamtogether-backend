-- =============================================================================
-- Migration: 000003_create_permissions.up
-- Purpose:   Creates the permissions table — the atomic capability units
--            of the RBAC model.
-- Dependencies:
--   000001_create_users — pgcrypto extension must exist.
-- Objects created:
--   TABLE  permissions
-- Rollback: 000003_create_permissions.down
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Table: permissions
-- -----------------------------------------------------------------------------
-- Single, granular capabilities following the resource:action naming convention
-- (e.g., 'party:create', 'chat:delete_any').
-- Permissions are defined at design time and seeded once. They are never
-- renamed or removed — new permissions are added as features grow.
-- The ck_permissions_name_format constraint enforces the resource:action
-- convention at the database level, making violations fail at INSERT time.
-- No updated_at: permission names are immutable and seeded data is static.
-- -----------------------------------------------------------------------------
CREATE TABLE permissions (
    id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    category    TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_permissions            PRIMARY KEY (id),
    CONSTRAINT uq_permissions_name       UNIQUE (name),
    CONSTRAINT ck_permissions_name_format CHECK (name ~ '^[a-z_]+:[a-z_]+$')
);