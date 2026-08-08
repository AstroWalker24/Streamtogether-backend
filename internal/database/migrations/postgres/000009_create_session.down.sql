-- =============================================================================
-- Migration: 000009_create_sessions.down
-- Purpose:   Reverses 000009_create_sessions.up.
--            Drops the sessions table and its indexes.
-- Note:      Indexes are dropped automatically with the table.
-- WARNING:   All session records will be permanently lost.
--            refresh_tokens references sessions with ON DELETE CASCADE, so
--            if 000010_create_refresh_tokens has been run, golang-migrate
--            will run its down first, removing refresh_tokens before this
--            migration drops sessions.
-- =============================================================================

DROP TABLE IF EXISTS sessions;