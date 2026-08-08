-- =============================================================================
-- Migration: 000012_seed_roles_permissions.up
-- Purpose:   Seeds the four immutable system roles, the initial permission set,
--            and the role-to-permission assignments that constitute the RBAC
--            baseline. All inserts are idempotent via ON CONFLICT guards.
-- Dependencies:
--   000002_create_roles        — roles table must exist.
--   000003_create_permissions  — permissions table must exist.
--   000005_create_role_permissions — role_permissions table must exist.
-- Rollback: 000012_seed_roles_permissions.down
-- =============================================================================

-- ---------------------------------------------------------------------------
-- System roles
-- ---------------------------------------------------------------------------
-- is_system = TRUE marks these rows as undeletable by the application layer.
-- gen_random_uuid() produces stable UUIDs per row per insert; the name UNIQUE
-- constraint ensures re-runs are idempotent via ON CONFLICT DO NOTHING.
-- ---------------------------------------------------------------------------
INSERT INTO roles (id, name, label, description, is_system) VALUES
    (gen_random_uuid(), 'user',
        'User',
        'Default role for all registered users.',
        TRUE),
    (gen_random_uuid(), 'moderator',
        'Moderator',
        'Extends user with content moderation capabilities.',
        TRUE),
    (gen_random_uuid(), 'admin',
        'Admin',
        'Full platform access including user and system management.',
        TRUE),
    (gen_random_uuid(), 'service',
        'Service',
        'Internal machine identity for service-to-service calls.',
        TRUE)
ON CONFLICT (name) DO NOTHING;

-- ---------------------------------------------------------------------------
-- System permissions
-- ---------------------------------------------------------------------------
-- Permissions grow as features are built; additions are new data migrations,
-- not changes to this file. The ck_permissions_name_format CHECK constraint
-- on the table enforces the resource:action convention at the DB level.
-- ---------------------------------------------------------------------------
INSERT INTO permissions (id, name, description, category) VALUES
    -- party
    (gen_random_uuid(), 'party:create',
        'Create a watch party.',
        'party'),
    (gen_random_uuid(), 'party:join',
        'Join a watch party.',
        'party'),
    (gen_random_uuid(), 'party:kick_member',
        'Remove a member from a party.',
        'party'),
    -- chat
    (gen_random_uuid(), 'chat:send',
        'Send messages in a party.',
        'chat'),
    (gen_random_uuid(), 'chat:delete_own',
        'Delete own chat messages.',
        'chat'),
    (gen_random_uuid(), 'chat:delete_any',
        'Delete any chat message (moderation).',
        'chat'),
    -- user self-management
    (gen_random_uuid(), 'user:manage_self',
        'Manage own account settings.',
        'user'),
    -- admin
    (gen_random_uuid(), 'admin:suspend_user',
        'Suspend or reactivate a user account.',
        'admin'),
    (gen_random_uuid(), 'admin:manage_roles',
        'Assign and remove roles from users.',
        'admin'),
    (gen_random_uuid(), 'admin:view_audit_log',
        'View the platform audit log.',
        'admin')
ON CONFLICT (name) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Role-to-permission assignments
-- ---------------------------------------------------------------------------
-- Uses a cross-join with WHERE filters so the INSERT resolves IDs by name
-- rather than hardcoding UUIDs — correct regardless of insert order or
-- gen_random_uuid() output.
--
-- service role intentionally receives no permissions: it is a machine
-- identity authenticated via internal mechanisms, not checked against RBAC.
-- ---------------------------------------------------------------------------
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM   roles r, permissions p
WHERE
    (r.name = 'user' AND p.name IN (
        'party:create',
        'party:join',
        'chat:send',
        'chat:delete_own',
        'user:manage_self'
    ))
    OR
    (r.name = 'moderator' AND p.name IN (
        'party:create',
        'party:join',
        'party:kick_member',
        'chat:send',
        'chat:delete_own',
        'chat:delete_any',
        'user:manage_self'
    ))
    OR
    (r.name = 'admin' AND p.name IN (
        'party:create',
        'party:join',
        'party:kick_member',
        'chat:send',
        'chat:delete_own',
        'chat:delete_any',
        'user:manage_self',
        'admin:suspend_user',
        'admin:manage_roles',
        'admin:view_audit_log'
    ))
ON CONFLICT DO NOTHING;