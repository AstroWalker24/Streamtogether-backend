-- =============================================================================
-- Migration: 000014_create_profiles.up
-- Purpose:   Creates the profiles table for the Profile domain (Phase 3.1).
--            Provides the social-facing identity record for every application
--            user. Every users row should have exactly one corresponding
--            profiles row (enforced by the application at provisioning time;
--            the UNIQUE constraint on user_id prevents duplicates).
-- Dependencies:
--   000001_create_users — users table and fn_set_updated_at() must exist.
-- Objects created:
--   TABLE   profiles
--   TRIGGER trg_profiles_updated_at
-- Rollback: 000014_create_profiles.down
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Table: profiles
-- ---------------------------------------------------------------------------
-- Social-facing identity layer. Stores only presentational fields.
-- Authentication data (email, password_hash, status, etc.) stays in users.
-- Lifecycle follows users: when a users row is hard-deleted, the profiles row
-- is removed automatically via the CASCADE on fk_profiles_user_id.
-- No deleted_at column — profile accessibility is determined by users.status
-- at the application layer.
-- ---------------------------------------------------------------------------
CREATE TABLE profiles (
    -- Surrogate primary key (consistent with all project tables)
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),

    -- Ownership binding — immutable after creation
    user_id         UUID        NOT NULL,

    -- User-editable social fields (all nullable: user has not set them yet)
    display_name    TEXT        NULL,
    bio             TEXT        NULL,
    avatar_url      TEXT        NULL,

    -- Audit columns
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Primary key
    CONSTRAINT pk_profiles
        PRIMARY KEY (id),

    -- One profile per user (enforces the 1:1 relationship with users)
    CONSTRAINT uq_profiles_user_id
        UNIQUE (user_id),

    -- Referential integrity; cascade removes profile on user hard-deletion
    CONSTRAINT fk_profiles_user_id
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,

    -- Guard against empty-string sentinels — application must normalize to NULL
    CONSTRAINT ck_profiles_display_name_not_empty
        CHECK (display_name IS NULL OR display_name <> ''),
    CONSTRAINT ck_profiles_avatar_url_not_empty
        CHECK (avatar_url IS NULL OR avatar_url <> ''),

    -- Hard domain invariant: bio may not exceed 300 Unicode code points
    CONSTRAINT ck_profiles_bio_length
        CHECK (bio IS NULL OR char_length(bio) <= 300)
);

-- ---------------------------------------------------------------------------
-- Trigger: trg_profiles_updated_at
-- ---------------------------------------------------------------------------
-- fn_set_updated_at() is defined in 000001_create_users. No redefinition needed.
-- ---------------------------------------------------------------------------
CREATE TRIGGER trg_profiles_updated_at
    BEFORE UPDATE ON profiles
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
