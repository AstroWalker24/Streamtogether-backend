---
title: Presence Domain Design
description: Domain design for real-time presence, availability, connection lifecycle, and visibility semantics in the StreamTogether social graph.
ms.date: 2026-09-10
status: draft
phase: 3 — Social Graph
step: 3.5.1
depends-on: authentication-domain-design.md, authentication-domain-contracts.md, profile-domain-design.md, friends-domain-design.md
---

# Presence Domain Design

## Table of Contents

1. [Domain Purpose and Boundaries](#1-domain-purpose-and-boundaries)
2. [Presence States and State Model](#2-presence-states-and-state-model)
3. [Temporal Semantics and "Last Seen"](#3-temporal-semantics-and-last-seen)
4. [Device and Session Relationships](#4-device-and-session-relationships)
5. [Multiple Connections and Aggregation](#5-multiple-connections-and-aggregation)
6. [Connection Identity](#6-connection-identity)
7. [Lease Expiration and Stale Presence](#7-lease-expiration-and-stale-presence)
8. [Explicit vs Derived Presence](#8-explicit-vs-derived-presence)
9. [Privacy and Visibility Boundary](#9-privacy-and-visibility-boundary)
10. [Blocking and Restriction Boundary](#10-blocking-and-restriction-boundary)
11. [Social Scope and Friends Integration](#11-social-scope-and-friends-integration)
12. [Self Visibility](#12-self-visibility)
13. [Domain Operations and Service Contract](#13-domain-operations-and-service-contract)
14. [Read Models and Bulk Reads](#14-read-models-and-bulk-reads)
15. [Public Visibility Representation](#15-public-visibility-representation)
16. [Storage Model Boundary](#16-storage-model-boundary)
17. [Failure Semantics and Resilience](#17-failure-semantics-and-resilience)
18. [Account Lifecycle Integration](#18-account-lifecycle-integration)
19. [Phase 3.5 vs Phase 5 Architectural Boundary](#19-phase-35-vs-phase-5-architectural-boundary)
20. [Open Decisions](#20-open-decisions)

---

## 1. Domain Purpose and Boundaries

The Presence domain represents a user's **current platform availability and communication readiness** within StreamTogether. It answers: _"Is this user currently available on the platform, when were they last seen, and what is their effective availability state?"_

Presence is a core social building block for Friends lists, Watch Parties, Direct Messaging, and Real-Time Synchronization. However, presence must not be conflated with the underlying network transport or high-level party activities.

### Core Boundary Distinctions

To ensure clean system architecture and avoid tight coupling with transport protocols, the following concepts are strictly distinguished:

| Concept | Meaning | Owning Layer / Module |
|---|---|---|
| **Transport Connection** | An active raw TCP/TLS socket or WebSocket connection between a client and a specific gateway server node. | Phase 5 Real-Time Gateway / Transport |
| **Presence Connection** | A leased, logical presence session registered to a User, anchored by an authenticated Auth Session and Device. | Phase 3.5 Presence Domain |
| **User Presence State** | The aggregate availability state (`online`, `away`, `offline`) derived from a user's active presence connections and preferences. | Phase 3.5 Presence Domain |
| **Last Seen** | The temporal record indicating when a user was last known to be actively connected to the platform. | Phase 3.5 Presence Domain |
| **User Activity** | What a user is currently doing on the platform (e.g., watching a specific video title, participating in a watch party). | Watch Party / Playback Domain (Phase 4) |
| **Presence Visibility** | The filtered projection of a user's presence after evaluating privacy settings, friendship relations, and block rules. | Privacy & Social Graph Facade |

```
┌────────────────────────────────────────────────────────┐
│                   Party / Playback                     │
│      "Watching Movie X in Party #123" (Activity)        │
└──────────────────────────┬─────────────────────────────┘
                           │ contextual overlay
┌──────────────────────────▼─────────────────────────────┐
│                   Presence Domain                      │
│     State: "online" | "away" | "offline"               │
│     Aggregate user availability & last_seen_at         │
└──────────────────────────┬─────────────────────────────┘
                           │ derives state from
┌──────────────────────────▼─────────────────────────────┐
│                 Presence Connections                   │
│   Active Leases: Conn #1 (Web), Conn #2 (Mobile)       │
└──────────────────────────┬─────────────────────────────┘
                           │ established via
┌──────────────────────────▼─────────────────────────────┐
│             Phase 5 Real-Time Transport                │
│       WebSocket TCP Connections & Gateway Nodes        │
└────────────────────────────────────────────────────────┘
```

Presence owns the **truth of availability**. It does not own WebSocket connection loops, network framing, push notifications, video playback synchronization, or social privacy policies.

---

## 2. Presence States and State Model

### Initial Core Presence States

The Presence domain establishes three initial core states:

| State | Semantic Meaning | Entry Condition | Exit Condition |
|---|---|---|---|
| `online` | The user is actively connected to the platform and ready for social interaction. | At least one valid, unexpired presence connection exists AND the user is not marked `away`. | All connections disconnect/expire, OR user/client enters `away`. |
| `away` | The user is connected to the platform, but is temporarily idle or has explicitly indicated unavailability. | Client reports user inactivity (idle threshold reached), OR user manually selects "Away". | User resumes active interaction, user manually toggles back to `online`, OR all connections disconnect/expire (transitions to `offline`). |
| `offline` | The user has no active, unexpired presence connections on any device or client. | All active presence connections are closed or their heartbeat leases expire without renewal. | A new presence connection is successfully established and leased. |

### Extended and Contextual States ("Watching", "In Party")

Product requirements list states such as `watching` and `in party`. 

**Architectural Decision:** These are **not** distinct base presence states in the core Presence state machine. Conflating party participation or video playback state with core network availability introduces circular dependencies between the Social Graph (Phase 3), Presence (Phase 3.5), and Watch Party / Playback (Phase 4).

Instead:
- The base Presence state remains `online` (or `away`).
- Contextual activity is represented as an optional **Activity Overlay** (`UserActivity`) attached to the presence read model by the application layer when the user is in a party.
- If a user is in a party and their network connection drops, their base presence transitions to `offline` via heartbeat expiration, which cleanly informs the Party module of their disconnection without corrupting base presence invariants.

### State Transitions

```
                    ┌─────────────────────────┐
                    │                         │
                    │         OFFLINE         │
                    │   (0 active leases)     │
                    │                         │
                    └───────────┬─────────────┘
                                │
                 First connection established
                                │
                                ▼
                    ┌─────────────────────────┐
                    │                         │
         ┌─────────►│         ONLINE          │◄────────┐
         │          │   (Active connection)   │         │
         │          │                         │         │
         │          └───────────┬─────────────┘         │
         │                      │                       │
         │          User idle or explicit "Away"        │
         │                      │                       │
         │                      ▼                       │
User interaction    ┌─────────────────────────┐         │
  or explicit       │                         │         │
    "Online"        │          AWAY           │─────────┘
         │          │    (Connected, idle)    │
         │          │                         │
         │          └───────────┬─────────────┘
         │                      │
         └──────────────────────┼───────────────────────┘
                                │
                 All connections disconnected/expired
                                │
                                ▼
                    ┌─────────────────────────┐
                    │                         │
                    │         OFFLINE         │
                    │   (Update last_seen_at) │
                    │                         │
                    └─────────────────────────┘
```

---

## 3. Temporal Semantics and "Last Seen"

Timestamps in a real-time system must have unambiguous owners and purposes. The Presence domain defines four distinct timestamps across its domain and connection layers:

| Timestamp | Scope | Ownership | Persistence | Purpose |
|---|---|---|---|---|
| `last_seen_at` | User aggregate | Presence Domain | **Durable** (PostgreSQL) | Indicates when the user was last known to be online. Rendered in friends lists and profiles when the user is `offline`. |
| `last_active_at` | Session / Device | Auth Domain | **Durable** (PostgreSQL) | Owned by `internal/auth/domain` ([internal/auth/domain/session.go](internal/auth/domain/session.go#L20)). Used for session lifetime extension, idle timeouts, and credential revocation. **Not modified by real-time presence heartbeats.** |
| `connected_at` | Presence Connection | Presence Connection | **Ephemeral** (Redis) | Records when a specific connection instance was registered. Used for connection duration metrics and connection ordering. |
| `last_heartbeat_at` | Presence Connection | Presence Connection | **Ephemeral** (Redis) | Records the most recent liveness signal for a connection instance. Used internally to calculate lease expiry; never exposed to observers. |

### Preservation of Auth Domain Boundaries

A common anti-pattern in real-time systems is updating the database `sessions.last_active_at` on every real-time ping (e.g., every 15 seconds). With thousands of concurrent users, this would cause severe database write amplification.

**Rule:** High-frequency presence heartbeats update `last_heartbeat_at` exclusively in ephemeral storage (Redis). `last_seen_at` is persisted to durable storage only upon state transitions (e.g., when the user's last connection closes or expires, or lazily throttled to disk at most once every 15–30 minutes while continuously online). Auth's `session.last_active_at` is never updated by Presence heartbeats.

---

## 4. Device and Session Relationships

StreamTogether already models `User`, `Device`, and `Session` in the Authentication domain:
- A `User` is the persistent human identity ([internal/auth/domain/user.go](internal/auth/domain/user.go#L21)).
- A `Device` is a persistent endpoint (browser fingerprint, mobile device) registered to a user ([internal/auth/domain/device.go](internal/auth/domain/device.go#L21)).
- A `Session` is the server-side revocation anchor binding a `User` and a `Device` for a specific authentication lifespan ([internal/auth/domain/session.go](internal/auth/domain/session.go#L14)).

### Multiplicity in Presence

A single user may engage with StreamTogether across multiple devices and sessions simultaneously:
- A user may be logged into their mobile app (Session 1, Device 1).
- The same user may be logged into their desktop browser (Session 2, Device 2).
- The user may open multiple tabs in their desktop browser, or run both the Web App and the Browser Extension simultaneously (Session 2, Device 2, but **multiple transport connections**).

### Relationship Rules

1. **No changes to Auth entities:** Presence does not alter the schema or domain rules of `Session` or `Device`.
2. **Derived user-level presence:** User-level presence is not a static flag on `users`. It is dynamically derived from the collection of active presence connections currently registered under that `user_id`.
3. **Connection-to-Session binding:** Every `PresenceConnection` holds a `UserID` and references the `SessionID` from the validated JWT claims. This enables the Presence service to immediately terminate all presence connections associated with a session when that session is revoked or logged out.

```
User (auth.User)
  └── Session A (auth.Session, Laptop)
  │     ├── PresenceConnection 1 (Tab A - WebApp)
  │     └── PresenceConnection 2 (Tab B - Extension)
  └── Session B (auth.Session, Mobile Phone)
        └── PresenceConnection 3 (iOS App)

Derived User Presence:
  - Active Connections: 3
  - Overall State: ONLINE
```

---

## 5. Multiple Connections and Aggregation

### The Multi-Connection Problem

When a user has multiple active connections (e.g., two browser tabs or mobile + desktop):
1. Connection A connects $\rightarrow$ User is `online`.
2. Connection B connects $\rightarrow$ User remains `online`.
3. Connection A disconnects $\rightarrow$ Connection B is still open.

**Invariant:** A user must **not** transition to `offline` when Connection A disconnects if Connection B remains healthy and active.

### Presence Aggregation and Reference Tracking

To enforce this safely, the Presence domain treats a user's presence as an aggregate of their active presence connection set:

- **Connection Registration:** Adding a connection to a user's active set increments the active count and establishes the user's presence as `online` (if previously `offline`).
- **Connection Removal:** Removing a connection decrements the active count. Only when the active connection count drops to zero does the user initiate transition to `offline`.
- **State Precedence:** If different connections report conflicting explicit states (e.g., Connection 1 reports active interaction while Connection 2 has been idle for 20 minutes), the aggregate presence resolves using **highest activity precedence**:
  $$\text{online} > \text{away} > \text{offline}$$

---

## 6. Connection Identity

Every active real-time connection requires an explicit logical identity within the Presence domain to enable deterministic routing, heartbeats, and cleanup.

### PresenceConnection Entity

```text
PresenceConnection
├── connection_id    uuid.UUID      — unique surrogate identity for this connection instance
├── user_id          uuid.UUID      — immutable FK to auth.User
├── session_id       uuid.UUID      — immutable FK to auth.Session (from JWT claims)
├── device_id        uuid.UUID      — identifier of originating device (if present in claims/session)
├── node_id          string         — identifier of gateway cluster node hosting the connection
├── platform         DevicePlatform — client platform (web, desktop, extension, ios, android)
├── state            PresenceState  — reported state for this specific connection (online, away)
├── connected_at     time.Time      — connection registration timestamp
├── last_heartbeat_at time.Time     — latest confirmed heartbeat ping
└── expires_at       time.Time      — absolute lease expiry timestamp unless renewed
```

### Uniqueness and Gateway Compatibility

- `connection_id` is a UUID generated by the gateway node upon accepting the WebSocket handshake.
- It is globally unique across the entire server cluster.
- This design is 100% compatible with the future Phase 5 WebSocket Connection Manager: the Connection Manager maps network sockets to `connection_id` locally, while the Presence domain manages the distributed lifecycle of `connection_id` in the shared store.

---

## 7. Lease Expiration and Stale Presence

In distributed real-time systems, clients frequently disconnect ungracefully due to browser crashes, network loss, mobile app suspension, or power cuts. In such cases, no TCP FIN or WebSocket Close frame is delivered to the server.

### Lease / Heartbeat Model

1. **Heartbeat Lease:** Every registered `PresenceConnection` is issued a temporary **lease** with a time-to-live (TTL), typically 30–45 seconds.
2. **Periodic Refresh:** The client sends a lightweight heartbeat ping at a defined interval (e.g., every 15 seconds). Each heartbeat extends `expires_at` in the presence store by the lease duration.
3. **Stale Lease Eviction:** If no heartbeat is received before `expires_at`, the lease expires and the connection is marked dead.

### Detection Responsibilities

- **Passive Eviction (Presence Store):** Expired connection keys are automatically pruned by the ephemeral presence store (e.g., Redis key expiry or sorted set timestamp pruning).
- **Active Eviction (Gateway Sweep):** The Phase 5 Connection Manager runs a local periodic reaper that cleans up local socket resources for connections that failed to ping.
- **Domain Invariant:** The Presence domain treats any connection whose `expires_at < now` as non-existent, even before passive or active storage sweeps complete.

### Flapping and Reconnection Grace Period

When a user refreshes their browser page or briefly navigates between routes:
- Tab connection A closes.
- 500 milliseconds later, Tab connection B opens.

If the user has only one connection, immediately transitioning them to `offline` and broadcasting that transition to all friends causes annoying **presence flapping** (rapidly flickering between online and offline).

**Domain Rule:** When a user's active connection count reaches zero, the user enters a transitional **Disconnecting Grace Period** (10 seconds, adopted per [Section 20, OD-3](#20-open-decisions)):
- The user is scheduled to become `offline`.
- If a new connection is registered for that user before the grace period expires, the offline transition is aborted and the user remains continuously `online`.
- If the grace period elapses without any new connections, the user transitions to `offline`, `last_seen_at` is committed to durable storage, and downstream social notifications/broadcasts are dispatched.

---

## 8. Explicit vs Derived Presence

Presence transitions are divided strictly between client-controlled signals and server-controlled state rules.

| Transition | Controlling Agent | Trigger | Validation / Invariant |
|---|---|---|---|
| Connect $\rightarrow$ `online` | Server-controlled | Valid connection registered | Requires valid authenticated session. |
| Disconnect $\rightarrow$ `offline` | Server-controlled | Zero active connections remaining and grace period expired | Server updates `last_seen_at`. |
| Timeout $\rightarrow$ `offline` | Server-controlled | Heartbeat lease expires without renewal | Server treats dead connections as disconnected. |
| Idle $\rightarrow$ `away` | Client-controlled or Server-inferred | User inactive for configured threshold (e.g., 10 min) | Connection remains leased and active. |
| Return $\rightarrow$ `online` | Client-controlled | User resumes input / activity | Resets connection state from `away` to `online`. |
| Manual "Away" | Client-controlled | User explicitly selects "Set as Away" | Explicit user preference overrides auto-idle. |
| "Appear Offline" | Client-controlled | User toggles "Invisible" / "Appear Offline" | Treated as a **visibility mask** (see Section 9), not a false domain state. |

### Multi-Device Conflict Resolution

If a user is logged in on both Laptop (Tab A) and Mobile (Phone B):
- Laptop is idle $\rightarrow$ reports `away`.
- Mobile receives active touch input $\rightarrow$ reports `online`.
- **Resolution:** Aggregate status is `online`. A user is considered reachable and available if any of their active devices is currently active.

---

## 9. Privacy and Visibility Boundary

A critical requirement of the social graph architecture is separating **Presence as objective domain state** from **Presence Visibility as a privacy policy**.

### Separation of Concerns

```
┌────────────────────────────────────────────────────────┐
│                    Presence Domain                     │
│  "User 42 is currently ONLINE with 2 active leases"    │
└──────────────────────────┬─────────────────────────────┘
                           │ Raw Presence State
                           ▼
┌────────────────────────────────────────────────────────┐
│                   Visibility Layer                     │
│  Evaluates:                                            │
│    1. Is observer querying their own presence?         │
│    2. Does a block exist between observer & target?    │
│    3. What are target's privacy settings?              │
│    4. Are observer and target mutual friends?          │
└──────────────────────────┬─────────────────────────────┘
                           │ Masked / Filtered Projection
                           ▼
┌────────────────────────────────────────────────────────┐
│                    Observer View                       │
│  "User 42 is OFFLINE" (Masked by Privacy or Block)     │
└────────────────────────────────────────────────────────┘
```

1. **Presence owns truth:** The Presence store records whether the user is physically online, away, or offline. Presence never stores privacy toggles, friend lists, or block lists.
2. **Privacy owns visibility:** When User B requests the presence of User A, a visibility evaluation determines whether User B is authorized to see User A's real status.
3. **Invisible Mode ("Appear Offline"):** If User A enables "Appear Offline", the Presence domain still tracks User A as `online` internally (so the system knows they are reachable, can receive real-time notifications, or manage their own parties), but the Visibility Layer projects them as `offline` to all other users.

---

## 10. Blocking and Restriction Boundary

Blocking (Phase 3.6) imposes strict social separation between two users.

### Architectural Invariants for Blocking and Presence

1. **No Blocking Data in Presence:** The Presence domain does not store, index, or query block tables.
2. **Bidirectional Masking:** If User A blocks User B, OR User B blocks User A:
   - When User A requests User B's presence $\rightarrow$ User B appears `offline` with `last_seen_at = nil`.
   - When User B requests User A's presence $\rightarrow$ User A appears `offline` with `last_seen_at = nil`.
3. **No Real-Time Leakage:** Real-time presence updates (Phase 5 broadcasts) must never be sent to blocked users. Broadcast fanout lists must be filtered against the blocking rules before transport dispatch.
4. **Error Masking:** Queries for a blocked user's presence do not return an error (which would leak the existence of a block); they return an innocuous `offline` response.

---

## 11. Social Scope and Friends Integration

Presence visibility is intrinsically tied to the social graph established in [docs/features/friends-domain-design.md](docs/features/friends-domain-design.md).

### Visibility Scopes

In StreamTogether, presence visibility is partitioned into four potential social scopes:

| Scope | Description | Applicability |
|---|---|---|
| **Self** | The authenticated user viewing their own status. | Always receives unmasked true state. |
| **Friends** | Mutual friends established in the Friends domain ([internal/friends/domain/friendship.go](internal/friends/domain/friendship.go#L11)). | Primary target for presence display. |
| **Party Room Members** | Users currently co-present in the same watch party room. | Temporary social context; users in a room see each other's room-presence. |
| **Public / Strangers** | Authenticated users who are not friends and share no party room. | By default, cannot see presence or last seen. |

### Adopted Visibility Policy (OD-1)

By default, presence visibility is restricted to confirmed **mutual friends** (adopted per [Section 20, OD-1](#20-open-decisions)). Non-friends and unauthenticated observers cannot view a user's real-time presence or last-seen timestamp unless a temporary party room co-presence context applies. The Presence domain design supports all scopes through its decoupled visibility projection.

---

## 12. Self Visibility

**Invariant:** A user querying their own presence (`observer_user_id == target_user_id`) must **always** receive their true, unmasked domain state.

Self-visibility guarantees:
- If a user is in "Appear Offline" mode, their self-view displays: `status: online`, `applied_visibility: appear_offline`, `active_connections: N`.
- If a user is `away`, their self-view displays `status: away`.
- A user can inspect their own active presence connections (e.g., to see that they have an active tab on Web and an active connection on Mobile).
- Privacy restrictions and block lists are bypassed when evaluating self-visibility.

---

## 13. Domain Operations and Service Contract

The Presence domain defines conceptual operations that future application services and repositories will implement. These operations define the business contract independent of transport or storage engine.

### Connection Lifecycle Operations (Internal / Real-Time Gateway)

```go
// RegisterConnection records a newly established presence connection and refreshes user presence.
RegisterConnection(ctx context.Context, conn PresenceConnection) error

// RemoveConnection terminates an active connection instance and evaluates user offline transition.
RemoveConnection(ctx context.Context, connectionID uuid.UUID, reason DisconnectReason) error

// RecordHeartbeat refreshes the lease expiration for an active connection.
RecordHeartbeat(ctx context.Context, connectionID uuid.UUID) error

// PruneStaleConnections identifies and purges connections whose leases have expired.
PruneStaleConnections(ctx context.Context) (int, error)
```

### User Preference Operations (Client-Driven)

```go
// SetUserPresencePreference allows a user to explicitly set their state (e.g., Away, Online, Appear Offline).
SetUserPresencePreference(ctx context.Context, userID uuid.UUID, pref PresencePreference) error

// ClearUserPresencePreference clears manual overrides, returning the user to auto-derived presence.
ClearUserPresencePreference(ctx context.Context, userID uuid.UUID) error
```

### Read and Query Operations (Social & Application Layer)

```go
// GetUserPresence returns the filtered presence view for a single user as seen by an observer.
GetUserPresence(ctx context.Context, observerID uuid.UUID, targetUserID uuid.UUID) (*UserPresenceView, error)

// GetPresenceForUsers returns filtered presence views for a batch of users (e.g., a friend list).
GetPresenceForUsers(ctx context.Context, observerID uuid.UUID, targetUserIDs []uuid.UUID) (map[uuid.UUID]*UserPresenceView, error)

// GetRawUserPresence retrieves unmasked presence data for internal platform operations (e.g., push notification routing).
GetRawUserPresence(ctx context.Context, userID uuid.UUID) (*RawUserPresence, error)
```

---

## 14. Read Models and Bulk Reads

### The Need for High-Performance Bulk Reads

Presence is heavily read-dominant. When an active user navigates to the StreamTogether dashboard or opens the Friends drawer, the client must display the presence state of up to 50–100 friends simultaneously.

Performing $N$ individual network requests or single-key cache lookups would result in unacceptable latency ($O(N)$ network hops).

### Bulk Query Contract

The Presence domain mandates a first-class **bulk read model**:

```
GetPresenceForUsers(observerID, [Friend1, Friend2, ..., FriendN])
```

- **Execution Strategy:**
  1. Batch fetch presence state from the ephemeral presence store using multi-key commands (e.g., Redis `MGET` or pipelined hash lookups).
  2. Batch fetch durable `last_seen_at` timestamps for any users found to be `offline`.
  3. Batch evaluate privacy and blocking masks.
  4. Return a consolidated map of `map[uuid.UUID]*UserPresenceView`.

---

## 15. Public Visibility Representation

The public projection of presence must be strictly minimal to protect user privacy and prevent infrastructure leakage.

### UserPresenceView (Public Projection)

```text
UserPresenceView
├── user_id       uuid.UUID     — the user this presence record represents
├── status        PresenceState — visible availability: "online" | "away" | "offline"
├── last_seen_at  *time.Time    — nil if currently online, or if hidden by privacy
└── updated_at    time.Time     — timestamp when this presence state was evaluated
```

### Explicitly Excluded Attributes

The following internal attributes must **never** appear in the public presence representation returned to other users:

- `connection_id` (internal transport handle)
- `session_id` (auth revocation anchor; security credential)
- `device_id`, device fingerprints, browser, OS, or user agents
- IP addresses or geolocation
- Server / Gateway `node_id`
- Active connection counts
- Heartbeat timestamps or lease expiration deadlines
- Internal presence lease tokens

---

## 16. Storage Model Boundary

Presence data exhibits two distinct access patterns:
1. **High-frequency, ephemeral state:** Heartbeats, lease countdowns, active connection IDs, and real-time counters.
2. **Low-frequency, durable state:** Historical `last_seen_at` timestamps and persistent user presence preferences (e.g., "Always appear offline").

### Ephemeral vs. Durable Storage Separation

| Data Element | Storage Classification | Target Engine | Loss Consequence |
|---|---|---|---|
| Active Connection Leases | **Ephemeral** | Redis (Phase 3.5.2) | Completely recoverable. If Redis restarts, clients reconnect and re-register within one heartbeat cycle. |
| User Active Connection Sets | **Ephemeral** | Redis (Phase 3.5.2) | Recoverable on client reconnect. |
| User Aggregate Status | **Derived** | Computed in-memory / cached in Redis | Derived dynamically from active connections and preferences. |
| `last_seen_at` | **Durable** | PostgreSQL (Phase 3.5.2) | Must survive cluster restarts so offline friends' last seen times remain accurate. |
| Presence Preferences | **Durable** | PostgreSQL (Phase 3.5.2) | User preferences (e.g., "Appear Offline") must persist across logins. |

```
                       ┌─────────────────────────────────────────┐
                       │          Client Heartbeat Ping          │
                       └────────────────────┬────────────────────┘
                                            │ Every 15s
                                            ▼
                       ┌─────────────────────────────────────────┐
                       │           Redis (Ephemeral)             │
                       │  - Refresh connection lease TTL         │
                       │  - Update active connection sets        │
                       └────────────────────┬────────────────────┘
                                            │ On final disconnect
                                            ▼
                       ┌─────────────────────────────────────────┐
                       │         PostgreSQL (Durable)            │
                       │  - Commit final last_seen_at timestamp   │
                       └─────────────────────────────────────────┘
```

---

## 17. Failure Semantics and Resilience

### 1. Ephemeral Store (Redis) Unavailable
- **Impact:** Real-time presence tracking is degraded.
- **Behavior:** The system fails open or degraded gracefully. Core HTTP APIs (authentication, profile updates, video streaming) continue operating without crashing. Presence queries return `offline` or `unknown` rather than throwing fatal 500 errors.
- **No Redis-to-Postgres Fallback for Heartbeats:** The application must **not** redirect high-frequency heartbeats to PostgreSQL if Redis fails; doing so would saturate the database connection pool.

### 2. Durable Store (PostgreSQL) Unavailable
- **Impact:** Updates to `last_seen_at` cannot be committed to disk.
- **Behavior:** Real-time presence continues functioning via Redis. If a user disconnects, their `last_seen_at` is cached in Redis temporarily and committed to PostgreSQL via a retry buffer once the database recovers.

### 3. Application / Gateway Server Crash
- **Impact:** Transport connections to that node are abruptly terminated.
- **Behavior:**
  - Clients detect transport closure and attempt exponential backoff reconnection to surviving gateway nodes.
  - Heartbeat leases for the severed connections expire naturally in Redis after the 30–45s TTL.
  - When clients reconnect to a new node, `RegisterConnection` creates fresh leases, seamlessly restoring presence.

### 4. Client Network Drop / Device Sleep
- **Impact:** No disconnect signal is sent to the server.
- **Behavior:** The gateway node receives no further heartbeats. The lease in Redis expires after the TTL. The user is transitioned to `offline` after the grace period.

---

## 18. Account Lifecycle Integration

Presence must strictly honor the user account lifecycle defined in [internal/auth/domain/user.go](internal/auth/domain/user.go#L14):

| User Account Status | Allowed to Connect? | Visible Presence State | Disconnect Behavior |
|---|---|---|---|
| `active` | Yes | Normal (`online`, `away`, `offline`). | Normal lease lifecycle. |
| `pending_verification` | No (blocked by Auth) | `offline` | Cannot authenticate WebSocket. |
| `suspended` | No | Masked as `offline` | Immediate forceful termination of all active presence connections; leases evicted immediately. |
| `deleted` (soft-deleted) | No | Masked as `offline` | Immediate forceful termination of all active connections; excluded from presence lookups. |
| `logged_out` (session revoked) | N/A | Evaluates remaining sessions | Connections tied to revoked `session_id` are purged immediately. |

**Security Invariant:** Suspended or soft-deleted users must never appear as `online` under any circumstances, even if an orphaned connection lease remains temporarily in memory.

---

## 19. Phase 3.5 vs Phase 5 Architectural Boundary

To preserve strict architectural layering and avoid premature optimization or scope creep, the responsibilities are cleanly divided between Phase 3.5 and Phase 5:

```
┌────────────────────────────────────────────────────────────────────────┐
│                        PHASE 3.5: PRESENCE                             │
│                                                                        │
│  ✓ Presence Domain Entities & Value Objects (UserPresence, State)      │
│  ✓ Connection Identity & Reference Tracking Concepts                   │
│  ✓ Derived State Machine (online, away, offline, idle)                 │
│  ✓ Lease Duration, Stale Presence, and Grace Period Definitions        │
│  ✓ Visibility & Privacy Boundaries (Friends, Blocks, Invisible Mode)   │
│  ✓ Service & Repository Storage Contracts                              │
│  ✓ Read Models & Batch Lookup Interfaces                               │
│  ✓ Redis Data Model & Schema Blueprint (Step 3.5.2)                    │
│                                                                        │
│  ✗ NO WebSockets or TCP socket handling                                │
│  ✗ NO Real-time message broadcasting or fanout                         │
│  ✗ NO Gateway connection manager implementation                        │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ consumes and powers
┌───────────────────────────────────▼────────────────────────────────────┐
│                    PHASE 5: REAL-TIME INFRASTRUCTURE                   │
│                                                                        │
│  ✓ WebSocket Upgrade Handlers & Transport Protocol                     │
│  ✓ Connection Manager (in-memory socket registry per node)             │
│  ✓ WebSocket Heartbeat Ping/Pong Wire Framing                          │
│  ✓ Redis Pub/Sub / Streams for Inter-Node Broadcast Fanout             │
│  ✓ Real-Time Push Events: "friend_online", "friend_offline"            │
│  ✓ Room Clustering, Synchronization, and Host Election                 │
│  ✓ Client Reconnect, Backoff, and Recovery Handshake                   │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 20. Open Decisions

No unresolved Presence domain decisions remain for Step 3.5.1.

The following product decisions are now resolved and adopted by this document:

| ID | Decision | Selected Option | Summary of Adopted Policy |
|---|---|---|---|
| **OD-1** | Default Presence Visibility Scope | **Option A (Friends Only)** | Unconfigured users share real-time presence and last seen only with confirmed mutual friends. |
| **OD-2** | Contextual / Rich Presence Representation | **Option B (Separate Activity Overlay)** | Core presence remains `online`/`away`/`offline`. Rich states ("Watching", "In Party") are separate contextual overlays. |
| **OD-3** | Reconnection / Flapping Grace Period | **Option B (10 seconds)** | Disconnecting grace period of 10 seconds prevents offline flickering during page refresh or quick reconnects. |
| **OD-4** | Idle / Away State Transition Mechanism | **Option C (Hybrid)** | Client reports DOM/OS idle threshold (e.g. 10m), combined with manual user toggle override ("Set as Away"). |
| **OD-5** | "Invisible" / "Appear Offline" Semantics | **Option B (Privacy Mask)** | Treated as a privacy visibility projection mask. The user remains `online` internally for notifications and services. |
| **OD-6** | Multi-Device Status Conflict Precedence | **Option A (Highest Activity Wins)** | Precedence is $\text{Online} > \text{Away} > \text{Offline}$. If any device is active, user aggregate presence is `online`. |

### Detailed Resolution Notes

#### OD-1: Default Presence Visibility Scope
- **Resolution:** **Option A (Friends Only)**.
- **Rationale:** StreamTogether is a watch-party social platform. Defaulting presence visibility to confirmed mutual friends protects user privacy, prevents stalking/harassment by strangers or searchers, and ensures that presence queries cleanly align with the existing `internal/friends` social graph. Non-friends see `offline` unless a temporary room co-presence context applies.

#### OD-2: Representation of Rich / Contextual Presence ("Watching", "In Party")
- **Resolution:** **Option B (Separate Activity Overlay)**.
- **Rationale:** Avoids polluting the core availability state machine with room or playback lifecycles. Core presence tracks network reachability (`online`, `away`, `offline`), while an optional `activity: { type, title, party_id }` overlay is attached by higher-level application services.

#### OD-3: Reconnection / Flapping Grace Period Duration
- **Resolution:** **Option B (10 seconds)**.
- **Rationale:** 10 seconds provides the ideal buffer for browser page reloads, tab navigation, and brief Wi-Fi handshakes without leaving users appearing "ghost online" for long periods after closing their application.

#### OD-4: Client Idle vs. Server Inferred Inactivity
- **Resolution:** **Option C (Hybrid)**.
- **Rationale:** Clients detect OS/DOM inactivity and dispatch idle signals to set `away`, while users can also manually toggle "Away" in client UI. The backend remains decoupled from DOM event tracking while respecting user intent.

#### OD-5: "Invisible" / "Appear Offline" Placement and Semantics
- **Resolution:** **Option B (Privacy Mask)**.
- **Rationale:** Modeling "Appear Offline" as a privacy filter rather than a false domain state ensures the backend can still reliably deliver party invites, direct messages, and system security alerts while external observers see the user as `offline`.

#### OD-6: Multi-Device Status Precedence Conflict Resolution
- **Resolution:** **Option A (Highest Activity Wins)**.
- **Rationale:** Evaluates aggregate state across all active presence connections as $\text{online} > \text{away} > \text{offline}$. An idle background desktop tab will never erroneously downgrade an actively engaged mobile or extension user to `away`.
