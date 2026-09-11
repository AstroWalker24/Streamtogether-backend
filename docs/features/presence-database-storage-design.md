---
title: Presence Database and Storage Design
description: Concrete persistence and ephemeral storage architecture for the Presence domain, detailing Redis key structures, PostgreSQL durability, TTL semantics, and multi-device aggregation.
ms.date: 2026-09-11
status: draft
phase: 3 — Social Graph
step: 3.5.2
depends-on: presence-domain-design.md, authentication-database-schema.md, profile-database-schema.md, friends-domain-design.md
---

# Presence Database and Storage Design

**Document status:** Draft  
**Based on:** [docs/features/presence-domain-design.md](docs/features/presence-domain-design.md)  
**Target databases:** Redis 7.0+ (ephemeral presence store) & PostgreSQL 15+ (durable persistence)  
**Migration tool:** `golang-migrate/migrate` (file-based, [internal/database/migrations](internal/database/migrations))  
**Expected migration number:** `000018`  
**Expected migration name:** `000018_create_user_presences`

## Table of Contents

1. [Purpose and Architectural Foundation](#1-purpose-and-architectural-foundation)
2. [Storage Ownership and Classification](#2-storage-ownership-and-classification)
3. [Redis vs. PostgreSQL Decision](#3-redis-vs-postgresql-decision)
4. [Redis Key and Data Structure Strategy](#4-redis-key-and-data-structure-strategy)
5. [Per-Connection State Representation](#5-per-connection-state-representation)
6. [Effective User Presence and Aggregation Storage](#6-effective-user-presence-and-aggregation-storage)
7. [TTL, Heartbeat, and Expiration Model](#7-ttl-heartbeat-and-expiration-model)
8. [Last Seen Persistence Model](#8-last-seen-persistence-model)
9. [Away State and Manual Overrides](#9-away-state-and-manual-overrides)
10. [Bulk Presence Read Architecture](#10-bulk-presence-read-architecture)
11. [Visibility and Privacy Storage Boundary](#11-visibility-and-privacy-storage-boundary)
12. [Contextual Activity Overlay Boundary](#12-contextual-activity-overlay-boundary)
13. [Concurrency, Atomicity, and Lua Scripts](#13-concurrency-atomicity-and-lua-scripts)
14. [Idempotency and Deduplication](#14-idempotency-and-deduplication)
15. [Horizontal Scaling and Cluster Node Ownership](#15-horizontal-scaling-and-cluster-node-ownership)
16. [Failure Modes and Graceful Degradation](#16-failure-modes-and-graceful-degradation)
17. [Restart Semantics and State Recovery](#17-restart-semantics-and-state-recovery)
18. [Capacity and Performance Analysis](#18-capacity-and-performance-analysis)
19. [PostgreSQL Durable Schema Specification](#19-postgresql-durable-schema-specification)
20. [Migration Plan](#20-migration-plan)
21. [Phase Boundary (Phase 3.5 vs. Phase 5)](#21-phase-boundary-phase-35-vs-phase-5)
22. [Open Decisions](#22-open-decisions)

---

## 1. Purpose and Architectural Foundation

This document specifies the concrete storage architecture for the Presence domain established in [docs/features/presence-domain-design.md](docs/features/presence-domain-design.md). It bridges the conceptual domain model with the physical storage engines: the shared Redis infrastructure ([docs/15-redis-infrastructure.md](docs/15-redis-infrastructure.md)) and PostgreSQL 15+.

The design strictly preserves the finalized decisions from Phase 3.5.1:
- **Core presence states:** `online`, `away`, `offline`.
- **Contextual activity:** "Watching", "In Party", or playback states are handled as separate contextual overlays (`UserActivity`), never polluting the core availability state machine.
- **Default visibility:** Friends Only (OD-1).
- **Disconnecting grace period:** 10 seconds (OD-3) to eliminate flapping on browser reloads.
- **Away detection:** Hybrid model (client-reported DOM/OS idle + manual user toggle) (OD-4).
- **"Appear Offline":** Modeled as a privacy visibility projection mask, not a core presence state (OD-5).
- **Multi-device aggregation precedence:** $\text{online} > \text{away} > \text{offline}$ (OD-6).
- **Multiple simultaneous connections:** Fully supported per user, device, and session.
- **No changes to Auth entities:** `users`, `sessions`, and `devices` tables remain untouched. Heartbeats never mutate `sessions.last_active_at`.
- **Design-only:** No migrations, code, or background workers are implemented in this step.

---

## 2. Storage Ownership and Classification

Every element of Presence state has an explicit storage tier, owner, and lifecycle.

| State Element | Storage Tier | Storage Structure | Owner | Lifecycle & Durability |
|---|---|---|---|---|
| **Effective User Presence** | Redis (Cached) / Derived | Hash: `presence:u:{user_id}` | Presence Domain | Ephemeral. Derived on state mutation and cached for $O(1)$ reads. Auto-expires if connections die. |
| **Active Connection Leases** | Redis (Authoritative) | Sorted Set: `presence:u:{user_id}:leases` | Presence Domain | Ephemeral. Score is Unix expiry timestamp. Auto-pruned. |
| **Connection Metadata** | Redis (Authoritative) | Hash: `presence:u:{user_id}:conns` | Presence Domain | Ephemeral. Keyed by `connection_id`. Disappears when connection disconnects or lease expires. |
| **Connection Node Index** | Redis (Secondary) | Set: `presence:node:{node_id}:conns` | Phase 5 Real-Time Gateway | Ephemeral. Tracks connections hosted on a gateway cluster node for bulk cleanup on node crash. |
| **Disconnect Grace Timer** | Redis (Transitional) | String: `presence:u:{user_id}:grace` | Presence Domain | Ephemeral. Created with a 10-second TTL when connection count hits 0. Cancelled if user reconnects. |
| **`last_seen_at`** | **PostgreSQL (Durable)** & Redis (Cache) | Table: `user_presences.last_seen_at` & Hash field | Presence Domain | **Durable**. Written to PostgreSQL upon final offline transition; cached in Redis for fast access. |
| **Manual Presence Preference** | Redis (Cache) & PostgreSQL (Future Privacy) | Hash field `pref` | Presence / Privacy | Ephemeral override in Redis; user privacy settings stored durably in Privacy domain. |
| **Contextual Activity Overlay** | Redis (Party/Playback) | Independent keys (e.g. `party:{id}:members`) | Party / Playback Domain | Owned by Phase 4. Never stored in core presence keys. Combined dynamically at the API gateway layer. |
| **Visibility / Blocking Masks** | PostgreSQL / Cached | Friends & Blocking tables | Friends / Privacy | Evaluated at read time. Presence domain does not store friend or block graphs. |

---

## 3. Redis vs. PostgreSQL Decision

### Why Redis is the Primary Ephemeral Presence Store

1. **Extreme Write Volume and In-Memory Throughput:**
   - With 10,000 concurrent active users sending heartbeats every 15 seconds, the system sustains $\approx 667$ heartbeat writes per second. At peak (50,000 concurrent users), this reaches $\approx 3,333$ writes/second.
   - Performing this volume of updates on PostgreSQL would induce severe write amplification, WAL saturation, MVCC row version churn, and aggressive autovacuum thrashing on hot tables.
   - Redis executes in-memory operations in single-digit microseconds, comfortably handling tens of thousands of ops/second per core.
2. **Native Key Expiration (TTL):**
   - Distributed connection liveness is lease-based. Redis provides native, engine-level expiration for keys and Sorted Sets (`ZREMRANGEBYSCORE`), eliminating the need for periodic database polling queries (`SELECT ... WHERE expires_at < NOW()`).
3. **Atomic Multi-Key Primitives:**
   - Redis pipelines and Lua scripts permit atomic registration, heartbeat updates, lease extension, and state aggregation in a single non-blocking network hop.
4. **Natural Lifecycle Alignment:**
   - Active network sockets are inherently transient. If the application or server node crashes, all TCP sockets drop; persisting raw connection IDs in a relational database would create persistent orphan records that require complex crash-recovery sweeps.

### Why PostgreSQL is Retained for Durable `last_seen_at`

1. **Survival Across Reboots and Maintenance:**
   - When a user has been offline for three weeks, their `last_seen_at` timestamp must not vanish during a Redis flush, restart, or cache eviction.
2. **Low Write Frequency:**
   - In stark contrast to heartbeats, `last_seen_at` is persisted to PostgreSQL **only when a user transitions to offline** (after the 10-second grace period) or lazily throttled to disk at most once every 30 minutes during prolonged continuous activity.
   - For 10,000 concurrent users, offline transitions occur at $\approx 1\text{--}5$ writes/second, which PostgreSQL handles with negligible I/O impact.
3. **Referential Integrity:**
   - Binds directly to `users(id)` with `ON DELETE CASCADE`, ensuring compliance with account lifecycle, GDPR/data deletion policies, and soft-delete retention rules.

---

## 4. Redis Key and Data Structure Strategy

All presence keys in Redis use the project-standard prefix `presence:` and operate in Redis Database `0` (as configured in [docs/15-redis-infrastructure.md](docs/15-redis-infrastructure.md)).

```
┌────────────────────────────────────────────────────────────────────────┐
│                        REDIS KEY ARCHITECTURE                          │
├────────────────────────────────────────────────────────────────────────┤
│ 1. Aggregate User Presence Cache (Hash)                                │
│    Key: presence:u:{user_id}                                           │
│    Fields: status, last_seen, updated_at, manual_pref, conn_count      │
│    TTL: 60s (sliding, refreshed on heartbeat)                          │
├────────────────────────────────────────────────────────────────────────┤
│ 2. Active Connection Leases (Sorted Set)                               │
│    Key: presence:u:{user_id}:leases                                    │
│    Members: {connection_id}                                            │
│    Score:   Unix epoch expiration timestamp (now + 45s)                │
│    TTL: 60s (sliding, refreshed on heartbeat)                          │
├────────────────────────────────────────────────────────────────────────┤
│ 3. Connection Metadata Directory (Hash)                                │
│    Key: presence:u:{user_id}:conns                                     │
│    Fields: {connection_id} -> JSON / MessagePack metadata payload      │
│    TTL: 60s (sliding, refreshed on heartbeat)                          │
├────────────────────────────────────────────────────────────────────────┤
│ 4. Disconnect Flapping Grace Period (String Sentinel)                  │
│    Key: presence:u:{user_id}:grace                                     │
│    Value: timestamp                                                    │
│    TTL: 10s (fixed, non-sliding)                                       │
├────────────────────────────────────────────────────────────────────────┤
│ 5. Cluster Node Active Connections (Set)                               │
│    Key: presence:node:{node_id}:conns                                  │
│    Members: {connection_id}:{user_id}                                  │
│    TTL: None (managed by gateway node lifecycle)                       │
└────────────────────────────────────────────────────────────────────────┘
```

### Key Breakdown and Schemas

#### 1. `presence:u:{user_id}` (Redis Hash)
Stores the pre-computed aggregate state for fast single-user and bulk-user lookups.
- **`status`**: String (`online`, `away`, `offline`).
- **`last_seen`**: RFC3339 timestamp string of latest known activity.
- **`updated_at`**: RFC3339 timestamp of last aggregation calculation.
- **`manual_pref`**: String (`none`, `away`).
- **`conn_count`**: Integer string representing current count of active leases.
- **TTL:** 60 seconds (extended on each heartbeat; guarantees automatic eviction if all server nodes crash).

#### 2. `presence:u:{user_id}:leases` (Redis Sorted Set)
Authoritative index of active leases for a user.
- **Members:** `connection_id` (UUID string).
- **Score:** Unix timestamp in seconds representing absolute lease expiration ($T_{\text{now}} + 45\text{s}$).
- **Purpose:** Enables instant $O(\log N)$ cleanup of expired connections via `ZREMRANGEBYSCORE presence:u:{user_id}:leases 0 {now}`.

#### 3. `presence:u:{user_id}:conns` (Redis Hash)
Detailed metadata for each active connection.
- **Fields:** `connection_id` $\rightarrow$ JSON-encoded connection string (see Section 5).
- **Purpose:** Stores routing and device attributes needed for multi-device aggregation and session revocation without querying Postgres.

#### 4. `presence:u:{user_id}:grace` (Redis String)
Sentinel key created when a user's active connection count drops from 1 to 0.
- **Value:** RFC3339 timestamp of disconnect initiation.
- **TTL:** Exactly 10 seconds.
- **Purpose:** Prevents premature offline broadcasts while the user refreshes their browser.

#### 5. `presence:node:{node_id}:conns` (Redis Set)
Tracks which connections belong to which physical gateway instance.
- **Members:** `{connection_id}:{user_id}`.
- **Purpose:** If gateway `node_id` terminates abruptly, a surviving node or cleanup worker can iterate this set and cleanly purge orphaned leases.

---

## 5. Per-Connection State Representation

The field value stored under `presence:u:{user_id}:conns` for a given `connection_id` contains a compact JSON document:

```json
{
  "conn_id": "4a7b9213-9a3b-410e-948f-4cf641ec96a1",
  "user_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "session_id": "e3b0c442-98fc-1c14-9afbf4c8996fb924",
  "device_id": "8c42b931-419b-4e12-b103-62e91244e891",
  "node_id": "gateway-pod-us-east-1a-7b4f",
  "platform": "web",
  "state": "online",
  "connected_at": "2026-09-11T14:30:00Z",
  "last_heartbeat": "2026-09-11T14:32:15Z"
}
```

### Field Definitions and Privacy Constraints

| Field | Type | Description | Visibility Constraint |
|---|---|---|---|
| `conn_id` | UUID string | Globally unique identifier generated by the WebSocket gateway on handshake. | **Internal only**. Never exposed via API. |
| `user_id` | UUID string | Root user identity ([internal/auth/domain/user.go](internal/auth/domain/user.go#L21)). | Public in social contexts. |
| `session_id` | UUID string | Auth session anchor ([internal/auth/domain/session.go](internal/auth/domain/session.go#L14)). | **Internal only**. Never exposed via API. |
| `device_id` | UUID string | Registered device endpoint ([internal/auth/domain/device.go](internal/auth/domain/device.go#L21)). | **Internal only**. Never exposed via API. |
| `node_id` | String | Server cluster hostname or pod ID hosting the TCP socket. | **Internal only**. Infrastructure detail. |
| `platform` | String | Client platform (`web`, `desktop`, `extension`, `ios`, `android`). | Internal; exposed only to self-profile if requested. |
| `state` | String | Connection-level state (`online`, `away`). | Aggregated into user state. |
| `connected_at` | RFC3339 | Connection registration time. | Internal diagnostic timestamp. |
| `last_heartbeat` | RFC3339 | Time of most recent heartbeat received. | Internal diagnostic timestamp. |

---

## 6. Effective User Presence and Aggregation Storage

### Write-Time Aggregation vs. Read-Time Calculation

| Approach | Read Latency | Write Cost | Consistency | Evaluation |
|---|---|---|---|---|
| **Read-Time Calculation** | High ($O(N \cdot M)$ for friend lists) | Low ($O(1)$) | Guaranteed | **Rejected.** Unacceptable latency when rendering friends lists of 100+ friends. |
| **Write-Time Aggregation** | Minimal ($O(1)$ single key lookup) | Slightly higher ($O(M)$ where $M$ is user's device count $\le 5$) | Maintained via atomic Lua scripts | **Adopted.** Presence is read 100x more often than state transitions occur. |

### The Aggregation State Machine

When a connection changes state, disconnects, or registers:
1. Expired leases in `presence:u:{user_id}:leases` are purged.
2. The remaining connection states in `presence:u:{user_id}:conns` are inspected.
3. If user has set an explicit manual preference (`manual_pref = "away"`), the user is `away`.
4. Otherwise, aggregate status evaluates using the adopted precedence rule (OD-6):
   $$\text{online} > \text{away} > \text{offline}$$
   - If $\ge 1$ connection has `state = "online"`, effective status is `online`.
   - Else if $\ge 1$ connection has `state = "away"`, effective status is `away`.
   - Else (0 connections), status enters `disconnecting` grace period or `offline`.
5. The computed result is stored immediately into `presence:u:{user_id}`.

---

## 7. TTL, Heartbeat, and Expiration Model

The storage model coordinates multiple distinct timers to prevent stale states, ghost sessions, and network flapping.

```
Timeline: Normal Heartbeat vs. Disconnect vs. Crash
─────────────────────────────────────────────────────────────────────────────
0s                15s               30s               45s
├─────────────────┼─────────────────┼─────────────────┤
▲                 ▲                 ▲                 ▲
Client Connects   Heartbeat 1       Heartbeat 2       Lease Expired
Lease TTL = 45s   Lease TTL = 45s   Lease TTL = 45s   (If no heartbeat)
Grace = None      Grace = None      Grace = None      Grace Timer = 10s Starts
─────────────────────────────────────────────────────────────────────────────
45s                                 55s
├───────────────────────────────────┤
▲                                   ▲
Grace Timer (10s) Starts            Grace Timer Expires
User remains "online"               Status -> "offline"
(Flapping protection)               Commit last_seen_at to PostgreSQL
```

### Parameter Specifications

| Parameter | Value | Scope | Storage Mechanism | Purpose |
|---|---|---|---|---|
| **Heartbeat Interval** | 15s | Client $\rightarrow$ Server | Client timer | Routine ping interval over WebSocket. |
| **Lease Duration** | 45s | Server $\rightarrow$ Redis | Score in Sorted Set | Allows 2 dropped heartbeats before marking connection dead. |
| **Flapping Grace Period** | 10s | User Aggregate | Key `presence:u:{user_id}:grace` with TTL=10s | Absorbs page refreshes and Wi-Fi handshakes (OD-3). |
| **Redis Key Cache TTL** | 60s | User Aggregate & Sets | `EXPIRE presence:u:{user_id} 60` | Safety net: automatically frees memory if all servers die. |
| **Lazy DB Flush Interval** | 30m | User $\rightarrow$ PostgreSQL | In-memory / Redis throttled timestamp | Ensures `last_seen_at` is preserved during multi-hour sessions. |

### Disconnect Scenarios

1. **Clean Disconnect (Browser Tab Closed):**
   - Gateway receives WebSocket Close frame.
   - Gateway calls `RemoveConnection(conn_id)`.
   - `conn_id` is removed from Redis immediately.
   - If remaining active connections $> 0$, re-aggregate effective state immediately.
   - If remaining active connections $= 0$, create sentinel `presence:u:{user_id}:grace` with TTL 10s. Do **not** mark user `offline` yet.
   - After 10s, if no new connection arrives, the background reaper marks user `offline` and commits `last_seen_at` to PostgreSQL.

2. **Abrupt Network Loss / Mobile Background Kill:**
   - No Close frame received.
   - Client ceases heartbeats.
   - Score in `presence:u:{user_id}:leases` expires after 45s.
   - Next sweep or read removes the dead connection and triggers the 10s grace period.

---

## 8. Last Seen Persistence Model

### Definition and Semantic Meaning
`last_seen_at` is the timestamp representing the **last moment the user was confirmed to be actively connected to StreamTogether**.

### Dual-Layer Storage Pattern

```
                       ┌─────────────────────────────────────────┐
                       │          Client Heartbeat Ping          │
                       └────────────────────┬────────────────────┘
                                            │ Every 15s
                                            ▼
                       ┌─────────────────────────────────────────┐
                       │           Redis (Ephemeral)             │
                       │  - Update presence:u:{id} [last_seen]   │
                       │  - Refresh lease TTLs                   │
                       └────────────────────┬────────────────────┘
                                            │ On final disconnect
                                            │ (after 10s grace period)
                                            ▼
                       ┌─────────────────────────────────────────┐
                       │         PostgreSQL (Durable)            │
                       │  - UPDATE user_presences                │
                       │    SET last_seen_at = $2                │
                       │    WHERE user_id = $1                   │
                       └─────────────────────────────────────────┘
```

1. **Hot Path (Redis):** While the user is online, their latest activity timestamp is held in Redis field `presence:u:{user_id} -> last_seen`. It is never written to PostgreSQL on every heartbeat.
2. **Cold Path (PostgreSQL):**
   - When the user's last connection drops and the 10-second grace period elapses, the final `last_seen` timestamp is written to PostgreSQL table `user_presences`.
   - **Continuous Session Safety:** For users who remain logged in continuously for hours or days, `last_seen_at` is flushed to PostgreSQL at most once every 30 minutes to prevent loss of temporal accuracy in case of catastrophic data center outage.

---

## 9. Away State and Manual Overrides

The storage layer supports the hybrid away model adopted in OD-4:

### 1. Client-Reported Inactivity (Auto-Idle)
- Client measures local OS/DOM input (keyboard, mouse, touch).
- After 10 minutes of inactivity, client sends `SetConnectionState(conn_id, "away")`.
- Gateway updates connection metadata: `state = "away"` in `presence:u:{user_id}:conns`.
- Multi-device aggregation evaluates: if another connection is still `"online"`, aggregate remains `"online"`. If all connections are `"away"`, aggregate becomes `"away"`.

### 2. Manual User Override ("Set as Away")
- User selects "Away" in client status menu.
- Gateway sets `manual_pref = "away"` in `presence:u:{user_id}`.
- Manual preference overrides individual connection activity; effective status is forced to `away`.
- When user selects "Reset Status" or "Online", `manual_pref` is set back to `"none"`, returning the user to auto-derived status.

---

## 10. Bulk Presence Read Architecture

### The Query Challenge
When a user opens their Friends drawer, the frontend requests presence for 50–100 friends in a single view. Executing $N$ round-trips to Redis or PostgreSQL would create high latency ($O(N)$ network hops).

### Storage-Optimized Bulk Lookup Flow

Because `presence:u:{user_id}` is stored as a Redis **HASH** (carrying `status`, `last_seen`, `updated_at`, `manual_pref`, `conn_count`), string commands like `MGET` cannot be used directly on hash keys.

Instead, the bulk read leverages a **single Redis pipeline** executing batched `HMGET` (or `HGETALL`) commands:

```
Client Request: GetPresenceForUsers([UserA, UserB, UserC, UserD])
                                │
                                ▼
         ┌────────────────────────────────────────────────────────┐
         │ Single Pipelined Batch of HMGET Commands               │
         │ (HMGET presence:u:{id} status last_seen updated_at ...)│
         └──────────────────────┬─────────────────────────────────┘
                                │ (Single Redis network round-trip)
             ┌──────────────────┴──────────────────┐
             ▼                                     ▼
     Keys Found in Redis                   Keys Missing (Offline)
  [UserA: online, UserB: away]              [UserC, UserD: offline]
             │                                     │
             │                             ┌───────▼────────────────────────┐
             │                             │ Batch SELECT from PostgreSQL:  │
             │                             │ SELECT user_id, last_seen_at   │
             │                             │ FROM user_presences            │
             │                             │ WHERE user_id = ANY($1)        │
             │                             └───────┬────────────────────────┘
             │                                     │
             └──────────────────┬──────────────────┘
                                │
                                ▼
         ┌──────────────────────────────────────────────┐
         │ Filter through Privacy & Blocking Facades    │
         └──────────────────────┬───────────────────────┘
                                │
                                ▼
         Return map[uuid.UUID]*UserPresenceView to Client
```

### Protocol and Operational Details
1. **Redis Operation Used:** Pipelined `HMGET presence:u:{user_id} status last_seen updated_at manual_pref` (or pipelined `HGETALL`). In Go (`go-redis`), `pipe := r.Client().Pipeline()` enqueues $N$ `HMGET` commands and dispatches them in a **single socket write and read**.
2. **Network Round-Trips:** Exactly **1 Redis round-trip** for all $N$ users, regardless of batch size.
3. **Fields Retrieved:**
   - `status`: Base presence state (`online`, `away`).
   - `last_seen`: Ephemeral activity timestamp if available.
   - `updated_at`: Evaluation timestamp.
   - `manual_pref`: User-level manual override.
4. **Behavior for Missing Keys:**
   - In Redis, an `HMGET` on a non-existent key returns `[nil, nil, nil, nil]` without error.
   - Any user whose hash key does not exist or whose fields are empty is identified as **offline**.
5. **PostgreSQL Fallback for Durable `last_seen_at`:**
   - For all users identified as offline from the Redis pipeline, their IDs are collected into a single slice `missingUserIDs`.
   - If `missingUserIDs` is non-empty, a **single indexed batch query** executes against PostgreSQL:
     ```sql
     SELECT user_id, last_seen_at
     FROM user_presences
     WHERE user_id = ANY($1);
     ```
   - If a user has no record in `user_presences` either (e.g. newly registered user who has never logged in), their status is `offline` and `last_seen_at = nil`.
6. **Complexity & Total Round-Trips:**
   - **Redis:** 1 network round-trip ($O(N)$ execution in Redis memory; $<1\text{ms}$ for 100 users).
   - **PostgreSQL:** At most 1 network round-trip via primary key index (`WHERE user_id = ANY($1)`; $<2\text{ms}$).
   - **Total Round-Trips:** Exactly $1\text{--}2$ network round-trips total, completely eliminating $N+1$ query cascades.

---

## 11. Visibility and Privacy Storage Boundary

### Separation of Truth and Projection
Presence storage strictly isolates **physical availability** from **privacy permissions**:

1. **No Privacy Columns in Presence Storage:** Neither Redis presence keys nor the PostgreSQL `user_presences` table store block lists, friend lists, or visibility policies.
2. **"Appear Offline" (OD-5):**
   - The user's actual status in Redis remains `online`.
   - The user's Privacy setting (`appear_offline = true`) is stored in PostgreSQL (owned by Privacy / Settings).
   - During read aggregation, the Visibility Facade inspects the target's privacy settings:
     - If `appear_offline == true` and `observer_id != target_id`, the facade rewrites the returned status to `offline` and omits `last_seen_at`.
     - The user themselves (`observer_id == target_id`) receives their true unmasked status (`online` with indicator `applied_mask: appear_offline`).

---

## 12. Contextual Activity Overlay Boundary

Contextual activity represents dynamic watch-party engagement (e.g., "Watching Stranger Things S1:E3" or "In Party #42").

### Storage Isolation Rules
- **No Activity in Core Presence Keys:** `presence:u:{user_id}` never contains party IDs, video URLs, or playback timestamps.
- **Owned by Phase 4:** Watch party participation is stored in party-specific Redis keys (e.g., `party:{party_id}:members`).
- **Dynamic Assembly:** When an authenticated friend queries presence, the application layer queries Presence for availability, queries Party for active room membership, and returns a unified composite view:
  ```json
  {
    "user_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
    "status": "online",
    "activity": {
      "type": "party",
      "party_id": "c71e2182-3d8b-4b2a-8ef9-8a8b1a8f9a2e",
      "title": "Stranger Things"
    }
  }
  ```

---

## 13. Concurrency, Atomicity, and Lua Scripts

To prevent race conditions across distributed gateway nodes, multi-step state mutations in Redis must execute atomically.

### Race Condition Scenarios

1. **Disconnect / Reconnect Race:**
   - Tab 1 disconnects (Connection A).
   - Tab 2 opens (Connection B).
   - Connection A cleanup executes *after* Connection B registers.
   - **Risk:** Without atomic versioning, Connection A cleanup could erroneously mark the user `offline`.
2. **Concurrent Multi-Device Heartbeats:**
   - Phone and Laptop send heartbeats simultaneously.
   - **Risk:** Interleaved read-modify-write cycles could overwrite lease data.

### Lua Script Blueprints (Design Specification)

All state mutations in Redis will be orchestrated via four atomic Lua scripts in the repository implementation (Phase 3.5.3):

#### 1. `register_connection.lua`
- Atomically adds `conn_id` to `presence:u:{user_id}:leases` with score $T_{\text{now}} + 45\text{s}$.
- Writes connection metadata to `presence:u:{user_id}:conns`.
- Cancels any active grace period (`DEL presence:u:{user_id}:grace`).
- Re-evaluates effective status ($\text{online} > \text{away}$) and updates `presence:u:{user_id}`.
- Refreshes 60s sliding TTL on all user keys.

#### 2. `heartbeat_connection.lua`
- Verifies `conn_id` exists in `presence:u:{user_id}:leases`.
- Updates score in `presence:u:{user_id}:leases` to $T_{\text{now}} + 45\text{s}$.
- Updates `last_heartbeat` in `presence:u:{user_id}:conns`.
- Refreshes 60s sliding TTL on all user keys.

#### 3. `remove_connection.lua`
- Removes `conn_id` from `presence:u:{user_id}:leases` and `presence:u:{user_id}:conns`.
- Prunes any other expired leases (`ZREMRANGEBYSCORE`).
- Counts remaining active leases (`ZCARD`).
- If remaining leases $> 0$, re-evaluates effective status and updates `presence:u:{user_id}`.
- If remaining leases $== 0$, sets `presence:u:{user_id}:grace` with TTL=10s and marks `status = "disconnecting"`.

#### 4. `prune_stale_leases.lua`
- Iterates user leases and removes members with score $< T_{\text{now}}$.
- If all leases expired and grace period elapsed, transitions `status = "offline"` and returns final `last_seen` timestamp for PostgreSQL persistence.

---

## 14. Idempotency and Deduplication

Network retries and gateway reconnects can deliver duplicate packets. The storage design guarantees idempotency:

| Operation | Deduplication Mechanism | Effect of Repetition |
|---|---|---|
| Duplicate `RegisterConnection` | `HSET` and `ZADD` by unique `connection_id`. | Overwrites metadata with identical values; updates lease score safely. No duplicate count. |
| Duplicate `RecordHeartbeat` | `ZADD` with new timestamp score. | Monotonically advances lease expiry. Safe to repeat indefinitely. |
| Duplicate `RemoveConnection` | `ZREM` and `HDEL` by unique `connection_id`. | Second call finds 0 removed fields; returns success as a no-op. |
| Duplicate `SetAway` | `HSET presence:u:{id} manual_pref away`. | Value remains `"away"`. No side effects. |

---

## 15. Horizontal Scaling and Cluster Node Ownership

In Phase 5, the WebSocket gateway will scale horizontally across multiple instances (e.g., `gateway-pod-1`, `gateway-pod-2`).

```
Gateway Node 1                          Gateway Node 2
  - Holds TCP Socket A (Laptop)           - Holds TCP Socket B (Phone)
  - conn_id: A                            - conn_id: B
         │                                       │
         ▼                                       ▼
  ┌────────────────────────────────────────────────────────┐
  │                 Shared Redis Cluster                   │
  │  presence:u:{user_id}:leases                           │
  │    ├── conn_id A (score: now+45s, node: gateway-1)     │
  │    └── conn_id B (score: now+45s, node: gateway-2)     │
  └────────────────────────────────────────────────────────┘
```

1. **Shared State:** All gateway nodes connect to the same shared Redis instance/cluster. Presence state is never kept solely in local node memory.
2. **Node Tracking:** Each node tracks its own connections in `presence:node:{node_id}:conns`.
3. **Graceful Node Shutdown:** When `gateway-pod-1` undergoes rolling deployment, it iterates its local connections, sends WebSocket Close frames to clients, and calls `RemoveConnection`.
4. **Ungraceful Node Crash:** If `gateway-pod-1` crashes instantly (e.g., OOM kill, hardware fault):
   - Surviving nodes do not need to panic or synchronously scan all connections.
   - Leases held by sockets on the crashed node will naturally expire in Redis after 45 seconds.
   - When clients reconnect to `gateway-pod-2`, they receive new connection IDs, seamlessly restoring presence.

---

## 16. Failure Modes and Graceful Degradation

### 1. Redis Outage
- **Impact:** Real-time presence updates, active connection tracking, and instant availability checks become unavailable.
- **Degradation Policy:**
  - Presence service fails gracefully degraded; it **must not** crash the backend or block HTTP requests.
  - Presence queries catch Redis connection errors and return a degraded response: `status = "unknown"` or fallback to PostgreSQL `last_seen_at`.
  - **Zero Heartbeat Fallback to Postgres:** The application layer must **never** redirect high-frequency heartbeats to PostgreSQL if Redis is down. That would immediately exhaust the PostgreSQL connection pool and take down authentication and payments.

### 2. PostgreSQL Outage
- **Impact:** `last_seen_at` cannot be committed to disk; historical offline timestamps cannot be loaded.
- **Degradation Policy:**
  - Real-time online/away presence continues functioning 100% normally via Redis.
  - Disconnect writes to `last_seen_at` are kept in Redis with extended TTL (e.g., 24 hours) and placed in an in-memory retry queue until PostgreSQL connectivity recovers.

---

## 17. Restart Semantics and State Recovery

### 1. Redis Server Restart
- Redis is treated as an **ephemeral cache** for active presence.
- If Redis restarts and loses memory state:
  - All active connection leases vanish.
  - Users appear temporarily `offline`.
  - Within 15 seconds (one client heartbeat cycle), all active clients send heartbeats/re-register, fully restoring active connection sets in Redis automatically.
  - Historical `last_seen_at` remains intact in PostgreSQL.

### 2. Application / Gateway Restart
- TCP sockets drop.
- Clients initiate exponential backoff reconnection.
- Reconnection creates fresh connection leases. The 10-second grace period prevents friends from seeing offline blips during rapid rolling restarts.

---

## 18. Capacity and Performance Analysis

### Workload Profile (Assumed 25,000 Concurrent Active Users)

| Metric | Estimated Value | Calculation Basis |
|---|---|---|
| Concurrent active users | 25,000 | Assumed baseline peak load |
| Concurrent WebSocket connections | 35,000 | 25,000 users $\times$ 1.4 average devices/tabs |
| Heartbeat write rate (user basis) | $\approx 1,667\text{ ops/sec}$ | $25,000 \text{ users} \div 15\text{s}$ interval |
| Heartbeat write rate (connection basis) | $\approx 2,333\text{ ops/sec}$ | $35,000 \text{ connections} \div 15\text{s}$ interval |
| Friend drawer presence reads | $\approx 250\text{ ops/sec}$ | Navigation / periodic UI refresh |
| Redis memory per active user | $\approx 450\text{ bytes}$ | 1 user hash + 1 lease set + 1 conn hash |
| Total Redis memory for 25k users | $\approx 11.25\text{ MB}$ | Extremely lightweight footprint |
| PostgreSQL write rate (`last_seen_at`) | $\approx 2\text{--}5\text{ writes/sec}$ | Disconnect events after 10s grace period |

### Performance Strategy
- All Redis commands run against indexed keys or small sets (max 5 connections per user). Time complexity is $O(1)$ or $O(\log K)$ where $K \le 5$.
- Bulk reads use a pipelined batch of `HMGET` commands over a single round-trip to avoid multiple TCP round-trips.
- PostgreSQL table `user_presences` has a single B-tree index on the primary key `user_id`.

---

## 19. PostgreSQL Durable Schema Specification

The durable store requires a dedicated table for long-term presence data. This table is owned by the Presence domain and contains only durable attributes.

### Table Definition: `user_presences`

```sql
CREATE TABLE user_presences (
    -- Direct 1:1 binding to the user identity
    user_id         UUID        NOT NULL,

    -- Durable temporal record of when user was last online
    last_seen_at    TIMESTAMPTZ NULL,

    -- Audit timestamp tracking record modification
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Primary key directly on user_id ensures 1:1 cardinality
    CONSTRAINT pk_user_presences
        PRIMARY KEY (user_id),

    -- Referential integrity; cascading delete removes presence when user is deleted
    CONSTRAINT fk_user_presences_user_id
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
```

### Column Specifications

| Column | Type | Nullability | Default | Mutability | Purpose |
|---|---|---|---|---|---|
| `user_id` | `UUID` | `NOT NULL` | None | Immutable | Foreign key to `users(id)`. Primary key of this table. |
| `last_seen_at` | `TIMESTAMPTZ` | `NULL` | None | Mutable | Recorded timestamp when user was last confirmed online. `NULL` if user has never logged in since registration. |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL` | `NOW()` | Mutable | Automatically maintained by `fn_set_updated_at()`. |

### Constraints and Indexes

1. `pk_user_presences PRIMARY KEY (user_id)`:
   - Enforces exactly one record per user.
   - Automatically generates the B-tree index supporting $O(1)$ lookups and `WHERE user_id = ANY($1)` bulk queries.
2. `fk_user_presences_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`:
   - Guarantees referential integrity without manual cleanup.
3. **No Secondary Indexes Required:** Queries against `user_presences` are always keyed by `user_id` (single user or batch array). No range scans on `last_seen_at` are required initially.

---

## 20. Migration Plan

### Migration Sequence
The existing migration sequence is:
- `000015_create_friendships`
- `000016_create_friend_requests`
- `000017_add_search_indexes`

The next available migration number is **`000018`**.

### Proposed Migration
- **Migration Identifier:** `000018_create_user_presences`
- **Migration Tool:** `golang-migrate/migrate` (file-based)
- **Target Table:** `user_presences`
- **Contents:**
  1. Creates table `user_presences` with constraints specified in Section 19.
  2. Attaches the existing project trigger `trg_user_presences_updated_at` calling `fn_set_updated_at()` (defined in migration `000001_create_users`).
  3. Rollback drops table `user_presences` and its trigger.

*(Note: Per constraints, migration SQL files are not created in this design step.)*

---

## 21. Phase Boundary (Phase 3.5 vs. Phase 5)

| Responsibility | Phase 3.5 (Presence) | Phase 5 (Real-Time Infrastructure) |
|---|---|---|
| **Storage Architecture** | Defined in this document (Redis keys, Sorted Sets, Postgres schema). | Implements Redis Pub/Sub channels for broadcast fanout. |
| **Repository Layer** | Presence repository: implements Redis Lua scripts and Postgres queries (Step 3.5.3). | None (consumes presence repository). |
| **Service Layer** | Presence service: coordinates leases, grace periods, and bulk reads (Step 3.5.4). | None (calls presence service on connect/disconnect). |
| **Network Transport** | None. | WebSocket upgrade, TCP framing, ping/pong frames, Connection Manager. |
| **Broadcasting / Push** | Generates domain event `UserPresenceChangedEvent`. | Subscribes to events, resolves friends list, pushes JSON messages over active WebSockets. |

---

## 22. Open Decisions

All core presence product decisions (OD-1 through OD-6) were finalized in Phase 3.5.1. The following storage-specific implementation decisions are resolved below for Step 3.5.2:

### SD-1: PostgreSQL Provisioning for New Users
- **Question:** Should a row in `user_presences` be created upon user registration (eager) or on first presence disconnect (lazy `UPSERT`)?
- **Options:**
  - *Option A (Eager Insert):* Insert a row into `user_presences` with `last_seen_at = NULL` during registration in [internal/auth](internal/auth).
  - *Option B (Lazy UPSERT):* Use `INSERT ... ON CONFLICT (user_id) DO UPDATE` whenever `last_seen_at` is first committed.
- **Adopted Decision:** **Option B (Lazy UPSERT)**.
- **Rationale:** Keeps the user registration transaction uncoupled from the Presence domain and avoids creating database rows for inactive or unverified accounts.

### SD-2: Storage of the Disconnecting Grace Period
- **Question:** How should the 10-second grace period be scheduled and checked in Redis?
- **Options:**
  - *Option A (Redis Key Expiration):* Set a key `presence:u:{id}:grace` with a 10s TTL.
  - *Option B (Sorted Set Delayed Queue):* Push disconnect jobs into a Redis sorted set `presence:disconnect_queue` with score $= T_{\text{now}} + 10\text{s}$, processed by a worker.
- **Adopted Decision:** **Option A (Redis Key Expiration) for state check, combined with Option B for Phase 5 event trigger**.
- **Rationale:** Option A allows any read operation to immediately know if a user is in a disconnecting grace state in $O(1)$ time without running a queue worker in Phase 3.5.
