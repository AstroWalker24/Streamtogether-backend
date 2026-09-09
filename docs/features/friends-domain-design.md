---
title: Friends Domain Design
description: Domain design for established, mutual friendships in the StreamTogether social graph.
ms.date: 2026-09-01
status: draft
phase: 3 — Social Graph
step: 3.2.1
depends-on: profile-domain-design.md, authentication-domain-design.md, authentication-domain-contracts.md
---

# Friends Domain Design

## Table of Contents

1. [Domain Purpose](#1-domain-purpose)
2. [Friendship Entity](#2-friendship-entity)
3. [Symmetry and Pair Canonicalization](#3-symmetry-and-pair-canonicalization)
4. [Invariants](#4-invariants)
5. [Lifecycle](#5-lifecycle)
6. [Conceptual Operations](#6-conceptual-operations)
7. [Ownership and Authorization](#7-ownership-and-authorization)
8. [User Lifecycle](#8-user-lifecycle)
9. [Read Model, Counts, Pagination, and Ordering](#9-read-model-counts-pagination-and-ordering)
10. [Domain Errors](#10-domain-errors)
11. [Module Boundaries](#11-module-boundaries)
12. [Future Database Requirements](#12-future-database-requirements)
13. [Concurrency](#13-concurrency)
14. [Open Decisions](#14-open-decisions)

---

## 1. Domain Purpose

The Friends domain represents an **established mutual social relationship** between exactly two distinct Users. It answers whether two users are friends; it does not represent any intermediate or directional social state.

If User A is friends with User B, then User B is friends with User A. Neither user is the owner, sender, recipient, follower, or followed user of the relationship.

| Concept | Meaning | Owner |
|---|---|---|
| Friendship | An existing, mutual relationship between two users. | Friends |
| Friend Request | A proposed relationship that has not yet been established, including its pending, accepted, rejected, or cancelled workflow. | Friend Requests (Phase 3.3) |
| Block | A unilateral restriction that affects an effective social relationship. | Blocking (Phase 3.6) |

Pending or rejected requests are never Friendship records. A block is not stored as a Friendship state and does not alter the definition of an established friendship.

## 2. Friendship Entity

`Friendship` belongs in `internal/friends/domain` when implementation begins. It is a pure Go entity with no serialization tags, database tags, or framework dependencies.

```
Friendship
├── id            UUID      — surrogate primary key
├── first_user_id UUID      — canonical first endpoint; immutable FK to auth.User
├── second_user_id UUID     — canonical second endpoint; immutable FK to auth.User
└── created_at    time.Time — immutable establishment timestamp
```

| Field | Type | Required | Purpose | Mutability | Lifecycle |
|---|---|---:|---|---|---|
| `ID` | `uuid.UUID` | Yes | System-assigned surrogate identifier, consistent with the project table convention. | Immutable | Set at creation; removed with the relationship. |
| `FirstUserID` | `uuid.UUID` | Yes | The lower endpoint in the canonical unordered pair. FK to `auth.User`. | Immutable | Set at creation only. |
| `SecondUserID` | `uuid.UUID` | Yes | The higher endpoint in the canonical unordered pair. FK to `auth.User`. | Immutable | Set at creation only. |
| `CreatedAt` | `time.Time` | Yes | Records when the friendship became established. Supports audit and chronological list order. | Immutable | Set at creation only. |

`updated_at` is intentionally omitted. A Friendship has no mutable domain fields: it exists until it is removed. A removal deletes the relationship rather than updating it. No status, request state, profile data, per-user metadata, or blocking/privacy flag belongs on this entity.

Friendships reference `auth.User` IDs, not Profile IDs. A User is the stable identity endpoint; Profile remains responsible for presentation data.

## 3. Symmetry and Pair Canonicalization

### Decision: One Canonical Relationship Record

The future persistence model must store **one record per unordered user pair**, not two directional records.

| Representation | Decision | Rationale |
|---|---|---|
| One canonical row: `A <-> B` | Adopted | Directly represents mutuality, prevents directional disagreement, minimizes storage, and permits one database uniqueness rule for the relationship. |
| Two rows: `A -> B` and `B -> A` | Rejected | Duplicates data and introduces partial-write and contradictory-state risks if only one row is created, removed, or corrupted. |

### Canonical Pair Invariant

User IDs must be deterministically ordered before a Friendship is created, queried for existence, or removed:

```
FirstUserID < SecondUserID
```

The ordering is the UUID ordering used by the future database comparison and must be applied consistently by the domain/service boundary. Therefore, inputs `(A, B)` and `(B, A)` normalize to the same persisted pair.

The names `FirstUserID` and `SecondUserID` deliberately avoid directional meanings such as `user_id` and `friend_user_id`.

## 4. Invariants

1. A Friendship has exactly two User endpoints.
2. The endpoints are distinct: `FirstUserID != SecondUserID`.
3. The stored pair is canonical: `FirstUserID < SecondUserID`.
4. At most one Friendship exists for a canonical pair.
5. A Friendship is mutual and has no direction.
6. A Friendship exists only while both referenced User records exist.
7. A Friendship contains no pending, rejected, cancelled, blocked, presence, privacy, or preference state.

The service/domain layer must reject self-friendship and canonicalize the pair before persistence. The future database schema must independently enforce the distinct-endpoint, canonical-order, and unique-pair invariants. Application checks improve errors; database constraints are the final integrity guarantee.

## 5. Lifecycle

```
[Created]
    |
    | friendship established
    v
[Active]
    |
    | either endpoint removes the relationship
    v
[Removed]
```

Creation makes the relationship immediately active. No explicit status field is needed because existence is the only active state. Removal is a hard deletion of the Friendship record; it removes the mutual relationship for both users at once. There is no one-sided unfriend state and no friendship history or tombstone in this initial design.

Friend Requests may cause creation after acceptance, but request lifecycle is outside this module.

## 6. Conceptual Operations

These are domain/service contract responsibilities for a future implementation, not API or repository definitions.

| Operation | Purpose | Required behavior |
|---|---|---|
| `EstablishFriendship(userA, userB)` | Creates an established friendship. | Rejects equal IDs, canonicalizes the pair, requires both users to be active and non-deleted, and creates one record. It does not send or accept a request. |
| `RemoveFriendship(actorUserID, otherUserID)` | Removes an established friendship. | Applies to the canonical relationship, so neither endpoint remains friends after success. |
| `AreFriends(userA, userB)` | Checks whether an established relationship exists. | Canonicalizes the pair; equal IDs returns `false` rather than treating a user as their own friend. |
| `GetFriends(userID, page)` | Obtains a user's established relationships. | Returns the user's friendship references/other User IDs and creation metadata, not Profile fields. |
| `CountFriends(userID)` | Counts established friendships for a user. | Counts relationships; no denormalized counter is required initially. |

`GetFriends` is a Friends query over relationship data. A higher application/query layer may resolve the returned User IDs into Profile/User projections, subject to Profile and future Privacy rules. The Friends entity must not duplicate `username`, `display_name`, `bio`, or `avatar_url`.

## 7. Ownership and Authorization

Friendship ownership is shared by its two endpoints. For ordinary user-initiated removal, the acting authenticated user must match either endpoint; a user cannot mutate a relationship involving two other users.

Authentication establishes the actor identity. The future Friends service enforces endpoint membership and business rules. Existing RBAC remains the authorization mechanism for any future administrative override; this design introduces no new Friends permission.

## 8. User Lifecycle

### Soft-Deleted and Hard-Deleted Users

The Friends domain follows the established `auth.User` lifecycle.

- During the soft-delete grace period, the User row remains and friendship records are preserved. The deleted account is ineligible for social operations and is not exposed through friend-list projections.
- On account restoration to `active`, preserved friendships become available again, subject to future Privacy rules.
- On hard deletion, every Friendship involving that User must be physically deleted by foreign-key cascade. This matches the existing dependent-data policy for Profiles and authentication records.

### Suspended Users

A suspended user remains in existing Friendship records. Suspension does not automatically remove friendships because suspension is an account lifecycle state, not a relationship mutation. Suspended users are ineligible to establish a new friendship and are hidden from friend-list results. The Friends record remains intact so reactivation to `active` restores the relationship without recreating it.

## 9. Read Model, Counts, Pagination, and Ordering

### Friend List

The initial Friends domain returns relationship references: the other User ID for a requested user, the Friendship ID, and `CreatedAt`. It does not return embedded User or Profile entities. A composite social view may join or resolve Profile data outside the Friendship entity.

### Friend Count

Friend count is a derived value: count active Friendship records containing the requested User ID. No `friend_count` column belongs on `users` or `profiles` initially.

### Pagination and Ordering

`GetFriends` must support pagination when implemented. Cursor pagination is appropriate because friend lists can grow and relationship rows are immutable. The stable default order is most recently established first:

```
CreatedAt DESC, ID DESC
```

The cursor must encode both values to make ordering deterministic when multiple friendships share a timestamp. Alphabetical or display-name ordering is a presentation/query concern because those values belong to auth/Profile and may change; it is not the default Friends domain order.

## 10. Domain Errors

The future Friends module must use the existing global `AppError` architecture and a domain-specific error package following the auth module pattern. It must not create a new error framework.

| Error | Condition | Suggested classification |
|---|---|---|
| `FRIENDS_CANNOT_FRIEND_SELF` | The two supplied user IDs are identical. | Bad request / validation failure |
| `FRIENDS_ALREADY_FRIENDS` | A canonical pair already has a Friendship. | Conflict |
| `FRIENDS_FRIENDSHIP_NOT_FOUND` | No Friendship exists for a requested removal or direct lookup. | Not found |
| Existing user/account errors | An endpoint is absent, soft-deleted, suspended, or otherwise ineligible. | Reuse the established auth/application error where applicable |

`FriendshipAlreadyExists` is not a separate public domain error from `AlreadyFriends`; both describe the same invariant violation and should resolve to one code.

## 11. Module Boundaries

| Module | Relationship to Friends | Friends responsibility |
|---|---|---|
| Profiles | Supplies social presentation for friend-list projections. | Store User IDs only; duplicate no profile attributes. |
| Friend Requests | Accepted requests may establish a Friendship. | Provide establishment capability only; do not send, accept, reject, cancel, or store pending requests. |
| Search | Can discover Users/Profiles to initiate future social actions. | Does not search or index users. |
| Presence | May show dynamic availability alongside friends. | Stores no online/offline or last-seen state. |
| Blocking | Can change the effective permitted interaction between two users. | Stores no block state and does not define block consequences. |
| Privacy | Governs whether friend lists or profiles may be viewed. | Represents the relationship only; enforces no privacy flags or visibility policy. |
| User Preferences | May configure social notifications or presentation behavior. | Stores no per-user preference data. |

## 12. Future Database Requirements

The future Friends schema must provide the following, without prescribing SQL or migration implementation here:

1. A UUID surrogate primary key for each Friendship, consistent with project conventions.
2. Two non-null UUID endpoint columns referencing `users(id)`.
3. Foreign keys with `ON DELETE CASCADE` so hard deletion of either User removes the Friendship.
4. A database check preventing equal endpoints.
5. A database check enforcing canonical endpoint order: `first_user_id < second_user_id`.
6. A unique constraint across the canonical endpoint pair, making `(A, B)` and `(B, A)` the same relationship.
7. A non-null `created_at` timestamp generated by the database in UTC-compatible `TIMESTAMPTZ` form.
8. Efficient lookup paths for friend lists from either endpoint and efficient canonical-pair existence checks.

No `updated_at`, `deleted_at`, status, directional flag, profile snapshot, or denormalized friend count is required for the initial schema.

## 13. Concurrency

Concurrent attempts to establish the same friendship must result in at most one row, including reversed inputs. For example, simultaneous attempts for `(A, B)` and `(B, A)` must normalize to the same pair and be protected by the database unique constraint.

The future service should translate the resulting duplicate-key failure into `FRIENDS_ALREADY_FRIENDS`. A pre-insert `AreFriends` check may improve the normal error path but cannot be relied on for correctness.

## 14. Open Decisions

No open Friends domain decisions remain for Step 3.2.1.