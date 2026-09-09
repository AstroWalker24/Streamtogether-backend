---
title: Friend Requests Domain Design
description: Domain design for directed friend-request workflows in the StreamTogether social graph.
ms.date: 2026-09-02
status: draft
phase: 3 - Social Graph
step: 3.3.1
depends-on: friends-domain-design.md, profile-domain-design.md, authentication-domain-design.md, authentication-domain-contracts.md
---

# Friend Requests Domain Design

## Table of Contents

1. [Domain Purpose](#1-domain-purpose)
2. [Friend Request Entity](#2-friend-request-entity)
3. [Status Lifecycle](#3-status-lifecycle)
4. [Invariants and Business Rules](#4-invariants-and-business-rules)
5. [Friendship Integration](#5-friendship-integration)
6. [Ownership and Authorization](#6-ownership-and-authorization)
7. [User Eligibility and Lifecycle](#7-user-eligibility-and-lifecycle)
8. [Request History and Retention](#8-request-history-and-retention)
9. [Read Model and Pagination](#9-read-model-and-pagination)
10. [Future Service Boundary](#10-future-service-boundary)
11. [Future Repository Boundary](#11-future-repository-boundary)
12. [Domain Errors](#12-domain-errors)
13. [Concurrency and Atomicity](#13-concurrency-and-atomicity)
14. [Module Boundaries](#14-module-boundaries)
15. [Future Database Requirements](#15-future-database-requirements)
16. [Open Decisions](#16-open-decisions)

---

## 1. Domain Purpose

The Friend Requests domain represents a proposed, directed social relationship:

```text
requester -> recipient
```

A request asks the recipient to establish a mutual Friendship with the requester. Direction is meaningful while a request is pending: the requester may cancel it, while the recipient may accept or reject it.

Friend Requests and Friendships are separate concepts and persistence models. A request is not a Friendship state, and a Friendship contains no request direction, status, or history.

| Concept | Meaning | Owner |
|---|---|---|
| Friend Request | A directed proposal to form a friendship, with a pending and terminal workflow. | Friend Requests |
| Friendship | An established, mutual relationship between two users. | Friends |
| Block | A unilateral restriction that may affect permitted social interaction. | Blocking (future) |
| Privacy setting | A rule governing discoverability, visibility, or who may request contact. | Privacy (future) |

The Friend Requests domain references `auth.User` IDs only. It does not own Profile data, User account state, Friendship persistence, blocking policy, privacy policy, notifications, search, or presence.

## 2. Friend Request Entity

When implemented, `FriendRequest` belongs in `internal/friends/requests/domain` or another package location chosen with the Phase 3.3 package design. It is a pure Go entity with no serialization tags, database tags, or framework dependencies.

```text
FriendRequest
├── id                UUID                - surrogate primary key
├── requester_user_id UUID                - immutable FK to auth.User
├── recipient_user_id UUID                - immutable FK to auth.User
├── status            FriendRequestStatus - current lifecycle state
├── created_at        time.Time           - immutable creation timestamp
├── updated_at        time.Time           - timestamp of the latest status change
└── responded_at      *time.Time          - terminal recipient response timestamp
```

| Field | Type | Required | Purpose | Mutability |
|---|---|---:|---|---|
| `ID` | `uuid.UUID` | Yes | Stable surrogate identity for authorization, idempotency, and future audit/reference use. | Immutable |
| `RequesterUserID` | `uuid.UUID` | Yes | User who initiated the directed request. References `auth.User`. | Immutable |
| `RecipientUserID` | `uuid.UUID` | Yes | User authorized to accept or reject the directed request. References `auth.User`. | Immutable |
| `Status` | `FriendRequestStatus` | Yes | One of `pending`, `accepted`, `rejected`, or `cancelled`; determines permitted transitions. | Forward-only |
| `CreatedAt` | `time.Time` | Yes | Records when the request was submitted. | Immutable |
| `UpdatedAt` | `time.Time` | Yes | Records the latest status transition; supports ordering and audit. | Updated on every transition |
| `RespondedAt` | `*time.Time` | No | Records when the recipient accepted or rejected. It is nil for `pending` and `cancelled`. | Set once for recipient terminal transitions |

`RespondedAt` distinguishes recipient decisions from requester cancellation without adding a separate event model. A `CancelledAt` field is not proposed initially: `UpdatedAt` records cancellation time and the status identifies the reason. If product requirements later need a complete relationship-event audit, that concern should become an explicit history/audit design rather than incrementally adding timestamps without a retention policy.

No profile snapshot, message body, friendship ID, block state, privacy state, soft-delete marker, or denormalized counter belongs on this entity.

## 3. Status Lifecycle

`FriendRequestStatus` contains exactly four initial values:

```text
                    recipient accepts
          +--------------------------------> [accepted]
          |
[pending] +---- recipient rejects --------> [rejected]
          |
          +---- requester cancels --------> [cancelled]
```

| From | Actor | Action | To | Required behavior |
|---|---|---|---|---|
| None | Requester | Send | `pending` | Validates both users, existing Friendship, policy, and pending-request conflicts. |
| `pending` | Recipient | Accept | `accepted` | Atomically establishes the Friendship through Friends and settles this request. |
| `pending` | Recipient | Reject | `rejected` | Settles the request without creating a Friendship. |
| `pending` | Requester | Cancel | `cancelled` | Withdraws the request without creating a Friendship. |

`accepted`, `rejected`, and `cancelled` are terminal states. A request never transitions backward to `pending`, changes direction, or changes either endpoint. Retrying a terminal action must not silently alter state; the future service must return an invalid-state or idempotency outcome selected by its eventual API contract.

No expiry state is proposed. Request expiration is a genuine product policy decision and is listed in Open Decisions.

## 4. Invariants and Business Rules

1. A Friend Request has exactly one requester and exactly one recipient.
2. `RequesterUserID` and `RecipientUserID` are non-nil, distinct User IDs.
3. Direction is preserved: `(A -> B)` and `(B -> A)` are distinct representations, although simultaneous opposite pending requests require an explicit resolution policy.
4. At most one `pending` request may exist for the same directed pair.
5. Both endpoints must be eligible active Users when a request is sent.
6. The recipient must remain an eligible active User to accept or reject; the requester must remain eligible active to cancel.
7. A request cannot be sent when the two Users already have an established Friendship. The service must report an explicit already-friends error before attempting persistence.
8. An acceptance can establish at most one Friendship, and no Friendship is duplicated in Friend Requests persistence.
9. Only the requester may cancel their pending request. Only the recipient may accept or reject it.
10. Terminal requests are immutable. No transition returns a request to `pending`.
11. The database must independently enforce structural invariants and pending-request uniqueness; service checks provide clear errors but cannot prevent races.

### Duplicate and Reverse-Direction Requests

A second `A -> B` request while `A -> B` is pending is a duplicate and must be rejected as a conflict. Recreating a request after a terminal state is governed by the history/retention decision in section 8.

An existing pending `B -> A` request is not a duplicate of `A -> B`, because the direction differs. The desired user-visible outcome when A and B send to each other is an unresolved product policy. The future service must detect both pending directions during the same transactional decision, rather than permitting independent rows to determine an accidental outcome. See OD-01.

## 5. Friendship Integration

Friend Requests initiates acceptance workflow; Friends exclusively owns the established Friendship.

```text
[pending Friend Request]
            |
            | recipient accepts
            v
Friend Requests service coordinates transaction
            |
            +--> Friends.EstablishFriendship(requester, recipient)
            |
            +--> marks request accepted
            v
[accepted Friend Request] + [active Friendship]
```

The future Friend Requests service is responsible for validating the recipient actor and request state, then coordinating the acceptance transaction. The Friends service remains the only component that creates the canonical, mutual Friendship record and applies its pair canonicalization, active-user eligibility, and duplicate-Friendship rules.

Acceptance must use a single database transaction shared by the request update and Friendship creation. Neither of these partial outcomes is acceptable:

- A request is `accepted` but no Friendship exists.
- A Friendship exists but the request is still `pending`.

If a Friendship already exists when acceptance is attempted, the service must not create a second representation. How to settle the pending request in this recovery/race case is part of the acceptance error/idempotency contract and is covered by OD-04.

No `friendship_id` is stored on FriendRequest initially. A Friendship is identified by its user pair; retaining a mutable cross-reference adds coupling without a defined query need.

## 6. Ownership and Authorization

Authentication supplies the acting User ID. The future Friend Requests service, not the entity or handler, enforces actor-specific business authority.

| Operation | Authorized actor |
|---|---|
| Send request | Requester, derived from authenticated identity |
| Accept request | The request's recipient only |
| Reject request | The request's recipient only |
| Cancel request | The request's requester only |
| List incoming requests | The authenticated recipient only |
| List outgoing requests | The authenticated requester only |

User-supplied requester IDs must not be trusted. The send operation receives the requester from validated authentication context and only accepts a target/recipient identifier as input.

Administrative moderation or support overrides, if later required, must use existing RBAC authorization. This design introduces no Friend Requests-specific role or permission.

## 7. User Eligibility and Lifecycle

The established User statuses are `pending_verification`, `active`, `suspended`, and `deleted`. `deleted` with a non-nil `DeletedAt` is a soft-deleted account during its grace period.

| Account state | Send | Receive | Accept | Reject | Cancel | Visibility in request lists |
|---|---|---|---|---|---|---|
| `active` and not soft-deleted | Eligible | Eligible | Eligible | Eligible | Eligible | Visible subject to future privacy/blocking policy |
| `pending_verification` | Ineligible | Ineligible | Ineligible | Ineligible | Ineligible | Not exposed |
| `suspended` | Ineligible | Ineligible | Ineligible | Ineligible | Ineligible | Not exposed |
| `deleted` / soft-deleted | Ineligible | Ineligible | Ineligible | Ineligible | Ineligible | Not exposed |
| Hard-deleted | No User record | No User record | No User record | No User record | No User record | Removed by cascade |

This follows the implemented Friends and Profile rules: social operations require `User.IsActive()` and a non-deleted account. A suspended or soft-deleted account does not automatically mutate existing Friend Requests; requests are preserved through the grace period but cannot be acted on or displayed. When the account is restored to `active`, preserved request records again become eligible for normal processing, subject to the eventual blocking/privacy policy.

On hard deletion, database foreign keys must physically remove every Friend Request where the User is requester or recipient. This matches the dependent-data deletion policy used by Profiles and Friendships. Friend Requests does not add a separate account lifecycle or independent soft deletion.

## 8. Request History and Retention

The entity supports terminal statuses and timestamps, but the product has not determined whether terminal request records are retained permanently, retained for a bounded audit window, or removed immediately after settlement.

Until that decision is made, the design requires that the persistence model distinguish the current pending request from prior terminal requests. It must not use a permanent unique constraint on `(requester_user_id, recipient_user_id)` if users should be able to send another request after a rejection or cancellation.

The default working assumption for later schema design is:

- Preserve terminal records initially for audit and operational diagnosis.
- Enforce uniqueness only for `pending` requests, normally with a partial unique index.
- Allow a new request for the same direction after a terminal record, subject to future cooldown and reverse-direction policy.

This is a recommended default, not an adopted product decision; see OD-02 and OD-03.

## 9. Read Model and Pagination

Friend Requests read models return request relationship data, not embedded User or Profile entities. Incoming and outgoing lists may resolve `ProfileSummary` data at the application/query layer for display, using `user_id` as the join key. The entity itself stores neither username, display name, avatar URL, presence, nor privacy data.

| List | Ownership perspective | Default order | Pagination |
|---|---|---|---|
| Incoming requests | Requests where the actor is `RecipientUserID` | Most recently created first: `created_at DESC, id DESC` | Shared page/offset pagination |
| Outgoing requests | Requests where the actor is `RequesterUserID` | Most recently created first: `created_at DESC, id DESC` | Shared page/offset pagination |

The existing shared repository abstraction uses `repository.Pagination` with normalized `Page` and `PageSize`, defaulting to page 1 and 20 items with a maximum page size of 100. It returns `repository.PageMeta`; its `Cursor` field is reserved and not active. Future request lists must use this established page/offset convention rather than introduce cursor pagination in this module.

The exact terminal-status filtering for lists depends on OD-02. Pending requests are the minimum required result set. The API and DTO design later determines public response shapes and whether status-specific history endpoints exist.

## 10. Future Service Boundary

The future Friend Requests service owns workflow validation, authorization, account eligibility checks, transition control, conflict handling, and coordination with Friends. It does not contain SQL, HTTP concerns, DTO validation annotations, Profile persistence, or notification delivery.

| Conceptual operation | Service responsibility |
|---|---|
| `SendRequest(actorUserID, recipientUserID)` | Validate IDs and active endpoints; reject self-request and existing Friendship; resolve duplicate/reverse pending policy; create a pending request. |
| `AcceptRequest(actorUserID, requestID)` | Verify recipient ownership and active endpoints; lock and validate pending state; establish Friendship through Friends and mark the request accepted atomically. |
| `RejectRequest(actorUserID, requestID)` | Verify recipient ownership and active recipient; lock and transition pending to rejected. |
| `CancelRequest(actorUserID, requestID)` | Verify requester ownership and active requester; lock and transition pending to cancelled. |
| `GetIncomingRequests(actorUserID, page)` | Return authorized paged request references for the actor as recipient. |
| `GetOutgoingRequests(actorUserID, page)` | Return authorized paged request references for the actor as requester. |

The eventual API layer authenticates the caller, parses inputs, validates transport shape, maps responses, and translates the existing global `AppError` model. It must not decide transitions or bypass the service by directly updating a request.

## 11. Future Repository Boundary

The future repository abstracts persistence of Friend Request records and supports the service's transaction boundary through the established `repository.Option` model, including `WithTransaction` and `WithLock`.

| Conceptual operation | Required behavior |
|---|---|
| `Create` | Persist a new pending request and return database-populated fields. |
| `FindByID` | Find a request by stable ID, optionally locked for transition processing. |
| `FindPendingByDirectedPair` | Find the pending `requester -> recipient` request. |
| `FindPendingByEitherDirection` | Detect a pending request between the two Users in either direction for reverse-request policy resolution. |
| `ListIncoming` | Return paged requests where a User is recipient, with total count and stable ordering. |
| `ListOutgoing` | Return paged requests where a User is requester, with total count and stable ordering. |
| `TransitionPendingStatus` | Perform a conditional pending-to-terminal update and report whether it succeeded. |

The acceptance orchestration must execute request locking/transition and Friendship creation against the same transaction. The exact mechanism may be a transaction-aware repository interface or a higher application transaction coordinator, selected during implementation without weakening the atomicity requirement.

No repository method is required for blocking, privacy, notifications, search, presence, or Profile data. Those modules own their respective stores and query policies.

## 12. Domain Errors

The implementation must extend the existing global `AppError` architecture with a Friend Requests-specific error package, following the existing Friends pattern. The following are conceptual errors, not final code names or API payloads.

| Error concept | Condition | Suggested classification |
|---|---|---|
| Invalid user ID / invalid target | A required ID is nil or malformed at the relevant boundary. | Bad request / validation |
| Cannot request self | Requester and recipient are the same User. | Bad request |
| User unavailable / not eligible | An endpoint is absent, not active, suspended, pending verification, or soft-deleted. | Forbidden or existing account error |
| Already friends | The users already have an established Friendship. | Conflict |
| Duplicate pending request | An equivalent directed pending request already exists. | Conflict |
| Opposite pending request | A pending request already exists in the reverse direction. | Conflict or policy-specific result, pending OD-01 |
| Request not found | No request exists for the requested ID or authorized query. | Not found |
| Unauthorized request transition | The actor is neither the endpoint authorized for that action nor an authorized administrator. | Forbidden |
| Invalid request state | The request is no longer pending or the requested transition is unsupported. | Conflict |
| Friendship establishment conflict | A Friendship exists or is created concurrently during acceptance. | Conflict or idempotent settlement, pending OD-04 |

The service must translate database uniqueness and conditional-update races into these meaningful domain errors or an explicitly adopted idempotent result. A preliminary read cannot be the only integrity protection.

## 13. Concurrency and Atomicity

### Same-Direction Sends

Concurrent `A -> B` submissions may both observe no pending request. A database partial unique constraint for pending directed pairs is the final guarantee that at most one pending request persists. The service maps the duplicate-key failure to the duplicate-pending-request result.

### Opposite-Direction Sends

Concurrent `A -> B` and `B -> A` sends are not prevented by directed-pair uniqueness. The desired outcome is a product decision: automatic friendship, deterministic winner, or conflict requiring an explicit recipient action. The service must serialize or otherwise safely resolve the pair-level decision under concurrency. It must not allow the result to vary merely because two requests race.

The later schema/service design must select a concrete pair-level synchronization strategy, such as transaction-scoped advisory locks on a canonical pair or an equivalent lockable pair representation. The exact database mechanism is implementation detail; deterministic policy enforcement is required.

### Accept Versus Cancel, Reject, or Duplicate Accept

Transitions must conditionally update only a `pending` record while holding an appropriate row lock or using an equivalent atomic `WHERE status = 'pending'` update. Exactly one terminal transition wins. The loser receives an invalid-state or idempotency outcome; it must not overwrite the winning status.

### Accept Versus Friendship Creation

Acceptance must run in one transaction. The service locks the request, confirms it is pending and authorized, verifies eligibility and existing Friendship state, calls the transaction-aware Friends establishment operation, and marks the request accepted. Friendship's canonical-pair uniqueness remains the final guarantee against duplicate mutual relationships.

## 14. Module Boundaries

| Module | Relationship to Friend Requests | Friend Requests responsibility |
|---|---|---|
| Users / Auth | Owns User identity, status, soft deletion, and RBAC. | Reference User IDs and ask Auth whether endpoints are eligible; do not duplicate account state. |
| Friends | Owns canonical, established mutual Friendships. | Coordinate accepted requests with Friends; do not store a second Friendship representation. |
| Profiles | Owns social presentation data. | Resolve profile summaries only in read projections; store no profile attributes or Profile IDs. |
| Blocking (future) | Owns unilateral interaction restrictions. | Do not implement block fields or policy; later consult Blocking before send/accept/list visibility as defined by that module. |
| Privacy (future) | Owns request permission, discoverability, and visibility policy. | Do not implement privacy fields or rules; provide a service boundary where Privacy can authorize send/receive/list behavior. |
| Notifications (future) | Owns delivery and notification preference handling. | Do not deliver notifications or create an event bus. Future application events may include `friend_request.created`, `friend_request.accepted`, `friend_request.rejected`, and `friend_request.cancelled`, with notification eligibility resolved by Notifications. |
| Search | Discovers Users and Profiles. | Do not search or index users; accept a resolved target User ID. |
| Presence | Owns dynamic availability state. | Do not store or query online/offline state. |

Notifications are an integration point, not a current side effect. When an event mechanism exists, events should be emitted only after the request transaction commits, so consumers never observe a request state that rolled back.

## 15. Future Database Requirements

The future schema must satisfy these requirements without prescribing the migration implementation in this document:

1. A UUID surrogate primary key consistent with project conventions.
2. Non-null `requester_user_id` and `recipient_user_id` foreign keys to `users(id)`.
3. `ON DELETE CASCADE` on both user foreign keys so hard deletion removes dependent requests.
4. A check preventing equal requester and recipient IDs.
5. A constrained status representation permitting only the adopted lifecycle values.
6. Non-null database-generated `created_at` and `updated_at` values, with `updated_at` maintained on status transitions.
7. A nullable `responded_at` whose consistency with recipient terminal statuses is enforced by application logic and, where practical, database checks.
8. A pending-only directed-pair uniqueness rule, if OD-02 adopts retained terminal history and request recreation.
9. Indexes supporting pending directed-pair lookup, either-direction pending conflict detection, and incoming/outgoing list pagination ordered by `created_at DESC, id DESC`.
10. A transactional implementation path that can coordinate request settlement and Friendship creation.

The exact migration number, index definitions, terminal-record retention mechanics, expiration policy, and reverse-direction strategy belong to later database and implementation steps.

## 16. Open Decisions

| ID | Question | Recommended/default assumption |
|---|---|---|
| OD-01 | When A sends B a request while B has a pending request to A, should the system automatically establish a Friendship, retain one deterministic request, or return a conflict requiring an explicit recipient action? | No architecture-led default. Choose a product experience before implementation; automatic friendship has the largest consent implication. |
| OD-02 | Are accepted, rejected, and cancelled requests retained permanently, retained for a bounded period, or removed after settlement? | Retain terminal records initially, with a pending-only unique constraint, because the entity includes terminal state and operational/audit diagnosis benefits from history. |
| OD-03 | May a requester recreate a request after rejection or cancellation, and should a cooldown limit repeated requests? | Allow recreation after a terminal result only if OD-02 permits it; define a cooldown with the future Privacy/anti-harassment policy. |
| OD-04 | If a Friendship already exists during request acceptance because of a concurrent/manual establishment, should acceptance settle the request as accepted idempotently or return a conflict and leave it pending for reconciliation? | Settle as accepted in the same transaction when the Friendship exists, provided authorization and endpoint eligibility still pass; this avoids a stale pending request beside an established friendship. |
| OD-05 | Do pending requests expire automatically, and if so, after what duration and into which terminal status? | No expiration initially. Add only with a confirmed product requirement and a defined user-visible history policy. |
| OD-06 | Which blocking and privacy rules apply to sending, receiving, accepting, and listing requests once those modules exist? | Friend Requests delegates this policy. Blocking and Privacy designs must define the authorization/query contract before integration. |