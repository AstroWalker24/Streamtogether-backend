---
title: Authentication & Identity Domain Design
description: Architecture design for the Authentication & Identity domain of the StreamTogether backend. Covers user lifecycle, session model, token strategy, device management, roles, permissions, security requirements, and future extension points.
ms.date: 2026-08-07
status: approved
---

# Authentication & Identity Domain Design

## Table of Contents

1. [Overview](#1-overview)
2. [Goals](#2-goals)
3. [Non-Goals](#3-non-goals)
4. [Domain Model](#4-domain-model)
5. [Entity Relationships](#5-entity-relationships)
6. [User Lifecycle](#6-user-lifecycle)
7. [Authentication Flow](#7-authentication-flow)
8. [Session Model](#8-session-model)
9. [Device Model](#9-device-model)
10. [Roles & Permissions](#10-roles--permissions)
11. [Business Rules](#11-business-rules)
12. [Security Requirements](#12-security-requirements)
13. [Error Cases](#13-error-cases)
14. [Future Extensions](#14-future-extensions)
15. [Design Decisions](#15-design-decisions)

---

## 1. Overview

The Authentication & Identity domain is the trust foundation of StreamTogether. Every other domain — parties, playback synchronization, chat, presence — depends on knowing with certainty _who_ is making a request and whether they are permitted to do so.

### Responsibilities

- Establishing and verifying the identity of every user.
- Issuing, rotating, and revoking short-lived access tokens.
- Managing refresh tokens that allow silent reauthentication.
- Tracking active sessions across multiple devices simultaneously.
- Registering and recognizing the devices a user authenticates from.
- Verifying that a user controls the email address they registered with.
- Allowing users to recover access to their account when credentials are lost.
- Providing a role and permission model that all downstream domains can rely on for authorization decisions.
- Producing an auditable record of security-significant events.

### Out of Scope

The domain does not concern itself with anything beyond establishing and maintaining trusted identity. The following are explicitly out of scope:

- Chat, messaging, and real-time communication.
- Party creation, management, and membership.
- Playback synchronization and streaming coordination.
- Friend relationships and social graphs.
- Notifications (the auth domain may _trigger_ notifications for account events, but the delivery mechanism is owned by the notification domain).
- User profile content beyond the fields necessary to authenticate and identify a user (display names, avatars, bios, and preferences are the profile domain's concern).
- Business analytics and product telemetry.

---

## 2. Goals

| # | Goal | Rationale |
|---|------|-----------|
| G1 | Secure authentication | Every credential exchange must be protected against interception, replay, and brute-force attacks. |
| G2 | Multi-device login | Users expect to be authenticated simultaneously on a mobile app, a desktop browser, and potentially a TV client. |
| G3 | Session management | Users must be able to view and revoke any active session from any device. |
| G4 | Token rotation | Refresh tokens must be single-use and rotated on every use to limit the blast radius of a stolen token. |
| G5 | Scalable authorization foundation | The role and permission model must be extensible without requiring schema migrations for every new permission. |
| G6 | Account recovery | Users must have a secure and time-bounded path to regain access through email-based password reset. |
| G7 | Email verification | Accounts must be verified before they gain full access to the platform, preventing throwaway registrations. |
| G8 | Auditability | Security-significant events (login, logout, token rotation, password change, account deletion) must be traceable. |
| G9 | Graceful session expiration | Both idle sessions and absolute session lifetimes must be enforced to limit exposure of long-lived tokens. |
| G10 | Future-proof identity model | The design must accommodate OAuth providers, passkeys, and MFA without breaking existing sessions. |

---

## 3. Non-Goals

The following are explicitly excluded from this domain's current scope:

- **Chat & messaging** — owned by the real-time communication domain.
- **Friend requests & social graph** — owned by the social domain.
- **Party management** — owned by the party domain.
- **Playback synchronization** — owned by the synchronization domain.
- **Push notification delivery** — owned by the notification domain.
- **Business analytics** — owned by the analytics domain.
- **Streaming service OAuth** (e.g., linking a Netflix account) — this is an _integration_ concern, distinct from identity authentication.
- **Content access control** — what a user is allowed to watch is a streaming service concern, not ours.
- **Multi-factor authentication (MFA)** — defined as a future extension in section 14.
- **Single Sign-On (SSO)** — defined as a future extension in section 14.

---

## 4. Domain Model

### 4.1 User

**Purpose**  
The central identity entity. A User represents a real person who has created an account on the platform.

**Responsibilities**
- Holds the canonical identity: unique email address, unique username, and credential (hashed password).
- Carries account state that governs whether authentication is permitted.
- Acts as the root aggregate for all identity-related entities (sessions, devices, tokens, roles).

**Key Attributes**
- A globally unique, immutable identifier.
- A unique, case-insensitive email address (the authentication credential identifier).
- A unique, case-insensitive username (the human-facing identifier shown to other users).
- A cryptographically hashed password (the secret). The plaintext password is never stored.
- Account status: `pending_verification`, `active`, `suspended`, `deleted`.
- Email verification state: whether the registered email has been confirmed.
- Timestamps for creation, last update, and soft deletion.
- A flag indicating whether the account has been soft-deleted.

**Relationships**
- Owns zero or more Sessions.
- Owns zero or more Refresh Tokens.
- Owns zero or more registered Devices.
- Is assigned one or more Roles.
- May have one pending Email Verification token.
- May have one active Password Reset token.

**Lifecycle**  
Created on registration → awaiting email verification → active (after verification) → optionally suspended (by administration) → soft-deleted (on account deletion request, with a grace period before permanent removal).

---

### 4.2 Session

**Purpose**  
Represents a single authenticated context for a user on a specific device. A session is the logical container that binds a user, a device, and a time window together.

**Responsibilities**
- Tracks that a specific user is authenticated on a specific device.
- Records the time the session started and the most recent time it was active.
- Holds enough metadata (device, IP, user-agent) for the user to recognize and revoke it from a device management screen.
- Carries a revocation flag so that a logout or administrative action immediately invalidates all tokens issued under that session.

**Key Attributes**
- A globally unique session identifier.
- Reference to the owning User.
- Reference to the registered Device.
- The IP address at session creation (and optionally the most recent IP).
- Session creation time.
- Last activity time (updated on every token refresh).
- Absolute expiry time (set at creation, never extended).
- Revoked flag and revocation timestamp.
- A `remember_me` flag indicating whether the session was established with extended lifetime.

**Relationships**
- Belongs to exactly one User.
- Belongs to exactly one Device.
- May have zero or more Refresh Tokens issued against it (at most one valid at any point in time due to rotation).

**Lifecycle**  
Created at login → active while refresh tokens are rotated against it → revoked explicitly by logout or administrative action, or expired when the absolute timeout elapses → archived/deleted per retention policy.

---

### 4.3 Refresh Token

**Purpose**  
A long-lived, single-use credential that allows a client to obtain a new access token without re-entering credentials.

**Responsibilities**
- Carries a cryptographically random, opaque token value.
- Is associated with exactly one session.
- Is consumed (marked used) the moment it is presented, and immediately replaced by a new refresh token.
- Detects replay attacks: if a previously consumed refresh token is presented again, the entire session must be revoked.

**Key Attributes**
- A globally unique identifier.
- The opaque token value (stored as a secure hash; the plaintext is sent to the client once and never stored in recoverable form).
- Reference to the owning Session and User.
- Reference to the Device it was issued to.
- Issued-at timestamp.
- Expiry timestamp.
- A consumed/used flag and the timestamp it was consumed.
- The identifier of the refresh token that replaced this one (forms a chain for audit purposes).
- A revoked flag for explicit revocation independent of consumption.

**Relationships**
- Belongs to exactly one Session.
- Belongs to exactly one User.
- Issued to exactly one Device.
- May point to a successor Refresh Token (the one it was rotated into).

**Lifecycle**  
Issued at session creation → valid until first use → consumed on use (new token issued simultaneously) → invalid after expiry or revocation → old tokens retained briefly for replay detection, then purged.

---

### 4.4 Device

**Purpose**  
Represents a physical or logical endpoint that a user has authenticated from. Tracking devices enables per-device session management and gives users visibility into where their account is active.

**Responsibilities**
- Stores a stable fingerprint that allows the system to recognize a returning device.
- Provides a human-readable name so the user can identify it (e.g., "Chrome on MacBook Pro").
- Records the platform, browser, operating system, and IP address for display in the device management interface.
- Supports individual device revocation (all sessions on that device become invalid).

**Key Attributes**
- A globally unique device identifier.
- Reference to the owning User.
- A device fingerprint (a deterministic, privacy-preserving token derived from device characteristics; see section 9 for the conceptual model).
- A friendly name (auto-generated or user-customized).
- Platform (web, iOS, Android, desktop).
- Browser or app name and version.
- Operating system.
- Most recent IP address.
- First seen timestamp.
- Last active timestamp.
- Trusted flag (future: could unlock reduced friction for trusted devices).
- Revoked flag and revocation timestamp.

**Relationships**
- Belongs to exactly one User.
- Has zero or more Sessions (one active at most under normal operation).

**Lifecycle**  
Registered on first login from an unrecognized fingerprint → updated on subsequent logins (IP, last active) → revoked by the user or by administrative action → purged after a configurable inactivity period.

---

### 4.5 Role

**Purpose**  
A named collection of permissions that can be assigned to users. Roles provide a human-readable label for a set of capabilities and allow permission sets to be managed in one place.

**Responsibilities**
- Provides a stable name and description for a permission bundle.
- Is assigned to users to grant them a capability set.
- Is the primary object of the authorization check ("does this user have the `admin` role?").

**Key Attributes**
- A unique identifier.
- A unique, immutable system name (e.g., `user`, `moderator`, `admin`).
- A human-readable display label.
- A description.
- A system flag indicating whether the role is built-in (cannot be deleted).

**Relationships**
- Has zero or more Permissions.
- Is assigned to zero or more Users.

**Lifecycle**  
System roles are seeded at deployment time and cannot be deleted. Custom roles (future) can be created and deleted by administrators.

---

### 4.6 Permission

**Purpose**  
A single, granular capability within the system. Permissions are the atoms of the authorization model.

**Responsibilities**
- Names a specific action that can be taken (e.g., `party:create`, `chat:moderate`, `admin:suspend_user`).
- Is grouped into roles for practical assignment.
- Is the primitive checked at the authorization boundary of each operation.

**Key Attributes**
- A unique identifier.
- A unique, immutable name following the `resource:action` convention.
- A human-readable description.
- A category (for display grouping in an admin interface).

**Relationships**
- Belongs to one or more Roles.

**Lifecycle**  
Permissions are defined at design time and seeded at deployment. New permissions are added as new features are built; existing permissions are never renamed (to avoid breaking permission checks in code).

---

### 4.7 Email Verification Token

**Purpose**  
A short-lived, single-use token sent to a user's email address to confirm they control it.

**Responsibilities**
- Provides a time-bounded challenge: the user must click a link containing this token within the expiry window.
- Is invalidated on use, preventing replay.
- Is invalidated when a new verification request is made (only the most recent token is valid).

**Key Attributes**
- A unique identifier.
- Reference to the owning User.
- A cryptographically random opaque token value.
- Expiry timestamp (typically 24 hours after issuance).
- A used flag and used-at timestamp.

**Relationships**
- Belongs to exactly one User.

**Lifecycle**  
Issued on registration (and on explicit resend request) → valid until the user clicks the verification link or the token expires → consumed on successful verification → purged after a retention period.

---

### 4.8 Password Reset Token

**Purpose**  
A short-lived, single-use token that grants the bearer the right to set a new password, issued only via the account recovery flow.

**Responsibilities**
- Provides a time-bounded, single-use recovery credential.
- Is invalidated immediately on use.
- Is invalidated if a newer reset is requested, ensuring only the most recent token is valid.
- Does not grant any session access — only the right to change the password.

**Key Attributes**
- A unique identifier.
- Reference to the owning User.
- A cryptographically random opaque token value (stored as a hash).
- Expiry timestamp (typically 1 hour after issuance).
- A used flag and used-at timestamp.

**Relationships**
- Belongs to exactly one User.

**Lifecycle**  
Issued when the user initiates "Forgot Password" → delivered via email → valid for a short window → consumed when the user submits a new password → old tokens invalidated when a new reset is requested → purged after retention period.

---

## 5. Entity Relationships

```
User (root aggregate)
├── Sessions                  [1..n] — one per device login, concurrent allowed
│   └── Refresh Tokens        [0..1 valid at a time] — rotated on every use
├── Devices                   [1..n] — registered on first login, managed by user
├── Roles                     [1..n] — at least the default "user" role
│   └── Permissions           [n..m] — resolved transitively through roles
├── Email Verification Token  [0..1 active] — present only pre-verification
└── Password Reset Token      [0..1 active] — present only during recovery
```

### Ownership & Cardinality

| Relationship | Cardinality | Notes |
|---|---|---|
| User → Sessions | One-to-many | Concurrent sessions across devices are supported. |
| User → Devices | One-to-many | Upper bound enforced by business rule (configurable max devices). |
| Session → Device | Many-to-one | Each session belongs to one device; a device may have sessions over time. |
| Session → Refresh Tokens | One-to-many (one valid) | Token rotation creates a chain; only the current token is valid. |
| User → Roles | Many-to-many | A user can hold multiple roles; a role can be held by many users. |
| Role → Permissions | Many-to-many | A permission can belong to multiple roles. |
| User → Email Verification Token | One-to-one (optional) | Only while `pending_verification`; null after verification. |
| User → Password Reset Token | One-to-one (optional) | Only present during an active recovery flow. |

---

## 6. User Lifecycle

```
[Visitor]
    │
    │  POST /auth/register
    ▼
[pending_verification]
    │
    │  User clicks email verification link
    │  GET /auth/verify-email?token=...
    ▼
[active]
    │
    ├──────────────────────────────────────────────────┐
    │  POST /auth/login                                │
    ▼                                                  │
[authenticated session]                               │
    │                                                  │
    │  Client presents refresh token                  │
    │  POST /auth/token/refresh                        │
    ▼                                                  │
[new access token issued, old refresh token consumed] │
    │                                                  │
    │  POST /auth/logout                               │
    ▼                                                  │
[session revoked]                                      │
    │                                                  │◄─────┘
    │  POST /auth/password/change
    ▼
[all sessions invalidated, user must re-login]
    │
    │  POST /auth/account/delete  (initiates grace period)
    ▼
[soft-deleted — grace period: ~30 days]
    │
    │  No reactivation request within grace period
    ▼
[purged — data deleted per retention policy]
```

### Transition Explanations

| Transition | Trigger | Outcome |
|---|---|---|
| **Registration** | User submits email, username, and password. | Account created in `pending_verification` state. Verification email dispatched. |
| **Email Verification** | User follows the link in the verification email. | Account moves to `active`. The email verification token is consumed. |
| **Login** | Active user submits correct credentials. | Session and device registered (or recognized). Access token and refresh token issued. |
| **Token Refresh** | Client presents a valid, unconsumed refresh token. | New access token and new refresh token issued. Old refresh token marked consumed. Session last-active time updated. |
| **Logout** | User (or system) explicitly ends a session. | Session revoked. All refresh tokens under that session invalidated. |
| **Password Change** | Authenticated user submits old and new password. | Password updated. All active sessions invalidated (forcing re-login everywhere). |
| **Password Reset** | User completes the forgot-password flow. | Password updated via reset token. All sessions invalidated. Reset token consumed. |
| **Suspension** | Administrative action. | Account status set to `suspended`. All active sessions revoked. User cannot log in. |
| **Reactivation** | Administrative action. | Account status set to `active`. User must log in fresh. |
| **Soft Deletion** | User requests account deletion. | Account status set to `deleted`. All sessions revoked. A 30-day grace period begins before permanent data removal. |
| **Hard Deletion** | Grace period elapses with no reactivation. | Personal data removed per retention policy. Audit logs retained. |

---

## 7. Authentication Flow

### 7.1 Registration

1. Client submits: email, username, and password.
2. System validates that email and username are not already taken and that the password satisfies complexity requirements.
3. System creates the User in `pending_verification` state.
4. System generates an Email Verification Token and dispatches a verification email.
5. System responds to the client: account created, verification email sent.
6. No session is created. The user cannot access protected resources until verification is complete.

### 7.2 Login

1. Client submits: email (or username) and password, along with device fingerprint metadata.
2. System verifies the account exists, is `active`, and the password matches.
3. System identifies or registers the device based on the fingerprint.
4. System creates a new Session associated with the user and device.
5. System issues a short-lived **Access Token** (JWT) and a long-lived **Refresh Token** (opaque).
6. Both tokens are returned to the client. The refresh token must be stored securely (HttpOnly cookie on web; secure storage on mobile).

### 7.3 Access Token

- A signed, stateless JWT.
- Short-lived (minutes, not hours) to limit the impact of a stolen token.
- Contains: user ID, session ID, roles, issued-at, and expiry.
- Presented as a Bearer token in the `Authorization` header on every authenticated request.
- The server validates the signature and expiry; no database lookup is required for normal requests.

### 7.4 Refresh Token

- A cryptographically random, opaque string. It carries no information about the user.
- Long-lived (days or weeks, depending on `remember_me`).
- Stored server-side (a hash of the token is persisted; the plaintext is sent to the client once and never stored in recoverable form).
- Presented to a dedicated token refresh endpoint only.
- **Single-use**: consuming a refresh token atomically invalidates it and issues a replacement.

### 7.5 Token Refresh

1. Client presents the current refresh token to the refresh endpoint.
2. System locates the token record by its hash.
3. System verifies: token exists, not consumed, not revoked, not expired, associated session not revoked.
4. System marks the token as consumed.
5. System issues a new access token and a new refresh token atomically.
6. System updates the session's last-active timestamp.
7. New tokens are returned to the client.

**Replay detection**: if a consumed refresh token is presented, the session it belongs to is immediately and fully revoked. This indicates either a replay attack or a stolen token.

### 7.6 Logout

- **Single-device logout**: the specific session is revoked. The refresh token for that session is invalidated. Access tokens already issued continue to be technically valid until they expire naturally (acceptable given their short lifetime), but the session record will reject any further refresh attempts.
- **All-devices logout**: all sessions for the user are revoked simultaneously. All refresh tokens across all devices are invalidated.

### 7.7 Session Expiration

Sessions expire in two ways:

- **Idle timeout**: if no token refresh occurs within the idle window, the session is considered expired on next access attempt.
- **Absolute timeout**: every session has a hard expiry timestamp set at creation that cannot be extended, regardless of activity. After absolute expiry, the user must re-authenticate.

---

## 8. Session Model

### Why Sessions Exist

Access tokens are stateless: once issued, the server cannot revoke them before expiry. Sessions provide the stateful hook that makes revocation possible. When a session is revoked, the refresh token associated with it is invalidated, so the client cannot obtain new access tokens. The short lifetime of access tokens means the window between revocation and full lockout is bounded and acceptable.

### Multiple Devices

Each device login creates a new, independent session. Sessions are scoped to a (user, device) pair. This means:
- Logging out on one device does not affect sessions on other devices (unless the user explicitly selects "log out everywhere").
- Users can see all active sessions from any device management interface.
- Each device has its own refresh token chain.

### Concurrent Logins

Concurrent logins are supported and expected. A user may be simultaneously authenticated on their phone, laptop, and a shared TV. There is no forced single-session constraint.

### Session Revocation

Sessions can be revoked by:
- The user logging out (single session).
- The user logging out of all devices.
- The user changing their password (all sessions invalidated).
- An administrator suspending the account (all sessions invalidated).
- The system detecting suspicious activity, such as a refresh token replay (the affected session is revoked).
- The absolute session timeout elapsing.

### Remember Me

When the user opts into "Remember Me" at login:
- The refresh token is issued with an extended lifetime (e.g., 30 days instead of 7 days).
- The absolute session timeout is correspondingly extended.
- This preference is stored on the Session entity.

When "Remember Me" is not selected, the session is treated as a standard session with shorter token lifetimes, suitable for shared or public devices.

### Idle Timeout

The idle timeout measures time since the last successful token refresh. If a refresh has not occurred within the idle window (e.g., 7 days), the session is considered idle-expired. The next refresh attempt will be rejected and the client must re-authenticate. The idle window resets on every successful refresh.

### Absolute Timeout

The absolute timeout is set at session creation and is never extended. It represents the maximum possible lifetime of any session regardless of activity. This ensures that even a very active "Remember Me" session will eventually require full re-authentication. The absolute timeout for a regular session might be 7 days; for "Remember Me," 30 days.

---

## 9. Device Model

### Device Registration

A device is registered automatically the first time a user authenticates from an unrecognized fingerprint. The user does not take any explicit action to register a device. On subsequent logins from the same device, the system recognizes it by fingerprint and updates the last-active metadata rather than creating a new device record.

### Device Fingerprint (Conceptual)

A device fingerprint is a deterministic, privacy-respecting token derived from characteristics available at authentication time. It is not a tracking identifier — it is used solely to recognize returning devices within a single user's account context.

The fingerprint is constructed from a combination of stable signals: browser/app version, operating system, platform type, and a client-generated persistent identifier (e.g., stored in local storage on web or generated once on mobile install). No biometric, hardware serial, or cross-site tracking identifiers are used.

The fingerprint is hashed before storage. The raw signals that produced it are not retained. The system treats the fingerprint as a probabilistic hint: a mismatch does not block login — it simply registers a new device.

### Friendly Device Names

A friendly name is auto-generated at device registration from the platform and browser/OS signals (e.g., "Chrome on Windows", "StreamTogether iOS App", "Safari on iPhone"). Users may later rename their devices to more personal names (e.g., "Work Laptop", "Living Room TV") from their account settings. The friendly name is purely presentational.

### Last Active

The device's last-active timestamp is updated on every successful token refresh that is associated with a session on that device. This allows the device list to display meaningful recency information.

### IP Address

The most recent IP address observed for the device is recorded on every login and token refresh. This is displayed to the user in the device management interface to help identify suspicious activity. Historical IP addresses are not retained beyond the most recent.

### Platform & Browser

The platform (web, iOS, Android, desktop) and browser/app name are recorded at device registration and updated if they change. These fields power the icon and label displayed in the device management interface.

### Revocation

A user can revoke any registered device from the device management interface. Revoking a device:
1. Marks the device as revoked.
2. Revokes all active sessions associated with that device.
3. Invalidates all refresh tokens issued to that device.

Access tokens already issued continue to be valid until their natural expiry (bounded by their short lifetime). The device record is retained in a revoked state for audit purposes; it is not re-used by a new login from the same fingerprint (a new device record is created instead).

---

## 10. Roles & Permissions

### Design Philosophy

The authorization model is Role-Based Access Control (RBAC) with direct permission assignment reserved for future use. Roles are named bundles of permissions. Users hold roles. Permission checks evaluate the union of permissions granted by all of a user's roles.

The model is designed to be extensible: adding a new permission requires only defining it and assigning it to the appropriate role(s). No user data needs to be migrated.

### System Roles

The following roles are seeded at deployment time and cannot be deleted:

| Role | Description |
|------|-------------|
| `user` | The default role assigned to every registered user. Grants access to core platform features: joining parties, sending chat, using voice, managing one's own account. |
| `moderator` | Extends `user` with moderation capabilities: removing chat messages, kicking party members, filing reports. |
| `admin` | Full platform access including user management, content moderation, system configuration, and all moderator capabilities. |
| `service` | An internal machine identity used for service-to-service calls. Not assignable to human users. |

### Permission Naming Convention

Permissions follow the `resource:action` convention:

```
party:create
party:join
party:kick_member
chat:send
chat:delete_own
chat:delete_any
user:manage
admin:suspend_user
admin:view_audit_log
```

This naming scheme makes permission checks self-documenting and supports wildcard or prefix-based checks in future implementations (e.g., `admin:*` grants all admin permissions).

### Role Assignment

- Every new user is automatically assigned the `user` role on registration.
- Additional roles (e.g., `moderator`) are assigned by administrators through the admin interface.
- A user may hold multiple roles simultaneously. Their effective permission set is the union of all permissions across all held roles.
- Roles are assigned and revoked as a unit; individual permissions within a role are not toggled per user at this time (future: direct permission overrides).

### Permission Checks

Authorization checks occur at two levels:

1. **Route-level middleware**: certain routes require a specific role (e.g., only `admin` can access `/admin/*`). This check is coarse and fast.
2. **Service-level check**: fine-grained permission checks occur within the business logic layer when the operation requires it. The check resolves the user's effective permission set and tests for the specific permission required.

Permission resolution is a read-heavy operation and is a candidate for caching (e.g., storing the resolved permission set in Redis with an appropriate TTL, invalidated on role change).

### Extensibility

The model supports future enhancements without redesign:
- **Custom roles** (admin-defined): add a `is_system` flag to distinguish built-in from custom roles.
- **Direct permission grants**: a `user_permissions` association can override role-derived permissions for individual users.
- **Resource-scoped permissions** (e.g., moderator of a specific community): introduce a `scope` field on the role assignment.
- **Organization accounts**: introduce an `organization` entity and scope roles to it.

---

## 11. Business Rules

### Identity Uniqueness
- BR-01: Email addresses must be globally unique, case-insensitively. Two accounts cannot share the same email.
- BR-02: Usernames must be globally unique, case-insensitively. Two accounts cannot share the same username.
- BR-03: Usernames must not match reserved words (e.g., "admin", "system", "support") defined in a deny list.

### Account State
- BR-04: Only `active` users may authenticate. Accounts in `pending_verification`, `suspended`, or `deleted` states are rejected at login.
- BR-05: An account in `pending_verification` state may request a new verification email but cannot access any protected resource.
- BR-06: Soft-deleted accounts cannot be logged into. Their identity (email, username) is released for re-use only after permanent deletion.

### Token Policy
- BR-07: Refresh tokens are single-use. A token that has been consumed cannot be used again.
- BR-08: If a consumed refresh token is presented, the entire session it belongs to must be immediately revoked. This is a mandatory security response to a potential replay attack.
- BR-09: Access tokens are not revocable individually. Revocation is achieved by revoking the session and allowing the short-lived access token to expire naturally.
- BR-10: Refresh tokens cannot be used after the session they belong to has been revoked, even if the token itself has not yet expired.

### Password Policy
- BR-11: Passwords must meet minimum complexity requirements: a minimum length of 8 characters, with at least one uppercase letter, one lowercase letter, one digit, and one special character.
- BR-12: The new password in a change-password flow must differ from the current password.
- BR-13: Password reset tokens are single-use and expire after 1 hour.
- BR-14: On successful password change or reset, all active sessions for that user (except optionally the current one, by design choice) must be revoked.

### Device Policy
- BR-15: A user account may have a maximum number of registered, active devices (default: 10, configurable). Attempting to log in from a new device when at the limit requires the user to revoke an existing device first.
- BR-16: Revoking a device revokes all sessions associated with it.

### Session Policy
- BR-17: Session idle timeout is enforced: sessions that have not been refreshed within the idle window are expired on the next access attempt.
- BR-18: Session absolute timeout is enforced: sessions cannot live beyond their absolute expiry regardless of activity.
- BR-19: Standard sessions have a shorter absolute timeout than "Remember Me" sessions.

### Email Verification
- BR-20: Email verification tokens are single-use.
- BR-21: A new verification email invalidates any previously issued, unused verification token for the same user.
- BR-22: Verification tokens expire after 24 hours.

### Soft Deletion
- BR-23: Account deletion is soft first: the account is moved to `deleted` state and a grace period (30 days) begins.
- BR-24: During the grace period, the user's email and username remain reserved (not available for re-registration).
- BR-25: After the grace period, personal data is permanently removed per the data retention policy. The user's contribution data (e.g., chat messages) is anonymized rather than deleted.

---

## 12. Security Requirements

### Password Handling
- Passwords must be hashed using a modern, adaptive, memory-hard algorithm (Argon2id is the preferred choice). The raw password must never leave the authentication layer.
- Password hashing parameters (memory, iterations, parallelism) must be tunable via configuration to allow cost increases over time without requiring password resets.
- Password comparison must use a constant-time comparison function to prevent timing side-channel attacks.

### Token Security
- Access tokens (JWTs) must be signed using a strong asymmetric algorithm (RS256 or ES256) or a symmetric HMAC with a secret of at least 256 bits (HS256). The signing key must be loaded from configuration, not hardcoded.
- Refresh tokens must be generated using a cryptographically secure random number generator (CSPRNG). They must have sufficient entropy (at least 256 bits) to make brute-force enumeration infeasible.
- Refresh tokens must be stored as a hash (e.g., SHA-256) of the plaintext value. Only the hash is persisted; the plaintext is transmitted to the client once and discarded.
- Access tokens must contain a minimum viable payload: user ID, session ID, and roles. Personal data must not be included in the JWT payload.

### Token Rotation & Replay Protection
- Refresh token rotation must be atomic: the old token is marked consumed and the new token is issued in the same operation. A failure at any point must result in neither change persisting (transactional).
- Detecting the reuse of a consumed refresh token must trigger immediate session revocation and produce an audit log entry.
- The session ID embedded in the access token enables server-side session validation if required (e.g., for WebSocket authentication, where the access token is presented once on connection).

### Transport Security
- All authentication endpoints must operate exclusively over HTTPS. The middleware layer already enforces HSTS.
- Sensitive tokens (refresh tokens) must be delivered and stored in HttpOnly, Secure, SameSite=Strict cookies on web clients, preventing JavaScript access.

### Brute-Force Protection
- Rate limiting middleware is already available at the infrastructure level. Authentication endpoints (login, registration, password reset request) must be protected by rate limiting.
- Login attempts should be counted per IP and per account to detect both distributed and targeted attacks. Configurable lockout thresholds will be enforced at the middleware level.
- Password reset requests must not reveal whether an email address is registered (return a generic success response regardless).

### Audit Trail
The following security events must be logged with sufficient context to reconstruct the event (timestamp, user ID, IP address, device, outcome):
- Successful and failed login attempts.
- Session creation and revocation.
- Refresh token issuance, rotation, and revocation.
- Consumed refresh token replay detected.
- Password change and password reset.
- Email verification.
- Account suspension, reactivation, and deletion.
- Device registration and revocation.
- Role assignment and removal.

Audit logs must be append-only from the application's perspective. They must not be modifiable through normal application flows.

### Secrets Management
- JWT signing keys, database credentials, and Redis passwords must be loaded from environment configuration. They must never be hardcoded or committed to version control.

---

## 13. Error Cases

The following are authentication-domain error conditions. Each must produce a clear, unambiguous error code and a safe, non-leaking message. Internal details (database errors, stack traces) must never be exposed to the client.

| Code | Error | Description |
|------|-------|-------------|
| `AUTH_EMAIL_EXISTS` | Email already registered | Registration attempt with an email that already exists. |
| `AUTH_USERNAME_EXISTS` | Username already taken | Registration attempt with a username that already exists. |
| `AUTH_USERNAME_RESERVED` | Username is reserved | Attempted username is on the platform deny list. |
| `AUTH_PASSWORD_TOO_WEAK` | Password does not meet requirements | Password fails complexity rules. |
| `AUTH_INVALID_CREDENTIALS` | Invalid email or password | Login with incorrect credentials. Message must not identify whether email or password was wrong. |
| `AUTH_EMAIL_NOT_VERIFIED` | Email address not verified | Login attempt on a `pending_verification` account. |
| `AUTH_ACCOUNT_SUSPENDED` | Account has been suspended | Login attempt on a suspended account. |
| `AUTH_ACCOUNT_DELETED` | Account has been deleted | Login attempt on a soft-deleted account. |
| `AUTH_SESSION_EXPIRED` | Session has expired | Refresh attempt on an idle-expired or absolute-expired session. |
| `AUTH_SESSION_REVOKED` | Session has been revoked | Refresh attempt on an explicitly revoked session. |
| `AUTH_TOKEN_INVALID` | Token is invalid | Malformed or unrecognizable refresh token. |
| `AUTH_TOKEN_EXPIRED` | Token has expired | Refresh token past its expiry timestamp. |
| `AUTH_TOKEN_REVOKED` | Token has been revoked | Refresh token explicitly revoked independent of session. |
| `AUTH_TOKEN_CONSUMED` | Token has already been used | Refresh token that was already consumed (potential replay — triggers session revocation). |
| `AUTH_DEVICE_LIMIT_REACHED` | Device limit reached | Login from a new device when the user is at the maximum registered device count. |
| `AUTH_DEVICE_REVOKED` | Device has been revoked | Login attempt from a fingerprint matching a revoked device. |
| `AUTH_VERIFICATION_TOKEN_INVALID` | Verification link is invalid | Email verification token does not exist or is malformed. |
| `AUTH_VERIFICATION_TOKEN_EXPIRED` | Verification link has expired | Email verification token is past its 24-hour window. |
| `AUTH_VERIFICATION_TOKEN_USED` | Verification link already used | Email verification token was already consumed. |
| `AUTH_RESET_TOKEN_INVALID` | Password reset link is invalid | Reset token does not exist or is malformed. |
| `AUTH_RESET_TOKEN_EXPIRED` | Password reset link has expired | Reset token is past its 1-hour window. |
| `AUTH_RESET_TOKEN_USED` | Password reset link already used | Reset token was already consumed. |
| `AUTH_SAME_PASSWORD` | New password must differ from current | Password change submitted an identical password. |
| `AUTH_WRONG_CURRENT_PASSWORD` | Current password is incorrect | Password change with an incorrect current password. |

---

## 14. Future Extensions

The domain model has been designed to accommodate the following capabilities without requiring a redesign.

### OAuth / Social Login

The `User` entity's credential model can be extended with an `identity_provider` concept. Rather than (or in addition to) a hashed password, a user may have one or more OAuth identities linked to their account (e.g., Google, Discord). The Session and Device model are unchanged — OAuth provides an alternative way to arrive at a trusted user identity, after which the token flow is identical.

To support this, the domain will need:
- An `OAuthIdentity` entity (provider name, provider user ID, linked user ID, created/updated timestamps).
- A distinction between "password-based" and "federated" accounts (a user who signed up via Google may have no password).
- A linking flow to merge an existing email/password account with a social login.

### Passkeys (WebAuthn)

Passkeys replace the password with a public-key credential stored on the user's device. The domain extension needed is:
- A `Passkey` entity (credential ID, public key, AAGUID, sign count, linked user and device, created/last-used timestamps).
- A registration flow (ceremony) and an authentication flow (assertion).
- The existing Session and Device model are reused without modification.

### Multi-Factor Authentication (MFA)

MFA adds a second verification step after primary credential validation. The domain extension needed is:
- An `MfaFactor` entity (type: TOTP / SMS / email OTP, secret or delivery address, verified flag, created timestamp).
- A `pending_mfa` transient login state: the user has passed primary authentication but must complete the second factor before a session is created.
- Backup codes as a special MFA factor type.

The Session model gains an `mfa_verified` flag to indicate whether the second factor was passed.

### SSO / Enterprise Authentication

SSO support (SAML, OIDC) follows the same extension pattern as OAuth but at an organizational level. The extension needed is:
- An `Organization` entity with SSO configuration.
- A user-to-organization membership.
- Role mapping from the SSO provider's groups to local roles.

### Organization Accounts

Multi-tenancy can be added without breaking the existing model:
- An `Organization` entity.
- A `Membership` entity joining users to organizations with a role.
- Resource-scoped permissions (a user may be `admin` within one organization and `user` in another).

### Granular Permission Overrides

The current RBAC model grants permissions via roles only. Future enhancement:
- A `UserPermission` override entity allowing specific permissions to be granted or denied directly on a user, taking precedence over role-derived permissions.

---

## 15. Design Decisions

### Why JWT for Access Tokens?

**Decision**: Access tokens are JWTs.

**Rationale**: JWTs allow the server to validate a request without a database or cache lookup. This is critical for a platform where every authenticated API call would otherwise require a round-trip to a shared session store. At the scale of a watch-party platform, the volume of requests during active parties makes stateless validation a practical necessity.

**Tradeoffs**: JWTs cannot be revoked before their expiry. This is mitigated by keeping access token lifetimes very short (minutes). The refresh token mechanism provides the revocation hook.

---

### Why Opaque Refresh Tokens?

**Decision**: Refresh tokens are opaque random strings, not JWTs.

**Rationale**: Refresh tokens are long-lived and high-value. If a refresh token were a JWT, its validity could be verified without a server lookup — which means revocation would be impossible within its lifetime. By making refresh tokens opaque, every refresh operation requires a server lookup, giving the system the opportunity to enforce revocation, rotation, and replay detection. The performance cost is acceptable because refresh operations are infrequent compared to access token validations.

---

### Why Separate Sessions from Refresh Tokens?

**Decision**: Session and Refresh Token are distinct entities, not collapsed into one.

**Rationale**: A session represents the _state_ of a user's authenticated context on a device. It has metadata (creation time, last active, device, IP, `remember_me` preference) that persists across many token rotations. A refresh token is a single-use _credential_ that is consumed and replaced. Collapsing these would mean losing the session's history and metadata every time a token is rotated. Keeping them separate allows the session to be a stable, long-lived audit record while the refresh token chain evolves underneath it.

---

### Why Single-Use Refresh Token Rotation?

**Decision**: Refresh tokens are rotated on every use and a consumed token may not be reused.

**Rationale**: If a refresh token is stolen and the attacker uses it before the legitimate client does, the legitimate client's next refresh attempt (with a now-consumed token) will fail and the system will detect the conflict. This is the strongest protection against stolen refresh tokens without requiring the user to re-authenticate after every token use. Detecting the replay of a consumed token is the signal that triggers full session revocation.

---

### Why RBAC?

**Decision**: Role-Based Access Control rather than Attribute-Based Access Control (ABAC) or simple permission lists per user.

**Rationale**: RBAC is well-understood, easy to audit, and sufficient for the current authorization requirements. The `resource:action` permission naming provides enough granularity for fine-grained checks when needed. ABAC's additional expressiveness (policies based on request context, resource attributes, and user attributes) is not needed at this stage. RBAC's simpler model reduces the likelihood of misconfiguration and is easier to explain to non-engineers. The design is extensible to ABAC if future requirements demand it.

---

### Why Multi-Device Support?

**Decision**: Concurrent sessions across multiple devices are supported, with per-device session and refresh token chains.

**Rationale**: Modern users operate across multiple devices simultaneously. Forcing single-session login (the device-monopoly model) would degrade the user experience significantly. The platform is intended for social, ambient use — a user might start watching on their laptop and continue on their TV. Per-device sessions also make the security model more granular: revoking a lost device only impacts that device's session, not all of the user's sessions.

---

### Why Soft Deletion?

**Decision**: Account deletion is soft first, with a grace period before permanent removal.

**Rationale**: Accidental or impulsive account deletion is a support burden and a poor user experience. A grace period allows users to recover their account within a bounded window. It also gives the system time to process any pending operations (e.g., completing a running party). After the grace period, personal data is removed per the privacy policy, while non-personal contribution data (anonymized) may be retained for platform integrity.

---

### Why Hash Refresh Tokens at Rest?

**Decision**: The server stores only the hash of a refresh token, never the plaintext.

**Rationale**: If the token storage layer is compromised (e.g., a database breach), an attacker who obtains hashed refresh tokens cannot use them directly — they do not have the plaintext values that the clients hold. This adds a layer of defense-in-depth at minimal cost.

---

### Why Enforce Idle and Absolute Timeouts?

**Decision**: Both an idle timeout (time since last activity) and an absolute timeout (set at session creation) are enforced.

**Rationale**: Idle timeout alone can be defeated by an attacker who periodically refreshes a stolen session. An absolute timeout bounds the maximum lifetime of any session regardless of activity, ensuring that even a perfectly maintained session eventually requires re-authentication. The two timeout types serve different threat models and complement each other.
