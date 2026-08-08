-- =============================================================================
-- Migration: 000004_create_user_roles.up
-- Purpose:   Creates the user_roles join table, which is the many-to-many
--            relationship between users and roles.
-- Dependencies:
--   000001_create_users  — users table must exist.
--   000002_create_roles  — roles table must exist.
-- Objects created:
--   TABLE  user_roles
--   INDEX  idx_user_roles_user_id
-- Rollback: 000004_create_user_roles.down
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Table: user_roles
-- -----------------------------------------------------------------------------
-- Assigns one or more roles to a user. The combination (user_id, role_id)
-- is the natural identity — no surrogate key is needed.
--
-- Cascade behavior:
--   user_id → ON DELETE CASCADE:
--     A permanently deleted user's role assignments are removed with them.
--   role_id → ON DELETE RESTRICT:
--     Prevents deleting a role while users still hold it. The application
--     must explicitly unassign all users before a role can be removed.
--   assigned_by → ON DELETE SET NULL:
--     Preserves the assignment record when the assigning admin is deleted.
--     The assignment history remains auditable.
--
-- assigned_by is NULL for system-assigned roles (e.g., the 'user' role
-- granted automatically at registration).
-- -----------------------------------------------------------------------------
CREATE TABLE user_roles (
    user_id     UUID        NOT NULL,
    role_id     UUID        NOT NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    assigned_by UUID        NULL,

    CONSTRAINT pk_user_roles             PRIMARY KEY (user_id, role_id),
    CONSTRAINT fk_user_roles_user_id     FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_user_roles_role_id     FOREIGN KEY (role_id)
        REFERENCES roles (id) ON DELETE RESTRICT,
    CONSTRAINT fk_user_roles_assigned_by FOREIGN KEY (assigned_by)
        REFERENCES users (id) ON DELETE SET NULL
);

-- -----------------------------------------------------------------------------
-- Index: idx_user_roles_user_id
-- -----------------------------------------------------------------------------
-- Supports the "get all roles for a user" query pattern used in permission
-- resolution and profile responses.
-- The composite PK (user_id, role_id) already supports leading-column lookups
-- on user_id; this index makes the intent explicit and is kept for forward
-- compatibility if the PK strategy ever changes.
-- -----------------------------------------------------------------------------
CREATE INDEX idx_user_roles_user_id
    ON user_roles (user_id);