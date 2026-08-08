-- =============================================================================
-- Migration: 000005_create_role_permissions.up
-- Purpose:   Creates the role_permissions join table, which defines the
--            capability set of each role.
-- Dependencies:
--   000002_create_roles       — roles table must exist.
--   000003_create_permissions — permissions table must exist.
-- Objects created:
--   TABLE  role_permissions
--   INDEX  idx_role_permissions_role_id
-- Rollback: 000005_create_role_permissions.down
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Table: role_permissions
-- -----------------------------------------------------------------------------
-- Assigns permissions to roles. The combination (role_id, permission_id)
-- is the natural identity — no surrogate key is needed.
--
-- Cascade behavior:
--   role_id → ON DELETE CASCADE:
--     If a role is deleted, its permission assignments are removed with it.
--     This is safe because permissions themselves are not affected.
--   permission_id → ON DELETE RESTRICT:
--     Prevents deleting a permission while it is assigned to any role.
--     Permissions are considered immutable by design (they are never
--     renamed or removed). This constraint enforces that invariant at the
--     database level.
--
-- assigned_at enables auditing of when a capability was added to a role,
-- which is useful for security reviews and incident reconstruction.
-- -----------------------------------------------------------------------------
CREATE TABLE role_permissions (
    role_id       UUID        NOT NULL,
    permission_id UUID        NOT NULL,
    assigned_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_role_permissions               PRIMARY KEY (role_id, permission_id),
    CONSTRAINT fk_role_permissions_role_id       FOREIGN KEY (role_id)
        REFERENCES roles (id) ON DELETE CASCADE,
    CONSTRAINT fk_role_permissions_permission_id FOREIGN KEY (permission_id)
        REFERENCES permissions (id) ON DELETE RESTRICT
);

-- -----------------------------------------------------------------------------
-- Index: idx_role_permissions_role_id
-- -----------------------------------------------------------------------------
-- Supports the "get all permissions for a role" query pattern used during
-- permission resolution (PermissionService.GetUserPermissions joins through
-- user_roles → role_permissions → permissions).
-- The composite PK (role_id, permission_id) covers this pattern; the index
-- is kept for the same forward-compatibility reason as idx_user_roles_user_id.
-- -----------------------------------------------------------------------------
CREATE INDEX idx_role_permissions_role_id
    ON role_permissions (role_id);