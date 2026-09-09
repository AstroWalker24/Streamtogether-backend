---
title: Search Domain Design
description: Domain design for authenticated user-discovery search over usernames and display names in the StreamTogether social graph.
ms.date: 2026-09-09
status: draft
phase: 3 — Social Graph
step: 3.4.1
depends-on: profile-domain-design.md, friends-domain-design.md, friends-requests-domain-design.md, authentication-domain-design.md, authentication-domain-contracts.md
---

# Search Domain Design

## Table of Contents

1. [Domain Purpose](#1-domain-purpose)
2. [Search Target](#2-search-target)
3. [Search Query and Normalization](#3-search-query-and-normalization)
4. [Search Result Read Model](#4-search-result-read-model)
5. [Matching and Ranking Behavior](#5-matching-and-ranking-behavior)
6. [Pagination and Deterministic Ordering](#6-pagination-and-deterministic-ordering)
7. [Account Visibility and Eligibility](#7-account-visibility-and-eligibility)
8. [Privacy Boundary](#8-privacy-boundary)
9. [Relationship Information (Derived)](#9-relationship-information-derived)
10. [Authorization](#10-authorization)
11. [Domain Errors](#11-domain-errors)
12. [Module Boundaries](#12-module-boundaries)
13. [Future Persistence Considerations](#13-future-persistence-considerations)
14. [Open Decisions](#14-open-decisions)

---

## 1. Domain Purpose

The Search domain answers one question: _"Which discoverable users match what this authenticated user typed?"_

Search is a **read-only discovery capability** over identity data owned by other modules. It lets an authenticated user find other users by username or display name so they can initiate a future social action (send a friend request, view a profile, invite to a party). Search does not perform that action itself.

| Concept | Meaning | Owner |
|---|---|---|
| Searchable identity | The subset of `username` (auth.User) and `display_name` (profile.Profile) used for matching. | Search reads it; Auth/Profile own it. |
| Match | A ranked candidate produced by comparing a normalized query against searchable identity. | Search |
| Discoverability | Whether a matched user is actually allowed to appear in someone else's results. | Search decides eligibility (account state); Privacy (future) decides policy-based visibility. |
| Relationship state | Whether the searcher and a result are already friends or have a pending request. | Excluded from the initial Search release; Friends / Friend Requests own the data. |

Search introduces no new identity, relationship, or presentation data. It contains no entity that is persisted as the source of truth for anything — its "entity" is a query/result contract, not a stored aggregate.

## 2. Search Target

### What is searchable

Search matches against exactly two fields, both already owned by other modules:

| Field | Owner | Rationale |
|---|---|---|
| `username` | `auth.User` | Canonical, unique, case-insensitive handle. Every user has exactly one. |
| `display_name` | `profile.Profile` | User-chosen presentational name shown in social contexts; may be `nil`. |

`bio` is explicitly **not searchable**. `profile-domain-design.md` (§17, OD-6) left bio search as an open decision for this module to resolve; this document resolves it: bio is excluded from matching in the initial Search design. A short free-text bio is intended for self-description, not as a discovery key, and indexing it would surface unrelated matches on common words. This can be revisited only as an explicit future product decision, not silently added later.

`email` is never searched and never appears in a result. It is an authentication credential (see `profile-domain-design.md` §1) and is out of scope for discovery entirely.

### Exact and partial matches

Both are supported:

- **Exact match** — the normalized query equals the normalized `username` or `display_name`.
- **Partial match** — the normalized query is a substring of the normalized `username` or `display_name`, with a prefix match treated as a stronger partial match than a mid-string (substring) match.

This matches the intent already recorded in `profile-database-schema.md` §9, which reserves a future `pg_trgm` index on `display_name` for the Search module — a mechanism that supports substring and similarity search, not prefix-only lookup.

### Case-sensitivity

Matching is **case-insensitive** for both fields, consistent with `username` already being a globally unique, case-insensitive identifier (`authentication-domain-design.md` §4.1, BR-02). `display_name` has no uniqueness constraint and preserves case as entered for display, but is casefolded for the purposes of matching only. Case-folding never mutates stored data; it is applied identically to the query and to the compared field at match time.

## 3. Search Query and Normalization

A future `SearchQuery` value object (not a persisted entity) carries:

```text
SearchQuery
├── raw_query   string                — exactly as submitted by the client
├── query       string                — normalized internal comparison key
└── pagination  repository.Pagination — existing shared page/page_size convention
```

Normalization applied before matching, in order:

1. Trim leading and trailing whitespace.
2. Collapse internal runs of whitespace to a single space (a query is compared as a name-like string, matching the `display_name` normalization convention in `profile-domain-design.md` §5).
3. Case-fold (Unicode-aware lowercasing) for comparison purposes only.

The normalized query is never persisted and never echoed back with different casing than submitted — it is an internal comparison key only.

Accent and diacritic folding are not applied initially. For example, `cafe` does not automatically match `café` unless a later product requirement explicitly adopts locale-aware accent folding.

### Query length

A query shorter than **2 characters after normalization** is rejected as a domain error rather than executed. This mirrors the minimum length already adopted for `display_name` (`profile-domain-design.md` §5) and prevents pathologically broad matches (e.g., a single common letter) that would return large, low-relevance result sets.

A query longer than **100 characters after normalization** is also rejected as a domain error. This provides a clear domain/API boundary even though general request-size middleware still protects the transport layer.

### Empty query behavior

An empty or whitespace-only query is invalid input, not a request to browse all users. Search in this design is strictly **query-driven discovery**; it is not a user directory or "browse all" listing endpoint. Returning the entire user base (or a default/trending subset) on an empty query is a distinct product feature that this document does not adopt.

## 4. Search Result Read Model

Search returns a projection, not an entity. It duplicates no field that another module already owns; it composes.

```text
SearchResultItem
├── user_id       UUID    — from auth.User; used for correlation and future actions
├── username      string  — from auth.User
├── display_name  string? — from profile.Profile; nil falls back to username on the client
└── avatar_url    string? — from profile.Profile; nil renders a default placeholder
```

This is deliberately the same shape as `profile.ProfileSummary` (`profile-domain-design.md` §12.3). Search does not define a competing "public user summary" type; it composes the existing profile summary fields needed for user discovery.

### Fields deliberately excluded from the result

| Field | Reason excluded |
|---|---|
| `email` | Authentication credential; never exposed in a public-facing view. |
| `bio` | Not part of the search identity (§2); also not required for a discovery list item. |
| `relationship_state` | Excluded from the initial release to keep Search focused on matching users and avoid coupling every result page to Friends and Friend Requests lookups (§9). |
| Account status, timestamps (`created_at`, `updated_at`, `deleted_at`) | Internal lifecycle data with no discovery purpose; consistent with Profile's own "Internal" classification of these fields (`profile-domain-design.md` §11). |
| Profile `id` | Internal correlation identifier; `user_id` is the public correlation key, consistent with `profile-domain-design.md` §14. |

## 5. Matching and Ranking Behavior

### Match tiers

Results are grouped into discrete relevance tiers, evaluated top to bottom:

| Tier | Condition |
|---|---|
| 1 | Normalized query equals normalized `username` (exact). |
| 2 | Normalized query equals normalized `display_name` (exact). |
| 3 | Normalized `username` starts with the normalized query (prefix). |
| 4 | Normalized `display_name` starts with the normalized query (prefix). |
| 5 | Normalized `username` contains the normalized query (substring, not prefix). |
| 6 | Normalized `display_name` contains the normalized query (substring, not prefix). |

`username` is ranked above `display_name` within an equivalent match strength because it is the unique, unambiguous identifier; a user searching for a known handle should see it first.

### Deterministic tie-breaking

Within a tier, results are ordered by `username ASC` and then `user_id ASC` as a final tiebreaker. `username` alone is already unique, so `user_id` only exists to make the ordering contract explicit and self-documenting — no two rows can actually tie on `username`. This matches the project's established pattern of a compound, fully deterministic sort (e.g., `friends-domain-design.md` §9's `CreatedAt DESC, ID DESC`), which guarantees reproducible ordering across paginated requests.

No relevance scoring beyond the tier number is defined in this design (no fuzzy-match distance, no popularity/engagement weighting). Tier + deterministic tiebreaker is the entire ranking contract for the initial implementation.

## 6. Pagination and Deterministic Ordering

Search reuses the existing shared `repository.Pagination` / `repository.PageMeta` page/offset convention (`internal/repository/pagination.go`) — the same convention adopted by Friend Requests lists (`friends-requests-domain-design.md` §9) — rather than introducing cursor pagination. `Pagination.Normalize()` already clamps out-of-range `page`/`page_size` values to defaults instead of erroring; Search does not override that behavior (see §11).

Because ranking is tiered rather than continuously scored, pagination must be computed over the **fully ordered** result set (tier, then `username ASC`, then `user_id ASC`), not per-tier, so that page boundaries are stable and do not skip or duplicate a result between requests.

## 7. Account Visibility and Eligibility

Search follows the same eligibility rule already established for Friends and Friend Requests: only users who could plausibly participate in a social interaction are discoverable.

| Account state | Discoverable in search results |
|---|---|
| `active` and not soft-deleted | Yes |
| `pending_verification` | No |
| `suspended` | No |
| `deleted` / soft-deleted (grace period) | No |
| Hard-deleted | No (no `User` record exists) |

This matches the eligibility table in `friends-requests-domain-design.md` §7 exactly: the same account states that are ineligible to send, receive, or appear in a Friend Request are ineligible to appear in a search result. A restored account (`active` again) becomes discoverable again without any Search-owned state change, because Search stores no state of its own.

The searching user's own account state is governed by authentication middleware (§10), not by this table.

### Self-exclusion

A user's own record is excluded from their own search results. A user cannot friend themselves (`friends-domain-design.md` §4), so surfacing the searcher's own identity as a discoverable "other user" serves no purpose and would need special-cased relationship state (§9) for no benefit.

## 8. Privacy Boundary

Search and the future Privacy module (Phase 3.7) divide responsibility along a clear seam:

| Question | Owner |
|---|---|
| Does this user's `username`/`display_name` match the query text? | Search |
| Is this user's account in an eligible, non-hidden state (§7)? | Search |
| Should this specific user be discoverable to *this* specific searcher at all, irrespective of text matching (e.g., "search visibility" opted out, or a future block relationship)? | Privacy (and, later, Blocking) |

Until Privacy exists, Search treats every account-eligible user (§7) as discoverable to any authenticated searcher — there is no interim opt-out. Search must not implement a privacy flag, a "searchable" toggle, or block-awareness itself; per the pattern already set by Friends and Friend Requests (`friends-domain-design.md` §11, `friends-requests-domain-design.md` §14), it must instead expose a seam — a filtering step applied to the candidate result set — that a future Privacy (and Blocking) integration can occupy without changing how Search matches or ranks text.

This mirrors the Profile domain's own privacy boundary: Profile/Search decide *what* the data is; Privacy decides *whether it may be shown* (`profile-domain-design.md` §11).

## 9. Relationship Information (Excluded Initially)

The initial Search release does **not** include relationship state in `SearchResultItem`.

Search should primarily answer:

```text
Which users match this query?
```

Adding friendship or pending-request state would make every result page depend on Friends and Friend Requests. A naive implementation would require additional per-result lookups, creating coupling and an N+1 query risk. Therefore, the initial result does not include any of the following:

```text
├── none                — no Friendship and no pending Friend Request in either direction
├── friends              — an established Friendship exists (Friends.AreFriends)
├── request_outgoing     — the searcher has a pending request sent to this result
└── request_incoming     — the searcher has a pending, unaccepted request from this result
```

Relationship state remains a valid future enhancement if product experience requires it. If added later, it must be **read-only derived information**, resolved with an optimized batch strategy against Friends and Friend Requests. Search must never store or duplicate a friendship or request record, and adding relationship state must not change Friends' or Friend Requests' ownership of the underlying data (`friends-domain-design.md` §1, `friends-requests-domain-design.md` §1).

## 10. Authorization

Search requires an authenticated user. There is no anonymous/unauthenticated search endpoint in this design, consistent with Profile's own default posture ("profile fields should be treated as visible only to authenticated users" — `profile-domain-design.md` §11) and with the enumeration-hardening requirement in `profile-domain-design.md` §14.

The authenticated identity is used for self-exclusion (§7): the searcher's own `user_id` is removed from their own results.

If relationship state is added in a future phase (§9), the authenticated user's `user_id` will also provide the perspective from which that derived state is computed.

No new role or permission is introduced. Existing RBAC remains the mechanism for any future administrative search capability (e.g., an admin "search all users regardless of state" tool referenced in `01-product-requirements.md`'s Administration Module); that is a distinct, out-of-scope capability from this authenticated-user-facing Search design.

## 11. Domain Errors

Search must extend the existing global `AppError` architecture with a Search-specific error package, following the pattern established by `internal/friends/errors` and `internal/profile/errors`. These are conceptual errors, not final code names or payloads.

| Error concept | Condition | Suggested classification |
|---|---|---|
| `SEARCH_QUERY_TOO_SHORT` | Normalized query is empty or below the minimum length (§3). | Bad request / validation |
| `SEARCH_QUERY_TOO_LONG` | Normalized query exceeds 100 characters (§3). | Bad request / validation |

No separate "invalid pagination" error is introduced. Consistent with the existing `repository.Pagination.Normalize()` convention (already relied on by Friend Requests lists), out-of-range `page`/`page_size` values are silently clamped to valid defaults rather than rejected.

No "user not found" or "no results" error exists. An empty match set is a normal, successful response (`success: true`, empty `data` array), not a domain error — consistent with how list endpoints behave elsewhere in the project.

An "inaccessible/non-discoverable user" is never surfaced as an error either; such users are simply absent from the result set (§7, §8). Search never reveals *that* a matching-but-ineligible user exists.

## 12. Module Boundaries

| Module | Relationship to Search | Search responsibility |
|---|---|---|
| Users / Auth | Owns `username` and account lifecycle state. | Read `username` and account state for matching and eligibility; never create, update, or delete a `User`. |
| Profiles | Owns `display_name` and `avatar_url`. | Read these fields for matching/display only; never create, update, or delete a `Profile`; never introduce a competing presentation field. |
| Friends | Owns established Friendships. | No initial Search dependency; if relationship state is added later, read only derived friendship state without creating or removing a Friendship. |
| Friend Requests | Owns directed request workflow. | No initial Search dependency; if relationship state is added later, read only derived request state without sending, accepting, rejecting, or cancelling a request. |
| Privacy (future) | Will own discoverability/visibility policy. | Provide a filtering seam (§8) for Privacy to occupy; implement no privacy flag or rule itself. |
| Blocking (future) | Will own unilateral interaction restriction. | Implement no block state or filtering logic until that module exists; the same seam as Privacy applies. |

### Explicitly, Search does NOT:

- Create Friendships or Friend Requests.
- Modify Profiles, Users, or privacy settings.
- Store a denormalized copy of `username`, `display_name`, `avatar_url`, or account status as its own source of truth.
- Return friendship or friend-request relationship state in the initial release.
- Decide policy-based discoverability (opt-out, blocking) — it only decides text matching and account-state eligibility.
- Provide a "browse all users" or directory capability (§3).

## 13. Future Persistence Considerations

No schema, migration, repository, or index is defined by this document. The following constraints are recorded so that the future database design step (3.4.2) has a stable domain contract to satisfy, without prescribing its mechanism here:

1. Matching must be efficient against both `username` (`auth.User`) and `display_name` (`profile.Profile`) without a full table scan at expected data volumes.
2. The eventual index strategy (a `pg_trgm` index, a generated `tsvector` column, or a separate denormalized search index/table) is an implementation choice deferred to 3.4.2, matching the option already reserved in `profile-database-schema.md` §9 and its OD-S1.
3. Whatever mechanism is chosen must support both prefix and substring matching (§5) and case-insensitive comparison (§2) to fulfill this domain design; it must not silently reduce Search to prefix-only matching for indexing convenience.
4. The eligibility filter (§7) must be applied at the query level (e.g., joined against `users.status` and `deleted_at`), not applied by filtering an already-paginated result in the application layer, to keep pagination counts and pages correct.
5. No new column is added to `users` or `profiles` by Search; if a denormalized search index/table is chosen, it is additive and owned by Search, not a mutation of the `users` or `profiles` schema.

## 14. Open Decisions

No unresolved Search domain decisions remain for Step 3.4.1.

The following product decisions are now adopted by this document:

| ID | Decision |
|---|---|
| OD-01 | Minimum normalized query length is 2 characters. |
| OD-02 | Empty or whitespace-only queries are rejected; no browse/suggested-users behavior is included. |
| OD-03 | `relationship_state` is excluded from the initial release. It may be added later only through an optimized batched design. |
| OD-04 | The tier ordering in §5 is the initial deterministic ranking policy. |
| OD-05 | Accent/diacritic folding is not applied; normalization is limited to whitespace handling and casefolding. |
| OD-06 | Maximum normalized query length is 100 characters. Search relies on existing middleware for rate limiting initially; no Search-specific limiter is introduced. |

No other open Search domain decisions remain for Step 3.4.1.
