-- =============================================================================
-- Migration: 000015_create_friendships.down
-- Purpose:   Reverses 000015_create_friendships.up in dependency order.
-- Objects dropped:
--   TABLE friendships (also drops Friends-owned indexes and constraints)
-- =============================================================================
DROP TABLE IF EXISTS friendships;