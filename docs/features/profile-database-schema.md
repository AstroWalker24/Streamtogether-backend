---
title: Profile Domain — Database Schema Design
description: Persistence model and PostgreSQL schema blueprint for the Profile domain. Covers the profiles table, relationship to users, constraint design, index strategy, trigger conventions, deletion semantics, eager-creation invariant, and migration placement.
ms.date: 2026-08-31
status: draft
phase: 3 — Social Graph
step: 3.1.2
depends-on: profile-domain-design.md, authentication-database-schema.md
---

# Profile Domain — Database Schema Design

**Document status:** Draft  
**Based on:** `docs/features/profile-domain-design.md` (draft)  
**Target database:** PostgreSQL 15+  
**Migration tool:** `golang-migrate/migrate` (file-based, `internal/database/migrations/postgres/`)  
**Expected migration number:** `000014`

---

## Table of Contents

1. [Philosophy & Goals](#1-philosophy--goals)
2. [Conventions Inherited from the Authentication Schema](#2-conventions-inherited-from-the-authentication-schema)
3. [PostgreSQL Types & Extensions](#3-postgresql-types--extensions)
4. [Profile Table Design](#4-profile-table-design)
5. [Primary Key Decision](#5-primary-key-decision)
6. [Relationship to `users`](#6-relationship-to-users)
7. [Column Specifications](#7-column-specifications)
8. [Constraint Design](#8-constraint-design)
9. [Index Strategy](#9-index-strategy)
10. [Trigger Design](#10-trigger-design)
11. [Deletion & Account Lifecycle Semantics](#11-deletion--account-lifecycle-semantics)
12. [Eager Profile Creation Invariant](#12-eager-profile-creation-invariant)
13. [Data Integrity Summary](#13-data-integrity-summary)
14. [Future Social Graph Compatibility](#14-future-social-graph-compatibility)
15. [Migration Placement](#15-migration-placement)
16. [Open Decisions](#16-open-decisions)

---

## 1. Philosophy & Goals

### Core Principles

**The profiles table is an extension of the users table, not an independent entity.** Every row in `profiles` must have a corresponding row in `users`. The schema enforces this at the database level through a foreign key and a unique constraint. The application never creates a profile without a user, and never creates a user without a profile.

**No speculative columns.** The profiles table contains only the fields defined in `profile-domain-design.md`. Columns for follower counts, friend counts, reputation scores, moderation state, cover images, location, date of birth, presence, or search vectors are not added until the domain design explicitly requires them.

**Database constraints protect domain invariants; application logic handles business rules.** The bio 300-character limit is a hard domain invariant — the database enforces it. Display-name minimum length (2 characters) and URL format validation are business rules — the application enforces them. This distinction is deliberate and consistent with the authentication schema philosophy.

**Consistency with the authentication schema is non-negotiable.** Every naming convention, type choice, trigger pattern, and FK strategy is inherited directly from the established authentication schema. No new conventions are introduced for this table.

---

## 2. Conventions Inherited from the Authentication Schema

All conventions from `authentication-database-schema.md` §2 apply without modification.

### Naming Quick Reference

| Element | Convention | Example |
|---|---|---|
| Table names | Plural `snake_case` | `profiles` |
| Column names | Singular `snake_case` | `user_id`, `display_name` |
| Primary key column | Always `id` | `id UUID` |
| Foreign key columns | `{referenced_table_singular}_id` | `user_id` |
| Primary key constraints | `pk_{table}` | `pk_profiles` |
| Unique constraints | `uq_{table}_{column(s)}` | `uq_profiles_user_id` |
| Foreign key constraints | `fk_{table}_{column}` | `fk_profiles_user_id` |
| Check constraints | `ck_{table}_{description}` | `ck_profiles_bio_length` |
| Indexes | `idx_{table}_{column(s)}` | (none additional — see §9) |
| Triggers | `trg_{table}_{action}` | `trg_profiles_updated_at` |
| Trigger function | Shared `fn_set_updated_at()` | (defined in migration 000001) |

### Standard Columns Applied to `profiles`

| Column | Type | Source |
|---|---|---|
| `id` | `UUID NOT NULL DEFAULT gen_random_uuid()` | Immutable surrogate PK. All tables carry this. |
| `created_at` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` | Set once on INSERT. Never updated. |
| `updated_at` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` | Maintained by `BEFORE UPDATE` trigger. Never set by the application. |

`profiles` does **not** carry `deleted_at`. See §11 for the deletion policy.

---

## 3. PostgreSQL Types & Extensions

No new extensions are required. `pgcrypto` and `fn_set_updated_at()` are already available from migration `000001_create_users`.

No new enum types are required. Profile fields use only built-in PostgreSQL types.

### Types Used

| Type | Used for | Rationale |
|---|---|---|
| `UUID` | `id`, `user_id` | Consistent with every other table in the project. Globally unique, opaque to clients. |
| `TEXT` | `display_name`, `bio`, `avatar_url` | No storage penalty vs. `VARCHAR(n)` in PostgreSQL. Length constraints are expressed via `CHECK` where they are domain invariants (bio). Application-layer validation handles format rules. |
| `TIMESTAMPTZ` | `created_at`, `updated_at` | UTC timestamps with timezone offset. Never `TIMESTAMP WITHOUT TIME ZONE`. |

---

## 4. Profile Table Design

```sql
CREATE TABLE profiles (
    -- Surrogate primary key
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),

    -- Ownership binding (1:1 with users)
    user_id         UUID        NOT NULL,

    -- User-editable social fields
    display_name    TEXT        NULL,
    bio             TEXT        NULL,
    avatar_url      TEXT        NULL,

    -- Audit columns
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Primary key
    CONSTRAINT pk_profiles
        PRIMARY KEY (id),

    -- One profile per user (enforces the 1:1 relationship)
    CONSTRAINT uq_profiles_user_id
        UNIQUE (user_id),

    -- Referential integrity: profile cannot outlive its user
    CONSTRAINT fk_profiles_user_id
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,

    -- Prevent empty-string sentinel values for nullable fields
    CONSTRAINT ck_profiles_display_name_not_empty
        CHECK (display_name IS NULL OR display_name <> ''),
    CONSTRAINT ck_profiles_avatar_url_not_empty
        CHECK (avatar_url IS NULL OR avatar_url <> ''),

    -- Hard domain invariant: bio may not exceed 300 Unicode code points
    CONSTRAINT ck_profiles_bio_length
        CHECK (bio IS NULL OR char_length(bio) <= 300)
);
```

---

## 5. Primary Key Decision

**Decision: `profiles` uses its own surrogate `id` UUID as the primary key. `user_id` carries a separate `UNIQUE` constraint.**

### Options Evaluated

| Option | Description |
|---|---|
| A — `user_id` as PK | `user_id UUID PRIMARY KEY REFERENCES users(id)`. No separate `id` column. |
| B — Separate `id` + UNIQUE `user_id` | `id UUID PRIMARY KEY`, `user_id UUID NOT NULL UNIQUE`. Adopted here. |

### Rationale for Option B

1. **Project convention.** Every table in the authentication schema — including `oauth_identities`, which is also a 1:1 extension of `users` — carries its own surrogate `id`. Deviating from this pattern for `profiles` would create an inconsistency that needs to be explained at every join and mapping layer.

2. **Domain entity alignment.** The `profile.Profile` domain entity (defined in `profile-domain-design.md` §2) carries both an `ID` field and a `UserID` field. The database schema mirrors the entity structure.

3. **Future reference flexibility.** Future Social Graph tables (Friends, Blocking, Privacy) may need to reference profiles by a stable key. Using a dedicated `id` decouples those references from the `user_id` semantics.

4. **The overhead is negligible.** One additional `UUID` column (16 bytes) per row is not a meaningful cost.

### Trade-off Acknowledged

The primary lookup path (`SELECT ... WHERE user_id = $1`) will use the unique index on `user_id`, not the PK index on `id`. This is correct and expected — the PK index exists for internal joins and referential integrity, not for the primary application read path.

---

## 6. Relationship to `users`

### Cardinality

```
users
  │
  │ 1
  │
  └────── 1
        profiles
```

One `users` row corresponds to exactly one `profiles` row. This is enforced by:
- `UNIQUE (user_id)` — prevents a second profile being inserted for the same user.
- `NOT NULL` on `user_id` — prevents a profile with no owner.
- `FOREIGN KEY (user_id) REFERENCES users (id)` — prevents a profile referencing a non-existent user.

### Foreign Key Deletion Behavior: `ON DELETE CASCADE`

When the `users` row is **physically deleted** (hard delete, after the 30-day soft-delete grace period), PostgreSQL automatically deletes the corresponding `profiles` row via the cascade.

**Why CASCADE and not RESTRICT?**

- `RESTRICT` would prevent user hard deletion until the profile is manually removed, forcing the purge job to delete profiles explicitly before users. This adds operational complexity with no benefit.
- `CASCADE` aligns with all other tables that reference `users` (`devices`, `sessions`, `refresh_tokens`, `email_verification_tokens`, `password_reset_tokens`, `oauth_identities`, `user_roles`). Every dependent row follows the user into deletion.

**Soft delete (grace period):**

During the soft-delete grace period, the user row is **not removed** — only `status = 'deleted'` and `deleted_at = NOW()` are set. The profile row therefore remains in place throughout the grace period. The application service checks `auth.User.Status` to determine whether the profile is accessible. No additional database mechanism is required on the `profiles` table.

---

## 7. Column Specifications

### `id`

| Attribute | Value |
|---|---|
| Type | `UUID` |
| Nullability | `NOT NULL` |
| Default | `gen_random_uuid()` |
| Constraint | `pk_profiles PRIMARY KEY` |
| Set by | System only. Never accepted from user input. |
| Mutability | Immutable after creation. |
| Purpose | Surrogate primary key. Consistent with all project tables. |

### `user_id`

| Attribute | Value |
|---|---|
| Type | `UUID` |
| Nullability | `NOT NULL` |
| Default | None. Must be provided at INSERT. |
| Constraints | `uq_profiles_user_id UNIQUE`, `fk_profiles_user_id FOREIGN KEY → users(id) ON DELETE CASCADE` |
| Set by | System only, at profile creation. |
| Mutability | Immutable after creation. A profile can never be transferred to a different user. |
| Purpose | The ownership binding that ties this profile to its user. The primary application lookup key. |

### `display_name`

| Attribute | Value |
|---|---|
| Type | `TEXT` |
| Nullability | `NULL` |
| Default | `NULL` (user has not set a display name) |
| Constraint | `ck_profiles_display_name_not_empty: display_name IS NULL OR display_name <> ''` |
| Set by | User (editable). |
| Mutability | Freely editable. |
| Purpose | User-chosen presentational name shown in friend lists, party rooms, and social contexts. Falls back to `users.username` in read models when NULL. |
| Not unique | No uniqueness constraint. Multiple users may share the same display name. `users.username` is the unique identifier. |
| Empty string | The application normalizes empty string to NULL before persistence. The CHECK constraint ensures empty strings are never stored. |
| Length | Maximum 50 Unicode code points enforced by the application layer. Not enforced at the database level, consistent with the auth schema philosophy on TEXT length. |

### `bio`

| Attribute | Value |
|---|---|
| Type | `TEXT` |
| Nullability | `NULL` |
| Default | `NULL` (user has no bio) |
| Constraint | `ck_profiles_bio_length: bio IS NULL OR char_length(bio) <= 300` |
| Set by | User (editable). |
| Mutability | Freely editable. User may update or clear at any time. |
| Purpose | Optional short self-description displayed on the user's profile page. |
| Empty string | The application normalizes empty string to NULL before persistence. No database CHECK for empty string — the length check `<= 300` already covers it, and NULL is the canonical "no bio" state. |
| Maximum length | **300 Unicode code points.** Enforced at the database level by `ck_profiles_bio_length`. This is a hard domain invariant, not merely application preference. |
| Full-text index | None. Search indexing for bio is deferred (OD-6). The constraint does not prevent a future `tsvector` column or `pg_trgm` index from being added by a later migration. |

### `avatar_url`

| Attribute | Value |
|---|---|
| Type | `TEXT` |
| Nullability | `NULL` |
| Default | `NULL` (user has no avatar) |
| Constraint | `ck_profiles_avatar_url_not_empty: avatar_url IS NULL OR avatar_url <> ''` |
| Set by | User (editable). |
| Mutability | Freely editable. User may update or clear at any time. |
| Purpose | HTTPS reference to the user's profile picture. Stores a URL string only — no binary image data is ever stored in PostgreSQL. |
| Empty string | The application normalizes empty string to NULL before persistence. The CHECK constraint ensures empty strings are never stored. |
| Format | HTTPS URL. Format validation is enforced by the application layer. The database enforces only that the value is non-empty when present. |
| Length | Maximum 2048 characters enforced by the application layer. Not enforced at the database level (consistent with TEXT philosophy). |
| Storage provider | Deferred (OD-3). The column stores whatever URL format the future storage provider requires. No migration change is needed when the provider is chosen. |

### `created_at`

| Attribute | Value |
|---|---|
| Type | `TIMESTAMPTZ` |
| Nullability | `NOT NULL` |
| Default | `NOW()` |
| Set by | Database default on INSERT. |
| Mutability | Immutable. Never updated. |
| Purpose | Records when the profile was first created. Consistent with all project tables. |

### `updated_at`

| Attribute | Value |
|---|---|
| Type | `TIMESTAMPTZ` |
| Nullability | `NOT NULL` |
| Default | `NOW()` |
| Set by | `trg_profiles_updated_at` trigger on every UPDATE. Never set by the application. |
| Mutability | Updated automatically on every row mutation. |
| Purpose | Tracks the most recent profile modification. Useful for cache invalidation and ordering queries. |

---

## 8. Constraint Design

### Complete Constraint Inventory

| Constraint Name | Type | Columns | Rule |
|---|---|---|---|
| `pk_profiles` | PRIMARY KEY | `id` | Surrogate PK. Every row has a unique, non-null identifier. |
| `uq_profiles_user_id` | UNIQUE | `user_id` | One profile per user. Prevents a second profile being inserted for the same `user_id`. Also creates the B-tree index used by the `WHERE user_id = $1` hot path. |
| `fk_profiles_user_id` | FOREIGN KEY | `user_id → users(id)` | Referential integrity. A profile cannot reference a non-existent user. Cascade deletes profile when the user is hard-deleted. |
| `ck_profiles_display_name_not_empty` | CHECK | `display_name` | `display_name IS NULL OR display_name <> ''`. Prevents the application from persisting an empty string as a display name. The application is expected to normalize empty → NULL. |
| `ck_profiles_avatar_url_not_empty` | CHECK | `avatar_url` | `avatar_url IS NULL OR avatar_url <> ''`. Same empty-string prevention for the avatar URL. |
| `ck_profiles_bio_length` | CHECK | `bio` | `bio IS NULL OR char_length(bio) <= 300`. Hard domain invariant. Enforced at the database level because a bio longer than 300 code points is a data corruption condition, not merely a display concern. |

### Why No Additional CHECK Constraints

| Consideration | Decision |
|---|---|
| `display_name` minimum length (2 chars) | Application-level validation. A single-character display name is a business rule, not a data integrity invariant. |
| `display_name` maximum length (50 chars) | Application-level validation. Consistent with the auth schema philosophy that length limits for TEXT fields are not repeated as DB CHECK constraints unless they are hard domain invariants. |
| `avatar_url` URL format (HTTPS only) | Application-level validation. URL format constraints are complex to express in SQL and belong in the service layer. |
| `avatar_url` maximum length (2048 chars) | Application-level validation. See above. |
| `display_name` UNIQUE | Explicitly excluded. Display names are not unique (OD-1). No uniqueness constraint is defined. |

### Database-Enforced vs Application-Enforced Invariants

| Invariant | Enforcement Layer | Mechanism |
|---|---|---|
| Every profile belongs to an existing user | Database | `fk_profiles_user_id` |
| A user has at most one profile | Database | `uq_profiles_user_id` |
| Profile is deleted when user is hard-deleted | Database | `ON DELETE CASCADE` |
| Bio cannot exceed 300 Unicode code points | **Database** | `ck_profiles_bio_length` |
| Display name is not empty string | **Database** | `ck_profiles_display_name_not_empty` |
| Avatar URL is not empty string | **Database** | `ck_profiles_avatar_url_not_empty` |
| Display name maximum 50 characters | Application | Service-layer validation |
| Display name minimum 2 characters | Application | Service-layer validation |
| Avatar URL must be HTTPS | Application | Service-layer validation |
| Avatar URL maximum 2048 characters | Application | Service-layer validation |
| Display names are not unique | N/A | No constraint defined |

---

## 9. Index Strategy

### Explicit Index Decision

Only one index is required beyond what constraints provide automatically.

| Index | Type | Provided By | Serves |
|---|---|---|---|
| PK on `id` | B-tree | `pk_profiles PRIMARY KEY` (automatic) | Internal joins, referential integrity lookups. |
| Unique index on `user_id` | B-tree | `uq_profiles_user_id UNIQUE` (automatic) | **Primary application read path:** `SELECT * FROM profiles WHERE user_id = $1`. No separate index is needed. |

**No additional explicit indexes are created.**

### Indexes Deliberately Not Created

| Index | Reason Not Created |
|---|---|
| `idx_profiles_display_name` | Display name search belongs to the Search module (Phase 3.4). No current read path queries profiles by display name without a user_id. |
| `idx_profiles_created_at` | No current query requires ordering or filtering profiles by creation time without first knowing the user_id. |
| `idx_profiles_updated_at` | No current query pattern. Future cache invalidation or CDC may require it — added when needed. |
| Full-text index on `bio` | Explicitly deferred (OD-6). |
| `pg_trgm` index on `display_name` | Belongs to the Search module. Not created here. |

### Index Sufficiency

The single unique index on `user_id` covers the only profile access pattern that exists at this phase of the Social Graph:

- Own profile read: `WHERE user_id = $1`
- Public profile read: `WHERE user_id = $1`
- Profile update: `WHERE user_id = $1`
- Existence check during eager creation: `WHERE user_id = $1`

All four hit the same index. No other index is needed at this stage.

---

## 10. Trigger Design

### `trg_profiles_updated_at`

**Purpose:** Automatically set `updated_at = NOW()` on every `UPDATE` to the `profiles` table. The application never sets `updated_at` directly.

```sql
CREATE TRIGGER trg_profiles_updated_at
    BEFORE UPDATE ON profiles
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
```

**The trigger function `fn_set_updated_at()` already exists.** It was defined in migration `000001_create_users`. It is shared by all tables that carry an `updated_at` column. No new trigger function is required for the Profile migration.

```sql
-- fn_set_updated_at is already defined in 000001_create_users.up.sql:
-- CREATE OR REPLACE FUNCTION fn_set_updated_at()
-- RETURNS TRIGGER LANGUAGE plpgsql AS $$
-- BEGIN
--     NEW.updated_at = NOW();
--     RETURN NEW;
-- END;
-- $$;
```

The migration for `profiles` creates only the trigger binding — not the function.

---

## 11. Deletion & Account Lifecycle Semantics

### Profile Does Not Have an Independent Lifecycle

`profiles` has no `deleted_at` column and no independent deletion state. Profile lifecycle is entirely driven by the associated `users` row.

### Lifecycle Mapping

| `users.status` | `users.deleted_at` | `profiles` row state | Public accessibility |
|---|---|---|---|
| `pending_verification` | NULL | Row exists, all fields potentially NULL | Not publicly accessible (account not verified) |
| `active` | NULL | Row exists, fields editable | Accessible per future privacy settings |
| `suspended` | NULL | Row exists, not modified | Not publicly accessible (account suspended) |
| `deleted` (soft) | Non-NULL (grace period) | Row exists, not modified | Not publicly accessible |
| Hard-deleted | User row removed | **Profile row auto-deleted via CASCADE** | Row no longer exists |

### How the Application Enforces Accessibility

The `profiles` table does not store a copy of `users.status`. The service layer reads the user's status from `auth.User` and refuses social operations when the account is not `active`. This is the same pattern used by the authentication domain — there is no redundant status column on the dependent tables.

### Profile Deletion Is Not Independent (OD-5)

There is no `DELETE FROM profiles WHERE user_id = $1` endpoint. The only path to profile deletion is user account hard-deletion. This is enforced architecturally: the Profile service does not expose a delete operation. The cascade is the only deletion mechanism.

---

## 12. Eager Profile Creation Invariant

### Database Invariant

Every row in `users` that represents an active account should have a corresponding row in `profiles`.

The database enforces half of this invariant:
- **The FK and UNIQUE constraints prevent** a `profiles` row without a `users` row.
- **The FK and UNIQUE constraints prevent** more than one `profiles` row per `users` row.

The database **cannot enforce** that every `users` row has a corresponding `profiles` row (that would require a FK in the reverse direction, which PostgreSQL does not support as a deferrable constraint without triggers).

### Application Responsibility

Eager profile creation — inserting into `profiles` atomically alongside the `users` INSERT — is the responsibility of the registration and OAuth provisioning service.

**Password-based registration:** The registration transaction must INSERT into `users` and INSERT into `profiles` in the same database transaction. If either INSERT fails, both are rolled back.

**OAuth provisioning:** The OAuth user provisioning transaction must INSERT into `users`, INSERT into `oauth_identities`, and INSERT into `profiles` in the same database transaction.

**Invariant enforcement:** If a `users` row exists without a corresponding `profiles` row, it indicates a bug in the provisioning path. The Profile repository's `GetByUserID` method will return `PROFILE_NOT_FOUND` for that user, which is the observable symptom of this bug.

---

## 13. Data Integrity Summary

### Complete Integrity Picture

```
Database-enforced:
────────────────────────────────────────────────────────────────
✓ profiles.user_id references a real users row       (FK)
✓ Each user has at most one profile                  (UNIQUE)
✓ Profile row is removed when user is hard-deleted   (CASCADE)
✓ bio does not exceed 300 Unicode code points        (CHECK)
✓ display_name is not an empty string when set       (CHECK)
✓ avatar_url is not an empty string when set         (CHECK)
✓ updated_at reflects the last row mutation          (TRIGGER)

Application-enforced:
────────────────────────────────────────────────────────────────
✓ Each user has exactly one profile (eager creation) (service)
✓ display_name is 2–50 characters when set           (service)
✓ avatar_url is an HTTPS URL when set                (service)
✓ avatar_url does not exceed 2048 characters         (service)
✓ bio whitespace is normalized before persistence    (service)
✓ display_name whitespace is normalized              (service)
✓ Empty strings are converted to NULL                (service)
✓ Profile is accessible only for active accounts     (service)
✓ Only the profile owner may mutate their profile    (service)
```

---

## 14. Future Social Graph Compatibility

The `profiles` table schema is designed to remain stable through the subsequent Social Graph modules. No changes to `profiles` are anticipated by the following modules:

| Module | Relationship to `profiles` | Schema impact |
|---|---|---|
| **Friends (3.2)** | A `friends` table will join `user_id` pairs. Reads `profiles` by `user_id` to render friend identities. | New table — no change to `profiles`. |
| **Friend Requests (3.3)** | A `friend_requests` table references `user_id` pairs. Resolves `ProfileSummary` at read time. | New table — no change to `profiles`. |
| **Search (3.4)** | May add a `tsvector` generated column or a `pg_trgm` index on `display_name` to `profiles`. Or may maintain a separate search index table. | Additive migration — no existing columns affected. |
| **Presence (3.5)** | A separate presence store (likely Redis). Resolves `ProfileSummary` for display. | No DB schema change to `profiles`. |
| **Blocking (3.6)** | A `blocks` table joins `user_id` pairs. Filters profile results at the query layer. | New table — no change to `profiles`. |
| **Privacy (3.7)** | A `privacy_settings` or `profile_visibility` table controls per-field visibility. | New table — no change to `profiles`. `profiles` fields are the data source; privacy is the visibility layer. |
| **User Preferences (3.8)** | A separate `user_preferences` table. Not structurally related to `profiles`. | New table — no change to `profiles`. |

The `profiles.user_id` column is the stable join key used by all future modules. No future module needs to reference `profiles.id` directly — they join through `user_id`.

---

## 15. Migration Placement

### File Location

All migration files must be placed in the existing directory:

```
internal/database/migrations/postgres/
```

No new directories are created. This is consistent with all 13 existing migrations.

### Expected Migration Number

The existing migrations are numbered:

```
000001 → 000010  (continuous)
000012           (note: 000011 is absent from the directory — gap is pre-existing)
000013
```

The next migration number is: **`000014`**

The profile migration will produce two files:

```
internal/database/migrations/postgres/000014_create_profiles.up.sql
internal/database/migrations/postgres/000014_create_profiles.down.sql
```

### Migration Dependencies

The profile migration depends on:

| Migration | Reason |
|---|---|
| `000001_create_users` | `fn_set_updated_at()` function must exist. `users` table must exist as FK target. |

No other migrations are required. The `profiles` table references only `users`.

### Migration Header Convention

The migration file header must follow the established format:

```sql
-- =============================================================================
-- Migration: 000014_create_profiles.up
-- Purpose:   Creates the profiles table for the Profile domain (Phase 3.1).
--            Provides the social-facing identity record for every application user.
-- Dependencies:
--   000001_create_users — users table + fn_set_updated_at must exist.
-- Objects created:
--   TABLE   profiles
--   TRIGGER trg_profiles_updated_at
-- Rollback: 000014_create_profiles.down
-- =============================================================================
```

---

## 16. Open Decisions

All schema-level decisions are resolved. No open items remain.

| # | Decision | Resolution |
|---|---|---|
| OD-3 | **Avatar storage provider.** | **Deferred.** `avatar_url TEXT NULL` with no provider-specific CHECK constraint. The column is compatible with any URL format. Resolve provider selection in a later phase. |
| OD-S1 | **Search index strategy for `display_name`.** | **Deferred to Phase 3.4 Search.** No search indexing in this migration. The schema is compatible with all future options (generated `tsvector`, `pg_trgm` index, or separate index table). |
| OD-S2 | **Missing `000011` migration.** | **Intentionally absent.** The gap exists because Step 2.7 indexes were created alongside their respective table migrations. No fill migration is needed. Profile migration proceeds as `000014`. |
