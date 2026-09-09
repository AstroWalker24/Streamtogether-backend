-- =============================================================================
-- Migration: 000016_create_friend_requests.up
-- Purpose:   Creates the friend_requests table for the Friend Requests domain
--            (Phase 3.3). Stores directed requests and their lifecycle state;
--            it does not create or duplicate established friendships.
-- Dependencies:
--   000001_create_users       — users table, pgcrypto/gen_random_uuid(), and
--                                fn_set_updated_at() must exist.
--   000015_create_friendships — establishes the preceding Social Graph schema.
-- Objects created:
--   TABLE   friend_requests
--   INDEX   uq_friend_requests_pending_direction
--   INDEX   idx_friend_requests_incoming_pending_created_at_id
--   INDEX   idx_friend_requests_outgoing_pending_created_at_id
--   TRIGGER trg_friend_requests_updated_at
-- Rollback: 000016_create_friend_requests.down reverses these objects.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Table: friend_requests
-- ---------------------------------------------------------------------------
-- One directed row represents a request from requester_user_id to
-- recipient_user_id. Terminal request history is retained, while the partial
-- unique index below permits at most one pending request in that direction.
-- ---------------------------------------------------------------------------
CREATE TABLE friend_requests (
    id                UUID        NOT NULL DEFAULT gen_random_uuid(),
    requester_user_id UUID        NOT NULL,
    recipient_user_id UUID        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'pending',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    responded_at      TIMESTAMPTZ NULL,

    CONSTRAINT pk_friend_requests
        PRIMARY KEY (id),

    CONSTRAINT fk_friend_requests_requester_user_id
        FOREIGN KEY (requester_user_id)
        REFERENCES users (id)
        ON DELETE CASCADE,
    CONSTRAINT fk_friend_requests_recipient_user_id
        FOREIGN KEY (recipient_user_id)
        REFERENCES users (id)
        ON DELETE CASCADE,

    CONSTRAINT ck_friend_requests_distinct_users
        CHECK (requester_user_id <> recipient_user_id),
    CONSTRAINT ck_friend_requests_valid_status
        CHECK (status IN ('pending', 'accepted', 'rejected', 'cancelled')),
    CONSTRAINT ck_friend_requests_responded_at_status
        CHECK (
            (status IN ('accepted', 'rejected') AND responded_at IS NOT NULL)
            OR (status IN ('pending', 'cancelled') AND responded_at IS NULL)
        )
);

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------
-- The partial unique index enforces one pending request per directed pair.
-- It intentionally allows A -> B and B -> A to coexist as distinct requests.
-- ---------------------------------------------------------------------------
CREATE UNIQUE INDEX uq_friend_requests_pending_direction
    ON friend_requests (requester_user_id, recipient_user_id)
    WHERE status = 'pending';

-- Pending request queues are paginated newest first using created_at, then ID.
CREATE INDEX idx_friend_requests_incoming_pending_created_at_id
    ON friend_requests (recipient_user_id, created_at DESC, id DESC)
    WHERE status = 'pending';

CREATE INDEX idx_friend_requests_outgoing_pending_created_at_id
    ON friend_requests (requester_user_id, created_at DESC, id DESC)
    WHERE status = 'pending';

-- ---------------------------------------------------------------------------
-- Trigger: trg_friend_requests_updated_at
-- ---------------------------------------------------------------------------
-- fn_set_updated_at() is defined in 000001_create_users. No redefinition needed.
-- ---------------------------------------------------------------------------
CREATE TRIGGER trg_friend_requests_updated_at
    BEFORE UPDATE ON friend_requests
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();