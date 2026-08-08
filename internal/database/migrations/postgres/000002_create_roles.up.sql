-- =============================================================================
-- Migration: 000002_create_roles.up
-- Purpose:   Creates the roles table, which is the primary unit of
--            authorization assignment in the RBAC model.
-- Dependencies:
--   000001_create_users — pgcrypto extension must exist for gen_random_uuid().
-- Objects created:
--   TABLE  roles
-- Rollback: 000002_create_roles.down
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Table: roles
-- -----------------------------------------------------------------------------
-- Named, stable collections of permissions assigned to users.
-- System roles (is_system = TRUE) are seeded at deployment and must never
-- be deleted. The application layer enforces this; the schema permits it
-- by design so that role records can be managed without DDL changes.
-- No soft delete: roles are either permanent (system) or hard-deleted (custom).
-- No updated_at: role names are immutable after creation.
-- -----------------------------------------------------------------------------
CREATE TABLE roles (
    id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    label       TEXT        NOT NULL DEFAULT '',
    description TEXT        NOT NULL DEFAULT '',
    is_system   BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_roles             PRIMARY KEY (id),
    CONSTRAINT uq_roles_name        UNIQUE (name),
    CONSTRAINT ck_roles_name_not_empty CHECK (name <> '')
);