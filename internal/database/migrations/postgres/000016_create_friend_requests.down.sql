-- =============================================================================
-- Migration: 000016_create_friend_requests.down
-- Purpose:   Reverses 000016_create_friend_requests.up in dependency order.
-- Objects dropped:
--   TRIGGER trg_friend_requests_updated_at (on friend_requests)
--   TABLE   friend_requests (also drops Friend Requests-owned indexes and
--                            constraints)
-- Note: fn_set_updated_at() is shared and must NOT be dropped here.
-- =============================================================================
DROP TRIGGER IF EXISTS trg_friend_requests_updated_at ON friend_requests;
DROP TABLE   IF EXISTS friend_requests;