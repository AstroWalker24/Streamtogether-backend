-- =============================================================================
-- Migration: 000012_seed_roles_permissions.down
-- Purpose:   Reverses 000012_seed_roles_permissions.up.
--            Removes seeded RBAC data in reverse dependency order.
--            Does not drop any tables or alter any schema.
-- Order:
--   1. role_permissions first — permission_id FK uses ON DELETE RESTRICT,
--      so permissions cannot be deleted while referenced.
--   2. permissions second.
--   3. roles last — role deletion would cascade role_permissions, but step 1
--      already cleaned them; explicit ordering makes the intent unambiguous.
-- =============================================================================

-- Step 1: remove role-permission assignments for all system roles
DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM roles WHERE is_system = TRUE
);

-- Step 2: remove seeded permissions
DELETE FROM permissions
WHERE name IN (
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
);

-- Step 3: remove system roles
DELETE FROM roles
WHERE name IN ('user', 'moderator', 'admin', 'service')
  AND is_system = TRUE;