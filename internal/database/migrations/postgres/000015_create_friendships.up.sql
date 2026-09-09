-- =============================================================================
-- Migration: 000015_create_friendships.up
-- Purpose:   Creates the friendships table for the Friends domain (Phase 3.2).
--            Stores one immutable, canonical row for each established mutual
--            friendship between two users.
-- Dependencies:
--   000001_create_users — users table and pgcrypto/gen_random_uuid() must exist.
-- Objects created:
--   TABLE   friendships
--   INDEX   idx_friendships_first_user_created_at_id
--   INDEX   idx_friendships_second_user_created_at_id
-- Rollback: 000015_create_friendships.down reverses these objects.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Table: friendships
-- ---------------------------------------------------------------------------
-- One canonical row represents a mutual friendship. The ordered endpoints and
-- unique pair prevent duplicate or directional representations of a friendship.
-- Friendship lifecycle is existence-based: removal hard-deletes the row.
-- ---------------------------------------------------------------------------
CREATE TABLE friendships (
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    first_user_id   UUID        NOT NULL,
    second_user_id  UUID        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_friendships
        PRIMARY KEY (id),

    CONSTRAINT fk_friendships_first_user_id
        FOREIGN KEY (first_user_id)
        REFERENCES users (id)
        ON DELETE CASCADE,
    CONSTRAINT fk_friendships_second_user_id
        FOREIGN KEY (second_user_id)
        REFERENCES users (id)
        ON DELETE CASCADE,

    CONSTRAINT ck_friendships_distinct_users
        CHECK (first_user_id <> second_user_id),
    CONSTRAINT ck_friendships_canonical_user_order
        CHECK (first_user_id < second_user_id),

    CONSTRAINT uq_friendships_user_pair
        UNIQUE (first_user_id, second_user_id)
);

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------
-- Each endpoint requires its own ordered index because a user can appear in
-- either position of the canonical pair. These support friend-list pagination
-- ordered by created_at DESC, id DESC.
-- ---------------------------------------------------------------------------
CREATE INDEX idx_friendships_first_user_created_at_id
    ON friendships (first_user_id, created_at DESC, id DESC);

CREATE INDEX idx_friendships_second_user_created_at_id
    ON friendships (second_user_id, created_at DESC, id DESC);