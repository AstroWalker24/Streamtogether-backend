---
title: Search Database / Indexing Design
description: PostgreSQL data-source, extension, query, and index design for authenticated user Search over usernames and display names.
ms.date: 2026-09-09
status: draft
phase: 3 — Social Graph
step: 3.4.2
depends-on: search-domain-design.md, profile-database-schema.md, authentication-database-schema.md, friends-database-schema.md, friend-requests-database-schema.md
---

# Search Database / Indexing Design

**Document status:** Draft  
**Based on:** `docs/features/search-domain-design.md`  
**Target database:** PostgreSQL 15+  
**Migration tool:** `golang-migrate/migrate` (file-based, `internal/database/migrations/postgres/`)  
**Expected migration number:** `000017`  
**Expected migration name:** `000017_add_search_indexes`

## Table of Contents

1. [Purpose](#1-purpose)
2. [Existing Schema and Data Sources](#2-existing-schema-and-data-sources)
3. [Search Strategy Evaluation](#3-search-strategy-evaluation)
4. [Extension Requirements](#4-extension-requirements)
5. [Index Design](#5-index-design)
6. [Case Normalization](#6-case-normalization)
7. [Ranking Implementation Feasibility](#7-ranking-implementation-feasibility)
8. [Pagination and Ordering](#8-pagination-and-ordering)
9. [Account Visibility](#9-account-visibility)
10. [Conceptual Query Shape](#10-conceptual-query-shape)
11. [Migration Plan](#11-migration-plan)
12. [Performance Considerations](#12-performance-considerations)
13. [Module Boundaries](#13-module-boundaries)
14. [Explicitly Excluded Design Choices](#14-explicitly-excluded-design-choices)
15. [Open Decisions](#15-open-decisions)

---

## 1. Purpose

This document defines the database and indexing design required to support Phase 3.4 Search. It does not define repository interfaces, service behavior, API routes, DTOs, migrations, or tests.

The finalized domain behavior from `search-domain-design.md` is binding:

- Search is authenticated.
- Minimum normalized query length is 2 characters.
- Maximum normalized query length is 100 characters.
- Empty or whitespace-only queries are invalid.
- Search matches `username` and `display_name` only.
- `bio`, `email`, account status/timestamps, Profile ID, and `relationship_state` are not searchable and are not returned.
- Matching supports exact, prefix, and substring matches.
- Matching is case-insensitive.
- Accent and diacritic folding are not applied.
- Ranking order is: username exact, display-name exact, username prefix, display-name prefix, username substring, display-name substring.
- The authenticated user is excluded from their own results.
- Existing `page` / `page_size` pagination conventions are used.
- Existing middleware handles rate limiting; no Search-specific rate limiter is introduced.

Search remains a PostgreSQL-backed read model over existing `users` and `profiles` data. No Search table, separate database, or external search engine is introduced.

## 2. Existing Schema and Data Sources

Search composes data from the existing `users` and `profiles` tables.

| Search need | Table | Column | Existing source of truth |
|---|---|---|---|
| Result user ID | `users` | `id` | `auth.User.ID` |
| Match/display username | `users` | `username` | `auth.User.Username` |
| Account status filter | `users` | `status` | `auth.User.Status`; `user_status` enum from `000001_create_users` |
| Soft-delete filter | `users` | `deleted_at` | `auth.User.DeletedAt` |
| Match/display display name | `profiles` | `display_name` | `profile.Profile.DisplayName` |
| Result avatar URL | `profiles` | `avatar_url` | `profile.Profile.AvatarURL` |
| User/Profile join | `profiles` | `user_id` | Unique FK to `users(id)` |

### Existing relevant indexes

| Index | Table | Definition | Relevance to Search |
|---|---|---|---|
| `pk_users` | `users` | `PRIMARY KEY (id)` | Joins by `users.id` and excludes `:actor_user_id` by row identity. |
| `uq_users_username` | `users` | `UNIQUE (username)` | Case-sensitive uniqueness only; not sufficient for case-insensitive Search. |
| `uq_users_username_lower` | `users` | `UNIQUE (lower(username))` | Supports case-insensitive exact username lookup. |
| `idx_users_status` | `users` | `(status)` | Helps account-state filtering in broad user queries, but does not support text matching. |
| `idx_users_deleted_at` | `users` | `(deleted_at) WHERE deleted_at IS NOT NULL` | Supports soft-delete cleanup, not active-user Search. |
| `pk_profiles` | `profiles` | `PRIMARY KEY (id)` | Not used by Search result construction; Profile ID is not returned. |
| `uq_profiles_user_id` | `profiles` | `UNIQUE (user_id)` | Supports joining the user's Profile row. |

`profiles.display_name` currently has no search index. `profile-database-schema.md` explicitly deferred `idx_profiles_display_name`, full-text indexing, and `pg_trgm` indexing to Phase 3.4.

## 3. Search Strategy Evaluation

### B-tree indexes

B-tree indexes are excellent for equality and ordered range/prefix scans when the indexed expression and operator class match the predicate.

| Requirement | B-tree fit | Decision |
|---|---|---|
| Case-insensitive exact username | Strong; already covered by `uq_users_username_lower`. | Reuse existing index. |
| Case-insensitive exact display name | Strong with `lower(display_name)`. | Covered by the proposed pattern index. |
| Case-insensitive prefix matching | Strong when using `text_pattern_ops` on `lower(...)`. | Add B-tree pattern indexes for username and display name. |
| Case-insensitive substring matching | Poor; leading-wildcard `LIKE '%term%'` cannot use normal B-tree ordering. | Do not rely on B-tree for substring. |

A plain functional B-tree index on `lower(display_name)` would support equality, but not portable prefix matching under all collations. A `text_pattern_ops` functional index supports both equality and prefix `LIKE` predicates on the lowered value, so it avoids an additional exact-only index.

### Functional `lower(...)` indexes

The project already uses functional indexes for case-insensitive identity lookup:

```sql
CREATE UNIQUE INDEX uq_users_username_lower
    ON users (lower(username));
```

Search follows the same convention. No normalized username or display-name column is added.

### `pg_trgm` / trigram indexes

`pg_trgm` is the best PostgreSQL-native fit for case-insensitive substring matching over short name-like fields. A GIN index using `gin_trgm_ops` can support `LIKE`, `ILIKE`, and equality-style matching on expressions such as `lower(username)` and `lower(display_name)`.

Search requires substring matching, so trigram indexes are required for the initial scalable PostgreSQL implementation.

### PostgreSQL full-text search

Full-text search is not selected for the initial Search design.

Full-text search tokenizes text into lexemes and ranks documents by linguistic relevance. That is useful for document search, but it does not naturally express the finalized Search semantics:

- exact username match
- exact display-name match
- prefix match
- substring match inside a username or display name
- deterministic tier ordering rather than linguistic relevance

Using full-text search would either fail the substring requirement or require extra mechanisms beside it. It also does not fit usernames well because usernames are identifiers, not prose.

### External search engines

No Elasticsearch, OpenSearch, Meilisearch, Typesense, or similar service is introduced. The current Search scope is a small user-discovery feature over two short text fields, and PostgreSQL can support it with additive indexes.

## 4. Extension Requirements

The only currently enabled PostgreSQL extension is `pgcrypto`, introduced in `000001_create_users` for `gen_random_uuid()` compatibility.

`pg_trgm` is not currently enabled and is required for the proposed trigram GIN indexes.

The future migration must enable it explicitly:

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
```

This follows the existing extension-management convention used by `000001_create_users`: extensions are created by migrations, not enabled manually in a developer or production database.

`citext` is not required. The project already chose `TEXT` plus functional `lower(...)` indexes for case-insensitive identity behavior, and Search should preserve that pattern.

## 5. Index Design

The design adds four explicit indexes and one extension. No table or column is added.

### Proposed indexes

| Index | Table | Indexed expression | Type | Purpose |
|---|---|---|---|---|
| `idx_users_active_username_lower_pattern` | `users` | `lower(username) text_pattern_ops` where `status = 'active' AND deleted_at IS NULL` | Partial B-tree | Supports case-insensitive exact and prefix username matching for discoverable accounts. |
| `idx_users_active_username_trgm` | `users` | `lower(username) gin_trgm_ops` where `status = 'active' AND deleted_at IS NULL` | Partial GIN | Supports case-insensitive substring username matching for discoverable accounts. |
| `idx_profiles_display_name_lower_pattern` | `profiles` | `lower(display_name) text_pattern_ops` where `display_name IS NOT NULL` | Partial B-tree | Supports case-insensitive exact and prefix display-name matching. |
| `idx_profiles_display_name_trgm` | `profiles` | `lower(display_name) gin_trgm_ops` where `display_name IS NOT NULL` | Partial GIN | Supports case-insensitive substring display-name matching. |

### Index definitions for the future migration

```sql
CREATE INDEX idx_users_active_username_lower_pattern
    ON users (lower(username) text_pattern_ops)
    WHERE status = 'active' AND deleted_at IS NULL;

CREATE INDEX idx_users_active_username_trgm
    ON users USING GIN (lower(username) gin_trgm_ops)
    WHERE status = 'active' AND deleted_at IS NULL;

CREATE INDEX idx_profiles_display_name_lower_pattern
    ON profiles (lower(display_name) text_pattern_ops)
    WHERE display_name IS NOT NULL;

CREATE INDEX idx_profiles_display_name_trgm
    ON profiles USING GIN (lower(display_name) gin_trgm_ops)
    WHERE display_name IS NOT NULL;
```

### Why these indexes are needed

#### `idx_users_active_username_lower_pattern`

The existing `uq_users_username_lower` supports exact case-insensitive username lookups across all users. Search also needs prefix matching and must exclude suspended, pending, and soft-deleted users. A partial pattern index gives the planner an active-user-only access path for:

```sql
lower(u.username) = :query
lower(u.username) LIKE :query || '%'
```

The `text_pattern_ops` operator class is selected because prefix `LIKE` over a lowered expression is the required access pattern.

#### `idx_users_active_username_trgm`

Substring username matching requires a leading wildcard:

```sql
lower(u.username) LIKE '%' || :query || '%'
```

A normal B-tree index cannot accelerate this predicate. The partial trigram GIN index supports this query while avoiding rows that Search can never return (`status <> 'active'` or `deleted_at IS NOT NULL`).

#### `idx_profiles_display_name_lower_pattern`

Display names are not unique and currently have no index. This partial pattern index supports exact and prefix display-name matching while excluding null display names:

```sql
lower(p.display_name) = :query
lower(p.display_name) LIKE :query || '%'
```

The associated query must still join `profiles.user_id` to `users.id` and apply account eligibility because Profile does not store account state.

#### `idx_profiles_display_name_trgm`

Substring display-name matching requires trigram indexing for the same reason as username substring matching. This index is partial on `display_name IS NOT NULL` because null display names cannot match and should not consume index space.

### Existing indexes reused

| Existing index | Use |
|---|---|
| `uq_users_username_lower` | Fast exact username lookup, duplicate-enforcement invariant remains owned by Auth. |
| `uq_profiles_user_id` | Join each matching Profile row to its User row. |
| `pk_users` | Join from `profiles.user_id` to `users.id`, and exclude the authenticated user. |

### Indexes deliberately not created

| Index | Reason not created |
|---|---|
| Search table or materialized search table | Duplicates User/Profile data and adds synchronization complexity without a current need. |
| Generated lowercase columns | Functional indexes already match project convention and avoid schema mutation. |
| `citext` conversion indexes | Would require changing column types or introducing a new case-insensitive type convention. |
| Full-text `tsvector` index | Does not fit identifier/name exact, prefix, and substring semantics. |
| `idx_users_active_status_deleted_id` | Existing status/deleted filters plus text indexes are sufficient; Search is text-selective first. Add only if plans show account filtering dominates. |
| Covering indexes for result columns | PostgreSQL GIN cannot provide the deterministic tier sort alone; adding broad covering B-tree indexes would increase write cost without eliminating the final sort. |
| Relationship-state indexes | `relationship_state` is deferred and not part of initial Search. |

## 6. Case Normalization

Search uses functional `lower(...)` expressions in both predicates and indexes:

```sql
lower(u.username)
lower(p.display_name)
```

The repository receives the domain-normalized query string and passes it as a parameter. The SQL still compares against `lower(...)` to guarantee case-insensitive behavior using the same expression stored in indexes.

No lowercase shadow columns are added. No stored data is rewritten. Display-name casing remains presentational and is returned as stored.

Accent and diacritic folding are not applied. The database query must not call `unaccent`, and the migration must not enable the `unaccent` extension.

## 7. Ranking Implementation Feasibility

The ranking tiers from 3.4.1 can be calculated directly in SQL using a `CASE` expression over lowered fields.

```sql
CASE
    WHEN lower(u.username) = :query THEN 1
    WHEN lower(p.display_name) = :query THEN 2
    WHEN lower(u.username) LIKE :prefix THEN 3
    WHEN lower(p.display_name) LIKE :prefix THEN 4
    WHEN lower(u.username) LIKE :contains THEN 5
    WHEN lower(p.display_name) LIKE :contains THEN 6
END AS rank_tier
```

The `WHERE` clause should include the same match predicates so rows that match neither field are excluded before ranking.

The B-tree pattern indexes support tiers 1-4. The trigram indexes support tiers 5-6 for substring matching, especially for queries of three or more characters. Two-character substring searches are valid by domain decision, but trigram indexes are less selective for patterns shorter than three characters; see §12.

## 8. Pagination and Ordering

Search uses the existing repository pagination model:

```text
LIMIT :page_size
OFFSET (:page - 1) * :page_size
```

The stable order is:

```sql
ORDER BY rank_tier ASC, username ASC, user_id ASC
```

The database cannot fully satisfy this order from a single index because:

1. Matches can originate from two different tables (`users.username` and `profiles.display_name`).
2. Ranking is computed from a `CASE` expression over multiple predicates.
3. Substring matching uses GIN indexes, which identify candidate rows but do not return rows in final sorted order.

The expected query therefore uses indexes to narrow candidates, computes `rank_tier`, sorts the candidate result set, and then applies page/offset pagination. This preserves the existing pagination convention and the deterministic ranking contract without introducing cursor pagination.

For accurate `PageMeta`, the total count must be computed using the same account-eligibility, self-exclusion, and match predicates as the data query.

## 9. Account Visibility

Search must exclude accounts that are not discoverable according to existing User lifecycle semantics.

The eligibility predicate is:

```sql
u.status = 'active'
AND u.deleted_at IS NULL
AND u.id <> :actor_user_id
```

This excludes:

| Account state | Database condition excluding it |
|---|---|
| `pending_verification` | `status <> 'active'` |
| `suspended` | `status <> 'active'` |
| `deleted` / soft-deleted | `status <> 'active'` and/or `deleted_at IS NOT NULL` |
| Hard-deleted | No `users` row exists; `profiles` row is removed by cascade. |

Search does not add account-state columns to `profiles` and does not create Search-owned visibility columns. The `users` table remains the source of truth for account lifecycle.

## 10. Conceptual Query Shape

The future repository should implement Search with separate ranked candidate branches. This lets PostgreSQL use B-tree pattern indexes for exact/prefix matches and trigram GIN indexes for substring-only matches. This is design-level SQL, not production repository code.

```sql
-- Parameters supplied by the repository after domain validation:
-- :actor_user_id UUID
-- :query         normalized query, already length-checked (2..100)
-- :prefix        :query || '%'
-- :contains      '%' || :query || '%'
-- :limit         normalized page_size
-- :offset        normalized offset

WITH base AS NOT MATERIALIZED (
    SELECT
        u.id AS user_id,
        u.username,
        p.display_name,
        p.avatar_url,
        lower(u.username) AS username_search,
        lower(p.display_name) AS display_name_search
    FROM users u
    JOIN profiles p ON p.user_id = u.id
    WHERE u.status = 'active'
      AND u.deleted_at IS NULL
      AND u.id <> :actor_user_id
), candidates AS (
        SELECT *, 1 AS rank_tier
        FROM base
        WHERE username_search = :query

        UNION ALL

        SELECT *, 2 AS rank_tier
        FROM base
        WHERE display_name_search = :query

        UNION ALL

        SELECT *, 3 AS rank_tier
        FROM base
        WHERE username_search LIKE :prefix
            AND username_search <> :query

        UNION ALL

        SELECT *, 4 AS rank_tier
        FROM base
        WHERE display_name_search LIKE :prefix
            AND display_name_search <> :query

        UNION ALL

        SELECT *, 5 AS rank_tier
        FROM base
        WHERE username_search LIKE :contains
            AND username_search NOT LIKE :prefix

        UNION ALL

        SELECT *, 6 AS rank_tier
        FROM base
        WHERE display_name_search LIKE :contains
            AND display_name_search NOT LIKE :prefix
), best_match AS (
        SELECT DISTINCT ON (user_id)
                user_id,
                username,
                display_name,
                avatar_url,
                rank_tier
        FROM candidates
        ORDER BY user_id, rank_tier ASC
)
SELECT
    user_id,
    username,
    display_name,
    avatar_url
FROM best_match
ORDER BY rank_tier ASC, username ASC, user_id ASC
LIMIT :limit OFFSET :offset;
```

An implementation may inline the base filter into each branch if the planner does not push predicates through the CTE as desired. The contract is the same: each branch returns a candidate tier, then `DISTINCT ON (user_id)` or an equivalent `MIN(rank_tier)` grouping chooses the best tier per user before final ordering and pagination.

A simpler `OR` predicate is valid only if query plans show reliable use of `BitmapOr` across the proposed indexes:

```sql
WHERE lower(u.username) LIKE :contains
   OR lower(p.display_name) LIKE :contains
```

The `UNION ALL` shape is the preferred starting point for 3.4.3 because it maps directly to the proposed indexes and keeps tier ordering explicit.

## 11. Migration Plan

The latest existing migration in the repository is `000016_create_friend_requests`. The next migration for Search indexing should be:

```text
internal/database/migrations/postgres/000017_add_search_indexes.up.sql
internal/database/migrations/postgres/000017_add_search_indexes.down.sql
```

These files are intentionally **not created** in this design step.

### Up migration operations

1. Enable `pg_trgm` if it does not exist.
2. Create `idx_users_active_username_lower_pattern`.
3. Create `idx_users_active_username_trgm`.
4. Create `idx_profiles_display_name_lower_pattern`.
5. Create `idx_profiles_display_name_trgm`.

Conceptual order:

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX idx_users_active_username_lower_pattern
    ON users (lower(username) text_pattern_ops)
    WHERE status = 'active' AND deleted_at IS NULL;

CREATE INDEX idx_users_active_username_trgm
    ON users USING GIN (lower(username) gin_trgm_ops)
    WHERE status = 'active' AND deleted_at IS NULL;

CREATE INDEX idx_profiles_display_name_lower_pattern
    ON profiles (lower(display_name) text_pattern_ops)
    WHERE display_name IS NOT NULL;

CREATE INDEX idx_profiles_display_name_trgm
    ON profiles USING GIN (lower(display_name) gin_trgm_ops)
    WHERE display_name IS NOT NULL;
```

### Down migration operations

Drop Search-owned indexes before dropping the extension:

```sql
DROP INDEX IF EXISTS idx_profiles_display_name_trgm;
DROP INDEX IF EXISTS idx_profiles_display_name_lower_pattern;
DROP INDEX IF EXISTS idx_users_active_username_trgm;
DROP INDEX IF EXISTS idx_users_active_username_lower_pattern;

DROP EXTENSION IF EXISTS pg_trgm;
```

If a later migration depends on `pg_trgm`, normal forward-only migration ordering means that later migration must be rolled back first. No `CASCADE` should be used when dropping `pg_trgm`; accidental removal of dependent future objects should not be hidden.

### Operational considerations

For small initial tables, normal `CREATE INDEX` is acceptable and matches existing migration style. For a production database with a large existing user base, `CREATE INDEX CONCURRENTLY` may be safer to reduce write blocking, but it has migration-runner implications and cannot run inside an explicit transaction. That operational choice belongs to the actual migration implementation step, not this design document.

No data backfill is required because all proposed indexes are built from existing `users` and `profiles` columns.

## 12. Performance Considerations

### Expected index use

| Predicate | Expected index support |
|---|---|
| `lower(username) = :query` | Existing `uq_users_username_lower` and/or proposed active username pattern index. |
| `lower(username) LIKE :query || '%'` | `idx_users_active_username_lower_pattern`. |
| `lower(username) LIKE '%' || :query || '%'` | `idx_users_active_username_trgm`, strongest for 3+ character queries. |
| `lower(display_name) = :query` | `idx_profiles_display_name_lower_pattern`. |
| `lower(display_name) LIKE :query || '%'` | `idx_profiles_display_name_lower_pattern`. |
| `lower(display_name) LIKE '%' || :query || '%'` | `idx_profiles_display_name_trgm`, strongest for 3+ character queries. |
| `u.status = 'active' AND u.deleted_at IS NULL` | Embedded in the partial username indexes; applied after joining for display-name matches. |

### Two-character query trade-off

The domain minimum is 2 characters. Trigram indexes are most selective when the search pattern contains at least one trigram (normally 3 or more characters). Therefore:

- Two-character exact and prefix searches are supported efficiently by the B-tree pattern indexes.
- Two-character substring searches are valid but may be less selective and may require more heap rechecks or broader scans.

This is an intentional trade-off to preserve the finalized 3.4.1 product behavior while avoiding a separate search system or n-gram table. If two-character substring traffic becomes expensive, the product should revisit either the minimum length for substring matching or a dedicated n-gram/search-index strategy.

### Sorting and pagination cost

The final ranking order is computed, not naturally stored in one index. PostgreSQL will use text indexes to find candidates, then sort candidates by:

```sql
rank_tier ASC, username ASC, user_id ASC
```

This is acceptable for an initial user-discovery feature because the text predicates should substantially narrow the candidate set before sorting. It is also necessary to preserve deterministic page/page_size pagination without inventing cursor semantics in this step.

### Growth path

PostgreSQL remains the right initial storage/query engine while Search covers two short fields and no relationship state. A dedicated search service or denormalized Search-owned index table becomes appropriate only if one or more of these conditions appears in measured production behavior:

- Search expands to many fields or long-form text (`bio`, watch history, interests, etc.).
- Ranking needs fuzzy matching, typo tolerance, personalization, popularity, or language-aware relevance.
- Two-character substring traffic becomes large enough that trigram indexes are not selective enough.
- Cross-module visibility rules require complex per-user filtering that cannot be expressed efficiently in SQL.
- Search traffic becomes operationally isolated enough to require separate scaling or caching.

No such requirement exists for Phase 3.4.2.

## 13. Module Boundaries

| Layer / module | Responsibility |
|---|---|
| Database schema | Store source-of-truth User/Profile data; provide indexes and extension support for efficient Search predicates. |
| Search repository (future) | Build SQL using normalized query parameters, apply eligibility/self-exclusion filters, compute rank tiers, join `users` and `profiles`, return page results and total count. |
| Search service (future) | Validate query length, normalize whitespace/case, require authenticated actor, call repository, map domain errors. |
| Auth / Users | Own `users.id`, `users.username`, `users.status`, and `users.deleted_at`; enforce account lifecycle semantics. |
| Profiles | Own `profiles.display_name` and `profiles.avatar_url`; enforce Profile data validation and lifecycle through User ownership. |
| Friends / Friend Requests | No initial Search database dependency because `relationship_state` is deferred. |
| Privacy / Blocking (future) | Define policy-based discoverability once those modules exist. Search indexes do not encode privacy or block state. |

Search queries existing data; it does not become the owner of User or Profile state.

## 14. Explicitly Excluded Design Choices

This design does not introduce:

- A `search_users`, `user_search`, or similar table.
- A materialized view.
- A generated `tsvector` column.
- Full-text search over `bio`.
- Email search.
- Profile ID lookup or result exposure.
- Relationship-state joins or indexes.
- Search-specific rate-limit tables.
- An external search database or search engine.
- New User/Profile columns.
- New User/Profile constraints beyond additive indexes.

## 15. Open Decisions

No unresolved database/indexing decisions remain for Step 3.4.2.

The actual migration implementation step must still decide whether to use normal `CREATE INDEX` or `CREATE INDEX CONCURRENTLY` based on the deployment environment and migration-runner constraints.