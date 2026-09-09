---
title: Friend Requests Database Schema Design
description: Persistence model and PostgreSQL schema blueprint for directed Friend Request workflows in the StreamTogether social graph.
ms.date: 2026-09-02
status: draft
phase: 3 - Social Graph
step: 3.3.2
depends-on: friends-requests-domain-design.md, friends-domain-design.md, profile-database-schema.md
---

# Friend Requests Database Schema Design

**Document status:** Draft  
**Based on:** `docs/features/friends-requests-domain-design.md`  
**Target database:** PostgreSQL 15+  
**Migration tool:** `golang-migrate/migrate` (file-based, `internal/database/migrations/postgres/`)  
**Expected migration number:** `000016`

## Table of Contents

1. [Purpose](#1-purpose)
2. [Conventions](#2-conventions)
3. [Table Definition](#3-table-definition)
4. [Column Definitions](#4-column-definitions)
5. [Primary Key](#5-primary-key)
6. [Foreign Keys and Delete Behavior](#6-foreign-keys-and-delete-behavior)
7. [CHECK Constraints](#7-check-constraints)
8. [Pending-Request Uniqueness](#8-pending-request-uniqueness)
9. [Indexes](#9-indexes)
10. [Timestamp Behavior](#10-timestamp-behavior)
11. [Supported Query Patterns](#11-supported-query-patterns)
12. [Concurrency and Integrity](#12-concurrency-and-integrity)
13. [Domain-to-Schema Mapping](#13-domain-to-schema-mapping)
14. [Explicitly Excluded Fields](#14-explicitly-excluded-fields)
15. [Migration Identification](#15-migration-identification)
16. [Open Decisions](#16-open-decisions)

---

## 1. Purpose

The `friend_requests` table stores directed proposals to establish a mutual Friendship:

```text
requester_user_id -> recipient_user_id
```

Unlike `friendships`, a Friend Request is not an unordered or canonical pair. `A -> B` and `B -> A` are distinct records at the persistence level. The future service resolves any reverse-direction policy and creates the actual mutual Friendship through the Friends domain.

The table retains the Friend Request entity's workflow data only. It does not duplicate the Friends canonical pair, Profile presentation data, User status, block/privacy policy, notifications, Search, or Presence.

## 2. Conventions

The schema follows the established PostgreSQL conventions in migrations `000001`, `000013`, `000014`, and `000015`.

| Element | Convention | Friend Requests application |
|---|---|---|
| Table name | Plural `snake_case` | `friend_requests` |
| Primary key | `id UUID NOT NULL DEFAULT gen_random_uuid()` | `pk_friend_requests` |
| Foreign key column | Descriptive singular `snake_case` ending in `_id` | `requester_user_id`, `recipient_user_id` |
| Timestamps | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` | `created_at`, `updated_at` |
| Nullable lifecycle timestamp | `TIMESTAMPTZ NULL` | `responded_at` |
| Primary key constraint | `pk_{table}` | `pk_friend_requests` |
| Foreign key constraint | `fk_{table}_{column}` | See section 6 |
| Check constraint | `ck_{table}_{description}` | See section 7 |
| Unique index/constraint | `uq_{table}_{description}` | `uq_friend_requests_pending_direction` |
| Explicit index | `idx_{table}_{columns}` | See section 9 |
| Updated timestamp trigger | `trg_{table}_updated_at`, shared `fn_set_updated_at()` | `trg_friend_requests_updated_at` |

`pgcrypto`, `gen_random_uuid()`, and `fn_set_updated_at()` already exist from migration `000001_create_users`. No extension, new PostgreSQL enum, or additional trigger function is introduced.

`status` is `TEXT` with a named CHECK constraint rather than a new enum. The project uses the shared `user_status` enum for the foundational User lifecycle, but Friend Request status is a module-local workflow with no cross-table use. A checked text value avoids expanding the global type surface while still preventing invalid persisted values.

## 3. Table Definition

The following is the authoritative blueprint for the future migration; it is not a migration file.

```sql
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

CREATE UNIQUE INDEX uq_friend_requests_pending_direction
    ON friend_requests (requester_user_id, recipient_user_id)
    WHERE status = 'pending';

CREATE INDEX idx_friend_requests_incoming_pending_created_at_id
    ON friend_requests (recipient_user_id, created_at DESC, id DESC)
    WHERE status = 'pending';

CREATE INDEX idx_friend_requests_outgoing_pending_created_at_id
    ON friend_requests (requester_user_id, created_at DESC, id DESC)
    WHERE status = 'pending';

CREATE TRIGGER trg_friend_requests_updated_at
    BEFORE UPDATE ON friend_requests
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
```

## 4. Column Definitions

| Column | Type | Nullability | Default | Mutability | Purpose |
|---|---|---|---|---|---|
| `id` | `UUID` | `NOT NULL` | `gen_random_uuid()` | Immutable | Surrogate primary key for the request. |
| `requester_user_id` | `UUID` | `NOT NULL` | None | Immutable | Directed source: User who sent the request. References `users(id)`. |
| `recipient_user_id` | `UUID` | `NOT NULL` | None | Immutable | Directed target: User authorized to accept or reject. References `users(id)`. |
| `status` | `TEXT` | `NOT NULL` | `'pending'` | Forward-only | Current lifecycle value: `pending`, `accepted`, `rejected`, or `cancelled`. |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL` | `NOW()` | Immutable | Database-generated request creation time. |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL` | `NOW()` | Database-managed | Latest row update; maintained by the shared trigger. |
| `responded_at` | `TIMESTAMPTZ` | `NULL` | None | Set once for recipient response | Timestamp when recipient accepts or rejects; null for pending and cancelled requests. |

The default `pending` status permits an insert to omit the column while preserving the entity's initial lifecycle state. The future service must explicitly set `responded_at` on accepted/rejected transitions and must never mutate endpoints or `created_at`.

## 5. Primary Key

`pk_friend_requests PRIMARY KEY (id)` provides the UUID identity required by the domain entity and follows every current project table. PostgreSQL automatically creates its B-tree index, which supports request retrieval by ID and row locking for transitions.

The database uses the established `gen_random_uuid()` default. Application-assigned UUIDs remain compatible, but no alternative UUID mechanism is required.

## 6. Foreign Keys and Delete Behavior

| Constraint | Reference | Delete behavior | Rationale |
|---|---|---|---|
| `fk_friend_requests_requester_user_id` | `requester_user_id -> users(id)` | `ON DELETE CASCADE` | A request cannot exist without its requester. |
| `fk_friend_requests_recipient_user_id` | `recipient_user_id -> users(id)` | `ON DELETE CASCADE` | A request cannot exist without its recipient. |

Soft deletion does not change or remove `friend_requests` rows because the `users` row remains during the grace period. The service determines eligibility and list visibility from `users.status` and `deleted_at`; account state is deliberately not denormalized here.

On hard deletion, both foreign keys ensure PostgreSQL removes every request in either direction involving that User. This matches `profiles` and `friendships` and preserves no dangling social relationship rows.

## 7. CHECK Constraints

| Constraint | Expression | Guarantee |
|---|---|---|
| `ck_friend_requests_distinct_users` | `requester_user_id <> recipient_user_id` | A User cannot send a request to themselves. |
| `ck_friend_requests_valid_status` | `status IN ('pending', 'accepted', 'rejected', 'cancelled')` | Only the four domain-defined lifecycle values can persist. |
| `ck_friend_requests_responded_at_status` | `responded_at` is non-null only for `accepted`/`rejected` and required for those statuses | Recipient-response timestamps reflect the entity semantics. |

The schema intentionally does not encode requester-versus-recipient authorization, active-account eligibility, pre-existing Friendship checks, forward-only transitions, or the full request state machine. Those require the actor, current relationship data, and transactional workflow context, and remain service responsibilities.

The `responded_at` check encodes a field-consistency invariant, not transition order. PostgreSQL cannot infer which actor changed a row, whether a Friendship was created, or whether a terminal status was reached from `pending`; the service enforces those rules with conditional updates and transaction handling.

## 8. Pending-Request Uniqueness

`uq_friend_requests_pending_direction` is a partial unique B-tree index:

```sql
UNIQUE (requester_user_id, recipient_user_id)
WHERE status = 'pending'
```

It enforces at most one simultaneous pending request for the exact directed pair. It does not apply unordered-pair uniqueness: `A -> B` and `B -> A` remain separate rows, as required by the directed domain model.

A full-table `UNIQUE (requester_user_id, recipient_user_id)` constraint is deliberately not used. The domain design's recommended default retains terminal history and allows a later request after a rejected or cancelled request. A full unique constraint would make that valid recreation impossible.

The partial unique index is both the database integrity guarantee for duplicate sends and the direct lookup path for a current directed pending request. The future service must map duplicate-key failures to the Friend Requests duplicate-pending error.

## 9. Indexes

The primary key and pending-direction unique index are supplemented by two pending-list indexes. All list indexes use the established stable ordering `created_at DESC, id DESC` and are intentionally partial because pending queues are the defined operational read model.

| Index | Definition | Query/access pattern |
|---|---|---|
| B-tree on `id` | Provided by `pk_friend_requests` | Find and lock a request by ID for accept, reject, or cancel. |
| `uq_friend_requests_pending_direction` | `(requester_user_id, recipient_user_id) WHERE status = 'pending'` | Duplicate send prevention and direct pending `requester -> recipient` lookup. |
| `idx_friend_requests_incoming_pending_created_at_id` | `(recipient_user_id, created_at DESC, id DESC) WHERE status = 'pending'` | Incoming pending requests by recipient, page/offset pagination, newest first. Also helps recipient-side FK cascade lookup for pending rows. |
| `idx_friend_requests_outgoing_pending_created_at_id` | `(requester_user_id, created_at DESC, id DESC) WHERE status = 'pending'` | Outgoing pending requests by requester, page/offset pagination, newest first. Also helps requester-side FK cascade lookup for pending rows. |

### Deliberately Excluded Indexes

| Index | Reason not created |
|---|---|
| Unordered-pair unique index | Would contradict directed persistence and preempt the unresolved reverse-direction product policy. |
| Full-table `(requester_user_id, recipient_user_id)` index | Pending-direction unique index serves the defined conflict lookup; terminal-history lookup is not a defined initial path. |
| Full-table `(recipient_user_id, created_at, id)` or `(requester_user_id, created_at, id)` indexes | Initial lists are pending queues; partial indexes are smaller and target the documented queries. Add history indexes only if OD-02 creates a history query requirement. |
| Single-column requester/recipient indexes | Redundant for pending operational reads because each partial composite index begins with the needed endpoint. |
| Standalone `status` index | Low-selectivity and unsupported by a defined query without an endpoint filter. |
| `(created_at, id)` index | Does not filter by requester or recipient. |
| Profile, friendship, block, privacy, notification, search, or presence indexes | Those columns and ownership concerns do not belong in this table. |

The partial endpoint indexes do not cover terminal rows during hard-delete cascades. PostgreSQL can still enforce the foreign keys correctly; a full endpoint index should be added only if measured hard-delete or terminal-history access makes it necessary.

## 10. Timestamp Behavior

| Column | Write behavior | Source |
|---|---|---|
| `created_at` | Set once on INSERT and never changed. | `DEFAULT NOW()` |
| `updated_at` | Set on INSERT and updated for every SQL `UPDATE`, including each status transition. | `DEFAULT NOW()` and `trg_friend_requests_updated_at` using shared `fn_set_updated_at()` |
| `responded_at` | Set by the service only when recipient accepts/rejects; immutable after settlement. | Application transition write, structurally guarded by CHECK constraint |

The trigger convention matches `users`, `oauth_identities`, and `profiles`. Application code must not attempt to manage `updated_at`.

## 11. Supported Query Patterns

| Query pattern | Canonical form | Supporting index or constraint |
|---|---|---|
| Find request for accept/reject/cancel | `WHERE id = :request_id` | `pk_friend_requests` |
| Same-direction pending duplicate check | `WHERE requester_user_id = :requester AND recipient_user_id = :recipient AND status = 'pending'` | `uq_friend_requests_pending_direction` |
| Incoming pending list | `WHERE recipient_user_id = :user_id AND status = 'pending' ORDER BY created_at DESC, id DESC LIMIT/OFFSET` | `idx_friend_requests_incoming_pending_created_at_id` |
| Outgoing pending list | `WHERE requester_user_id = :user_id AND status = 'pending' ORDER BY created_at DESC, id DESC LIMIT/OFFSET` | `idx_friend_requests_outgoing_pending_created_at_id` |
| Incoming/outgoing pending count | Same endpoint/status predicate without ordering | Corresponding partial endpoint index |
| Reverse-direction pending check | Query the directed pending lookup with swapped IDs, or query both directions. | `uq_friend_requests_pending_direction`; pair-level policy remains in the service |
| User hard deletion | Delete dependent rows involving either endpoint. | Both foreign keys; endpoint indexes accelerate pending rows |

The repository's established pagination contract is page/offset based (`Page`, `PageSize`, `LIMIT`, `OFFSET`, and `PageMeta`), with a default page size of 20 and maximum of 100. This schema intentionally introduces no cursor storage, cursor index, or cursor-specific query contract.

## 12. Concurrency and Integrity

### Database-Enforced Guarantees

- `ck_friend_requests_distinct_users` rejects self-requests.
- `ck_friend_requests_valid_status` rejects values outside the defined status set.
- `ck_friend_requests_responded_at_status` rejects timestamp/status combinations inconsistent with recipient response semantics.
- `uq_friend_requests_pending_direction` makes concurrent same-direction sends result in at most one pending row.
- User foreign keys reject unknown User IDs and cascade only when a User is hard-deleted.

### Service and Transaction Responsibilities

The database does not decide whether accounts are active, whether the actor is authorized, whether an established Friendship already exists, or how opposite-direction requests should resolve. The service must check those rules and use a transaction/locking strategy to serialize pair-level reverse-request decisions.

For accept, reject, and cancel, the service must conditionally transition only a pending row, either with a row lock or `UPDATE ... WHERE id = :id AND status = 'pending'`. This ensures one terminal action wins when concurrent operations target the same request.

Acceptance must share one database transaction with creation of the canonical Friendship. The Friend Requests table does not and must not create another friendship representation. The Friendship unique constraint remains the final protection against duplicate mutual relationships.

## 13. Domain-to-Schema Mapping

| Domain invariant or requirement | Database mechanism | Service/transaction responsibility that remains |
|---|---|---|
| Request has stable identity | `pk_friend_requests` UUID primary key | Generate/marshal identifiers at application boundaries. |
| Request direction matters | Separate immutable `requester_user_id` and `recipient_user_id` columns | Interpret actor authority and reverse-direction policy. |
| Requester and recipient differ | `ck_friend_requests_distinct_users` | Return a clear self-request error. |
| User endpoints exist | Two FKs to `users(id)` | Require active, non-soft-deleted endpoints. |
| One pending request per direction | `uq_friend_requests_pending_direction` partial unique index | Detect/map duplicate errors and resolve retry behavior. |
| Valid status values only | `ck_friend_requests_valid_status` | Enforce permitted forward transitions and actor-specific actions. |
| Recipient responses are timestamped | `responded_at` plus `ck_friend_requests_responded_at_status` | Set it once, at the valid accepted/rejected transition. |
| Request timestamps are reliable | `DEFAULT NOW()` and shared updated-at trigger | Do not override audit semantics. |
| Hard deletion removes dependent requests | `ON DELETE CASCADE` on both FKs | Soft-delete eligibility and visibility policy. |
| A Friendship is created only after acceptance | No Friendship column or duplicate pair representation | Execute Friends establishment and request settlement atomically. |
| Reverse requests are directionally distinct | No unordered-pair constraint | Make and serialize the product-policy decision. |

## 14. Explicitly Excluded Fields

The initial `friend_requests` table contains only `id`, `requester_user_id`, `recipient_user_id`, `status`, `created_at`, `updated_at`, and `responded_at`. It must not contain:

- Friendship IDs, canonical Friendship endpoints, or a second Friendship representation
- Profile IDs or snapshot attributes such as username, display name, avatar URL, email, or bio
- User status, `deleted_at`, soft-delete flags, suspension flags, or account lifecycle data
- Request text, notes, reasons, or moderation metadata
- Block, Privacy, notification, Search, Presence, or User Preference state
- `accepted_at`, `rejected_at`, or `cancelled_at` columns; `responded_at` and `updated_at` already express the domain-defined timestamps
- Expiry state, `expires_at`, cooldown, or retention timestamps before a product decision adopts them
- Denormalized request/friend counters or aggregate data

## 15. Migration Identification

The latest existing migration is `000015_create_friendships`. The expected next migration is therefore:

```text
000016_create_friend_requests.up.sql
000016_create_friend_requests.down.sql
```

Those files belong to Step 3.3.3 and are intentionally not created by this design step. The eventual migration must:

1. Create `friend_requests` with the named constraints in section 3.
2. Create the partial unique index and two pending-list indexes in section 9.
3. Attach `trg_friend_requests_updated_at` to the shared `fn_set_updated_at()` function.
4. Depend on `000001_create_users` for `users`, `pgcrypto`, `gen_random_uuid()`, and `fn_set_updated_at()`.
5. Provide a rollback that drops the Friend Requests trigger, indexes, and table only, without altering shared authentication or Friends infrastructure.

## 16. Open Decisions

| ID | Question | Impact on schema |
|---|---|---|
| OD-01 | When simultaneous or reverse pending requests exist, should the product automatically establish a Friendship, retain one deterministic request, or return a conflict requiring recipient action? | No unordered-pair constraint is added. The service needs a pair-level transaction/locking strategy; a later policy may change service behavior without changing directed storage. |
| OD-02 | Are terminal requests retained permanently, retained for a bounded period, or deleted after settlement? | The proposed partial unique index supports retained history and recreation after terminal states. Retention jobs or history-query indexes should be added only after a decision. |
| OD-03 | May a user recreate a request after rejection/cancellation, and is a cooldown required? | The schema permits recreation because uniqueness applies only to `pending`; a cooldown would require policy plus possibly a query/index or a new column if it cannot be derived from history. |
| OD-04 | If a Friendship exists during acceptance because of a concurrent/manual establishment, is settlement idempotent or a conflict? | No schema change. The answer determines the transaction's conditional update/error handling. |
| OD-05 | Should pending requests expire automatically? | No expiry column or status is included. Adoption requires a defined terminal/retention policy and additive schema design. |
| OD-06 | Which future Blocking and Privacy rules affect request creation, receipt, acceptance, and visibility? | No block/privacy columns or constraints are included; the future service integration determines policy checks. |