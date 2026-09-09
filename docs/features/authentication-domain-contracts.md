---
title: Authentication & Identity Domain — Implementation Contracts
description: Contract blueprint for the Authentication & Identity domain. Defines domain entities, DTOs, repository interfaces, service interfaces, token contracts, domain errors, mappers, business invariants, and dependency rules. No implementation — contracts only.
ms.date: 2026-08-07
status: draft
depends-on: authentication-domain-design.md
---

# Authentication & Identity Domain — Implementation Contracts

**Document status:** Draft for review  
**Based on:** `docs/features/authentication-domain-design.md` (approved)  
**Module:** `github.com/AstroWalker24/Streamtogether-backend`

---

## Table of Contents

1. [Package Map](#1-package-map)
2. [Domain Entities](#2-domain-entities)
3. [Data Transfer Objects (DTOs)](#3-data-transfer-objects-dtos)
4. [Repository Contracts](#4-repository-contracts)
5. [Service Contracts](#5-service-contracts)
6. [Token Contracts](#6-token-contracts)
7. [Domain Errors](#7-domain-errors)
8. [Mappers](#8-mappers)
9. [Business Invariants](#9-business-invariants)
10. [Dependency Rules](#10-dependency-rules)
11. [Package Responsibilities](#11-package-responsibilities)
12. [Future Extensions](#12-future-extensions)
13. [Appendix: Open Decisions](#appendix-open-decisions-to-resolve-before-implementation)

---

## 1. Package Map

```
internal/
└── auth/
    ├── domain/        Pure business entities. Zero external dependencies.
    ├── dto/           Shapes that cross layer boundaries (HTTP ↔ Service).
    ├── repository/    Read/write contracts toward the database layer.
    ├── service/       Business operation contracts.
    ├── token/         Signing, verification, and claims contracts.
    ├── mapper/        Transformation logic between layers.
    └── errors/        Domain-specific error codes extending the global AppError.
```

No package in `internal/auth/` may import Fiber, pgx, go-redis, a JWT library, or any email library. Those are infrastructure details owned by the layers outside this domain.

---

## 2. Domain Entities

Domain entities live in `internal/auth/domain`. They represent the business concepts as Go structs with no serialization tags, no database tags, and no framework annotations. They are the single source of truth for what the authentication domain _means_.

---

### 2.1 User

**Purpose**  
The root aggregate of the authentication domain. Represents a verified, persistent human identity on the platform.

**Fields**

| Field | Type | Notes |
|---|---|---|
| `ID` | UUID | Globally unique, immutable, system-assigned. |
| `Email` | string | Lowercased canonical form. Unique across all users. |
| `Username` | string | Lowercased canonical form. Unique across all users. |
| `PasswordHash` | string | Output of Argon2id over the plaintext password. Never the plaintext. |
| `Status` | `UserStatus` | One of: `pending_verification`, `active`, `suspended`, `deleted`. |
| `EmailVerified` | bool | `true` only after the email verification token has been successfully consumed. |
| `CreatedAt` | time.Time | Set once at creation. Never mutated. |
| `UpdatedAt` | time.Time | Updated on every mutation. |
| `DeletedAt` | *time.Time | Non-nil signals soft deletion. Nil for active accounts. |

**`UserStatus` type (enumeration)**  
`pending_verification` → `active` → (`suspended` ↔ `active`) → `deleted`

**Relationships**
- Owns a slice of `Session` (zero-to-many; concurrent across devices)
- Owns a slice of `Device` (zero-to-many; bounded by `MaxDevices`)
- Owns a slice of `RefreshToken` (zero-to-many; via sessions)
- Has a many-to-many association with `Role` (at least the built-in `user` role)
- May have at most one active `EmailVerificationToken`
- May have at most one active `PasswordResetToken`

**Lifecycle**  
Created (pending) → verified (active) → [suspended ↔ active] → soft-deleted → hard-deleted after grace period.

**Invariants**
- `Email` and `Username` are normalized to lowercase before persistence and comparison.
- `PasswordHash` is never empty for a password-based account.
- `Status` must be `active` for any session to be created.
- `DeletedAt` being non-nil implies `Status` is `deleted`.
- `EmailVerified` being `true` implies `Status` is not `pending_verification`.

**Business Rules**  
BR-01, BR-02, BR-03, BR-04, BR-05, BR-06 (see §9).

---

### 2.2 Session

**Purpose**  
A stateful, revocable container binding a user, a device, and a time window. Sessions are what make access token revocation possible without per-request database lookups.

**Fields**

| Field | Type | Notes |
|---|---|---|
| `ID` | UUID | Globally unique, system-assigned. |
| `UserID` | UUID | FK to `User`. |
| `DeviceID` | UUID | FK to `Device`. |
| `IPAddress` | string | IP at session creation time. |
| `UserAgent` | string | Raw user-agent string for display purposes. |
| `CreatedAt` | time.Time | Immutable once set. |
| `LastActiveAt` | time.Time | Updated on every successful token refresh. |
| `ExpiresAt` | time.Time | Absolute hard expiry. Never extended after creation. |
| `Revoked` | bool | `true` once the session is explicitly terminated. |
| `RevokedAt` | *time.Time | Non-nil when `Revoked` is `true`. |
| `RememberMe` | bool | Determines extended refresh-token lifetime at creation. |

**Relationships**
- Belongs to exactly one `User`
- Belongs to exactly one `Device`
- Has zero-to-many `RefreshToken` records (at most one unconsumed at any given moment)

**Lifecycle**  
Created at login → updated on each token refresh (`LastActiveAt`) → revoked on logout / password change / admin action / replay detection / absolute expiry.

**Invariants**
- `ExpiresAt` is set at creation and never modified.
- A revoked session (`Revoked = true`) may never issue new refresh tokens.
- A session past its `ExpiresAt` is treated as revoked for all operational purposes.

---

### 2.3 RefreshToken

**Purpose**  
A single-use, opaque credential that grants the right to obtain a new access token. The chain of consumed tokens provides an audit trail. Replay of a consumed token triggers full session revocation.

**Fields**

| Field | Type | Notes |
|---|---|---|
| `ID` | UUID | Globally unique. |
| `SessionID` | UUID | FK to `Session`. |
| `UserID` | UUID | Denormalized for efficient per-user queries. |
| `DeviceID` | UUID | Denormalized for device management queries. |
| `TokenHash` | string | SHA-256 of the plaintext token. The plaintext is never stored. |
| `IssuedAt` | time.Time | Creation timestamp. |
| `ExpiresAt` | time.Time | Hard expiry. Determined by `Session.RememberMe` at creation. |
| `ConsumedAt` | *time.Time | Non-nil when the token has been used for a refresh. |
| `Revoked` | bool | Explicit revocation flag, independent of consumption. |
| `RevokedAt` | *time.Time | Non-nil when `Revoked` is `true`. |
| `ReplacedByID` | *UUID | The ID of the next token in the rotation chain (set when consumed). |

**Relationships**
- Belongs to exactly one `Session`
- Belongs to exactly one `User`
- Issued to exactly one `Device`
- May reference a successor `RefreshToken` via `ReplacedByID`

**Lifecycle**  
Issued → valid → consumed on first use (successor created atomically) → expired or revoked. Old tokens retained for replay-detection window, then purged by background job.

**Invariants**
- `ConsumedAt` being non-nil means the token is permanently invalid.
- `Revoked = true` means the token is permanently invalid, regardless of expiry.
- Only one unconsumed, unrevoked, unexpired token may exist per session at any time.
- Presenting a consumed token must trigger revocation of the entire session it belongs to.

---

### 2.4 Device

**Purpose**  
A persistent record of a physical or logical endpoint from which a user has authenticated. Powers the device management interface and enables per-device session revocation.

**Fields**

| Field | Type | Notes |
|---|---|---|
| `ID` | UUID | Globally unique. |
| `UserID` | UUID | FK to `User`. |
| `FingerprintHash` | string | SHA-256 of the derived device fingerprint. The raw signals are not stored. |
| `FriendlyName` | string | Auto-generated at registration; user-customizable later. |
| `Platform` | `DevicePlatform` | One of: `web`, `ios`, `android`, `desktop`. |
| `Browser` | string | Browser or app name and version (display only). |
| `OS` | string | Operating system (display only). |
| `LastIPAddress` | string | Most recent IP. Updated on login and token refresh. |
| `FirstSeenAt` | time.Time | Immutable once set. |
| `LastActiveAt` | time.Time | Updated on every session activity on this device. |
| `Trusted` | bool | Reserved for future reduced-friction flows. Default `false`. |
| `Revoked` | bool | `true` when the user or admin has revoked this device. |
| `RevokedAt` | *time.Time | Non-nil when `Revoked` is `true`. |

**Relationships**
- Belongs to exactly one `User`
- Has zero-to-many `Session` records over its lifetime (typically at most one active)

**Lifecycle**  
Registered on first authenticated login with an unrecognized fingerprint → updated on subsequent logins → revoked by user or admin action → not reused for new logins after revocation (a new record is created).

**Invariants**
- A revoked device cannot be the basis of a new session.
- `FingerprintHash` combined with `UserID` identifies a returning device; it is not a global unique key.
- `FriendlyName` is purely presentational and has no security role.

---

### 2.5 Role

**Purpose**  
A named, stable collection of permissions. The primary unit of authorization assignment.

**Fields**

| Field | Type | Notes |
|---|---|---|
| `ID` | UUID | Globally unique. |
| `Name` | string | Immutable system identifier: `user`, `moderator`, `admin`, `service`. |
| `Label` | string | Human-readable display label. |
| `Description` | string | Explains the role's purpose. |
| `IsSystem` | bool | `true` for built-in roles that cannot be deleted. |

**Relationships**
- Has a many-to-many association with `Permission`
- Has a many-to-many association with `User`

**Lifecycle**  
System roles are seeded at deployment and never deleted. Future custom roles may be created by administrators.

**Invariants**
- `Name` is immutable after creation.
- System roles (`IsSystem = true`) cannot be deleted.
- Every newly registered user receives the `user` role automatically.

---

### 2.6 Permission

**Purpose**  
A single granular capability. The atomic unit of the authorization model.

**Fields**

| Field | Type | Notes |
|---|---|---|
| `ID` | UUID | Globally unique. |
| `Name` | string | Immutable. Follows `resource:action` convention (e.g., `party:create`). |
| `Description` | string | Human-readable explanation. |
| `Category` | string | Grouping label for the admin interface (e.g., `party`, `chat`, `admin`). |

**Relationships**
- Has a many-to-many association with `Role`

**Lifecycle**  
Defined at design time, seeded at deployment. Names are never changed.

**Invariants**
- `Name` is immutable after creation.
- Permissions are never renamed (doing so would silently break all permission checks referencing the old name).

---

### 2.7 EmailVerificationToken

**Purpose**  
A time-bounded, single-use proof that a user has access to their registered email address.

**Fields**

| Field | Type | Notes |
|---|---|---|
| `ID` | UUID | Globally unique. |
| `UserID` | UUID | FK to `User`. |
| `TokenHash` | string | SHA-256 of the plaintext token. |
| `ExpiresAt` | time.Time | 24 hours after issuance. |
| `UsedAt` | *time.Time | Non-nil when the token has been consumed. |

**Lifecycle**  
Issued on registration (and on resend request) → delivered via email → consumed when user follows the verification link → invalidated if a new token is requested for the same user.

**Invariants**
- At most one valid (unconsumed, unexpired) token exists per user at any time.
- A new request invalidates any previously issued, unconsumed token for that user.
- `UsedAt` being non-nil means the token is permanently invalid.

---

### 2.8 PasswordResetToken

**Purpose**  
A time-bounded, single-use grant that allows the bearer to set a new password during the account recovery flow.

**Fields**

| Field | Type | Notes |
|---|---|---|
| `ID` | UUID | Globally unique. |
| `UserID` | UUID | FK to `User`. |
| `TokenHash` | string | SHA-256 of the plaintext token. |
| `ExpiresAt` | time.Time | 1 hour after issuance. |
| `UsedAt` | *time.Time | Non-nil when consumed. |

**Lifecycle**  
Issued when user initiates "Forgot Password" → delivered via email → valid for 1 hour → consumed when user submits a new password → a new request invalidates the existing token.

**Invariants**
- At most one valid (unconsumed, unexpired) token exists per user at any time.
- This token does not create a session. It grants only the right to change the password.
- `UsedAt` being non-nil means the token is permanently invalid.

---

## 3. Data Transfer Objects (DTOs)

DTOs live in `internal/auth/dto`. They are the shapes that cross the boundary between the HTTP layer and the service layer (inbound) and between the service layer and the HTTP handler (outbound). They carry validation annotations (`go-playground/validator`) and must never contain domain logic.

No DTO may reference a domain entity directly. No DTO may be stored in the database. No DTO may appear in a repository signature.

---

### 3.1 Inbound (Request) DTOs

#### `RegisterRequest`

**Purpose:** Captures the input for the account creation flow.

**Fields & Validation**

| Field | Validation |
|---|---|
| `Email` | Required. Valid RFC 5322 email. Max 254 chars. Lowercased before use. |
| `Username` | Required. 3–30 chars. Alphanumeric + underscores only. Lowercased before use. |
| `Password` | Required. Min 8 chars. Must satisfy the complexity rules (see BR-11). |

---

#### `LoginRequest`

**Purpose:** Captures the credentials and device context submitted at login.

**Fields & Validation**

| Field | Validation |
|---|---|
| `Identifier` | Required. Accepts either a valid email address or a canonical username. The service layer detects which form was submitted and resolves the user accordingly. |
| `Password` | Required. Non-empty. |
| `DeviceFingerprint` | Required. Non-empty string from the client. |
| `DeviceName` | Optional. Max 100 chars. Used as the friendly name if provided. |
| `Platform` | Optional. One of `web`, `ios`, `android`, `desktop`. |
| `Browser` | Optional. Max 100 chars. |
| `OS` | Optional. Max 100 chars. |
| `RememberMe` | Optional. Boolean. Defaults to `false`. |

---

#### `RefreshRequest`

**Purpose:** Presents the current refresh token to obtain a new token pair.

**Fields & Validation**

| Field | Validation |
|---|---|
| `RefreshToken` | Required. Non-empty opaque string. |

**Delivery source by client type:**

| Client | Source |
|---|---|
| Browser Extension | HttpOnly, Secure, SameSite=Strict cookie (set by the server at login). |
| React Native | Request body (stored by the app in the platform secure store). |
| Future Web Dashboard | HttpOnly, Secure, SameSite=Strict cookie (same as extension). |

The handler layer reads the `Platform` field from the originating `LoginRequest` to determine which extraction path to use. The service layer and this DTO are delivery-agnostic — the refresh token string is always passed in as a plain field regardless of how the handler obtained it.

---

#### `LogoutRequest`

**Purpose:** Requests termination of one or all sessions.

**Fields & Validation**

| Field | Validation |
|---|---|
| `SessionID` | Optional UUID. If present, revokes only that specific session. |
| `AllDevices` | Optional boolean. If `true`, revokes all sessions for the user. |

**Rule:** At least one of `SessionID` or `AllDevices` must be provided. `AllDevices = true` takes precedence.

---

#### `ForgotPasswordRequest`

**Purpose:** Initiates the password recovery flow for a given email address.

**Fields & Validation**

| Field | Validation |
|---|---|
| `Email` | Required. Valid email format. |

**Note:** The service response must be identical whether or not the email exists, to prevent user enumeration.

---

#### `ResetPasswordRequest`

**Purpose:** Completes the password reset using the token delivered by email.

**Fields & Validation**

| Field | Validation |
|---|---|
| `Token` | Required. Non-empty string. |
| `NewPassword` | Required. Must satisfy complexity rules (BR-11). |
| `ConfirmPassword` | Required. Must equal `NewPassword`. |

---

#### `ChangePasswordRequest`

**Purpose:** Changes the password for an authenticated user.

**Fields & Validation**

| Field | Validation |
|---|---|
| `CurrentPassword` | Required. Non-empty. |
| `NewPassword` | Required. Must satisfy complexity rules. Must differ from `CurrentPassword` (BR-12). |
| `ConfirmPassword` | Required. Must equal `NewPassword`. |

---

#### `VerifyEmailRequest`

**Purpose:** Submits the email verification token from the link clicked by the user.

**Fields & Validation**

| Field | Validation |
|---|---|
| `Token` | Required. Non-empty string (extracted from query parameter). |

---

#### `ResendVerificationRequest`

**Purpose:** Requests that a new verification email be sent.

**Fields & Validation**

| Field | Validation |
|---|---|
| `Email` | Required. Valid email format. |

---

### 3.2 Outbound (Response) DTOs

#### `AuthResponse`

**Purpose:** The primary response returned after a successful login or token refresh. Contains the full token pair and the session context the client needs.

**Fields**

| Field | Description |
|---|---|
| `TokenPair` | Access token and refresh token (see below). |
| `User` | `UserResponse` — the authenticated user's public identity. |
| `Session` | `SessionResponse` — metadata about the session just created or refreshed. |

**Refresh token delivery:** The `TokenPair.RefreshToken` field is always populated in the response struct. For web and extension clients the handler additionally sets an HttpOnly cookie and may omit the field from the JSON body (or include it — decision left to the handler implementation). For mobile clients the field is consumed from the JSON body and stored in the platform secure store. The service layer is unaware of delivery mechanics.

---

#### `TokenPair`

**Purpose:** Groups the two tokens issued together. Never stored. Transmitted once.

**Fields**

| Field | Description |
|---|---|
| `AccessToken` | Signed JWT string. |
| `AccessTokenExpiresAt` | RFC 3339 timestamp of access token expiry. |
| `RefreshToken` | Opaque string. Sent to client only once. |
| `RefreshTokenExpiresAt` | RFC 3339 timestamp of refresh token expiry. |
| `TokenType` | Always `"Bearer"`. |

---

#### `UserResponse`

**Purpose:** The public representation of a user returned within auth responses. Deliberately minimal — this is not the full profile.

**Fields**

| Field | Description |
|---|---|
| `ID` | UUID string. |
| `Email` | Canonical email address. |
| `Username` | Canonical username. |
| `EmailVerified` | Boolean. |
| `Roles` | Slice of role names (`[]string`). |
| `CreatedAt` | RFC 3339 timestamp. |

---

#### `SessionResponse`

**Purpose:** Metadata about an active session, used in device management lists and login responses.

**Fields**

| Field | Description |
|---|---|
| `ID` | Session UUID. |
| `Device` | `DeviceResponse` — the device this session belongs to. |
| `IPAddress` | IP address at session creation. |
| `CreatedAt` | RFC 3339 timestamp. |
| `LastActiveAt` | RFC 3339 timestamp. |
| `ExpiresAt` | RFC 3339 timestamp. |
| `RememberMe` | Boolean. |
| `IsCurrent` | Boolean — `true` if this is the session making the request (for the device list view). |

---

#### `DeviceResponse`

**Purpose:** Public representation of a registered device.

**Fields**

| Field | Description |
|---|---|
| `ID` | Device UUID. |
| `FriendlyName` | Display name. |
| `Platform` | `web`, `ios`, `android`, or `desktop`. |
| `Browser` | Browser or app identifier. |
| `OS` | Operating system. |
| `LastIPAddress` | Most recent IP. |
| `LastActiveAt` | RFC 3339 timestamp. |
| `Trusted` | Boolean (reserved). |

---

#### `MessageResponse`

**Purpose:** A simple acknowledgment for operations that have no meaningful data to return (e.g., logout, password reset request, resend verification email).

**Fields**

| Field | Description |
|---|---|
| `Message` | Human-readable confirmation string. |

---

## 4. Repository Contracts

Repository interfaces live in `internal/auth/repository`. Each interface describes the persistence operations that the service layer requires. The repository has no knowledge of HTTP, business rules, or token formats. It translates domain-level intent into storage operations.

Repositories accept and return domain entities, never DTOs. All multi-step repository operations that must be atomic are wrapped in a transaction provided by the caller (the service layer).

---

### 4.1 `UserRepository`

**Responsibilities:** Create, read, and mutate User records.

**Read Operations**

- Find a user by their system-assigned ID.  
  Returns the user or an error if not found.

- Find a user by their canonical email address (case-insensitive).  
  Returns the user or an error if not found.

- Find a user by their canonical username (case-insensitive).  
  Returns the user or an error if not found.

- Find a user by a login identifier that may be either an email address or a username.  
  The repository detects the form (email contains `@`; otherwise username) and routes to the appropriate lookup. Used exclusively by the login flow.

- Check whether a given email is already registered.  
  Returns a boolean.

- Check whether a given username is already taken.  
  Returns a boolean.

**Write Operations**

- Create a new user record.  
  Returns the created user with all system-assigned fields populated.

- Update the user's account status.

- Update the user's email-verified flag.

- Update the user's password hash (used during password change and reset).

- Update the user's `UpdatedAt` and `DeletedAt` for soft deletion.

**Transaction Expectations**  
Password changes require updating the hash and revoking all sessions atomically. These operations span multiple repositories and must be coordinated by the service layer via a transaction passed in through the context.

---

### 4.2 `SessionRepository`

**Responsibilities:** Create, read, and revoke Session records.

**Read Operations**

- Find a session by ID.  
  Returns the session or an error if not found.

- List all active (non-revoked, non-expired) sessions for a given user.  
  Returns a slice, ordered by most recent creation time.

- List all sessions for a given device.

**Write Operations**

- Create a new session.  
  Returns the created session.

- Update the session's `LastActiveAt` timestamp.  
  Called on every successful token refresh.

- Revoke a single session by ID.  
  Sets `Revoked = true` and `RevokedAt = now`.

- Revoke all sessions for a given user (used on password change and logout-all).

- Revoke all sessions for a given device (used on device revocation).

**Transaction Expectations**  
Session creation occurs in the same transaction as refresh token creation. Revocation of all sessions occurs in the same transaction as invalidation of all associated refresh tokens.

---

### 4.3 `RefreshTokenRepository`

**Responsibilities:** Issue, validate, consume, and revoke refresh token records.

**Read Operations**

- Find an active (unconsumed, unrevoked, unexpired) refresh token by its hash.  
  This is the primary lookup used during the refresh flow. Returns the token or an error.

- Find a refresh token by ID (used for the rotation chain audit).

- List all refresh tokens belonging to a session (for audit and administrative views).

**Write Operations**

- Create a new refresh token record.  
  Called as part of login and token rotation.

- Mark a token as consumed.  
  Sets `ConsumedAt = now` and `ReplacedByID = successorID`. Must be atomic with creating the successor.

- Revoke a refresh token by ID.

- Revoke all refresh tokens associated with a given session (called when the session is revoked).

- Revoke all refresh tokens associated with a given user (called on password change and logout-all).

- Purge expired and consumed tokens older than the retention window.  
  Called by a background cleanup job; does not belong in normal request flows.

**Transaction Expectations**  
Token rotation (consume old token, create new token) is the most critical transactional boundary in the entire domain. It must be atomic. If either operation fails, neither must persist. The session's `LastActiveAt` update must also occur in the same transaction.

---

### 4.4 `DeviceRepository`

**Responsibilities:** Register, recognize, and manage device records.

**Read Operations**

- Find a device by ID.

- Find a device for a given user by fingerprint hash.  
  Returns the device if a match exists, or a not-found indicator. Used to distinguish returning devices from new ones.

- List all active (non-revoked) devices for a given user.

- Count active, non-revoked devices for a given user.  
  The result is compared by the service layer against `AppConfig.MaxDevicesPerUser` before creating a new device.

**Write Operations**

- Create a new device record.

- Update a device's mutable metadata: `FriendlyName`, `LastIPAddress`, `LastActiveAt`, `Browser`, `OS`.  
  Called on every login from a recognized device.

- Revoke a device by ID.  
  Sets `Revoked = true` and `RevokedAt = now`.

**Transaction Expectations**  
Device creation occurs in the same transaction as session creation at login time to ensure consistency.

---

### 4.5 `RoleRepository`

**Responsibilities:** Read roles and manage user-role assignments.

**Read Operations**

- Find a role by its system name (e.g., `"user"`, `"admin"`).

- List all roles.

- List all roles assigned to a given user.

- Resolve the full, deduplicated permission set for a given user.  
  This is the permission union across all assigned roles — the result used for authorization checks.

**Write Operations**

- Assign a role to a user.

- Remove a role from a user.

**Transaction Expectations**  
Role assignments should be performed in short transactions. The resolved permission set should be cached in Redis immediately after mutation to avoid stale reads.

---

### 4.6 `PermissionRepository`

**Responsibilities:** Read permission records. Permissions are defined at design time and not mutated at runtime.

**Read Operations**

- Find a permission by its name (e.g., `"party:create"`).

- List all permissions.

- List all permissions associated with a given role.

**Write Operations**  
None in normal operation. Permissions are seeded by the migration/seed layer, not the application runtime.

---

### 4.7 `EmailVerificationTokenRepository`

**Responsibilities:** Issue, consume, and invalidate email verification tokens.

**Read Operations**

- Find a valid (unconsumed, unexpired) token by its hash.

**Write Operations**

- Create a new token.

- Invalidate all existing, unconsumed tokens for a given user.  
  Called before issuing a new token (to enforce single-active-token invariant).

- Mark a token as used.

---

### 4.8 `PasswordResetTokenRepository`

**Responsibilities:** Issue, consume, and invalidate password reset tokens.

**Read Operations**

- Find a valid (unconsumed, unexpired) token by its hash.

**Write Operations**

- Create a new token.

- Invalidate all existing, unconsumed tokens for a given user.  
  Called before issuing a new token.

- Mark a token as used.

---

## 5. Service Contracts

Services live in `internal/auth/service`. Each service interface defines the business operations that handlers call. Services own business logic — validation, sequencing, invariant enforcement, event emission. Services coordinate across multiple repositories and the token layer.

Services accept DTOs or domain-level primitives, and return DTOs or domain entities (the mapper layer handles the conversion for HTTP responses). Services never accept or return HTTP-specific types.

---

### 5.1 `AuthService`

**Responsibilities:** The primary orchestrator of all authentication flows. Coordinates user identity verification, session creation, token issuance, and account lifecycle operations.

**Operations**

| Operation | Input | Output | Business Guarantee |
|---|---|---|---|
| `Register` | `RegisterRequest` | `UserResponse` | Creates user in `pending_verification`. Dispatches verification email. Rejects duplicate email or username. Enforces password policy. |
| `Login` | `LoginRequest`, IP, User-Agent | `AuthResponse` | Resolves the user by email or username from `Identifier`. Verifies credentials. Enforces account status. Identifies or registers device. Enforces configurable device limit (`AppConfig.MaxDevicesPerUser`). Creates session. Issues token pair. |
| `Refresh` | `RefreshRequest`, current session context | `AuthResponse` | Validates and atomically rotates the refresh token. Detects and handles replay. Returns new token pair. |
| `Logout` | `LogoutRequest`, authenticated user context | `MessageResponse` | Revokes one session or all sessions. Invalidates associated refresh tokens. |
| `VerifyEmail` | `VerifyEmailRequest` | `MessageResponse` | Consumes the token. Sets `EmailVerified = true`. Sets status to `active`. |
| `ResendVerification` | `ResendVerificationRequest` | `MessageResponse` | Invalidates previous token. Issues new token. Re-dispatches email. Fails silently if email not found (no enumeration). |
| `ForgotPassword` | `ForgotPasswordRequest` | `MessageResponse` | Issues reset token if account exists. Always returns a generic success (no enumeration). |
| `ResetPassword` | `ResetPasswordRequest` | `MessageResponse` | Validates token. Updates password hash. Revokes all sessions. Consumes token. |
| `ChangePassword` | `ChangePasswordRequest`, authenticated user context | `MessageResponse` | Validates current password. Enforces new password complexity and non-identity rules. Updates hash. Revokes **all** sessions including the one that triggered the change. The client must re-authenticate immediately after this operation completes. |

---

### 5.2 `SessionService`

**Responsibilities:** Session lifecycle management beyond the initial authentication flow. Powers the device management interface.

**Operations**

| Operation | Input | Output | Business Guarantee |
|---|---|---|---|
| `ListSessions` | authenticated user context | `[]SessionResponse` | Returns all active sessions for the user, annotating which one is the caller's current session. |
| `RevokeSession` | session ID, authenticated user context | `MessageResponse` | Revokes a specific session. Validates that the session belongs to the authenticated user. |
| `RevokeAllSessions` | authenticated user context | `MessageResponse` | Revokes all sessions for the user. |
| `GetSession` | session ID, authenticated user context | `SessionResponse` | Returns a specific session's metadata. Validates ownership. |

---

### 5.3 `DeviceService`

**Responsibilities:** Device registration, recognition, and revocation.

**Operations**

| Operation | Input | Output | Business Guarantee |
|---|---|---|---|
| `ListDevices` | authenticated user context | `[]DeviceResponse` | Returns all active (non-revoked) devices for the user. |
| `RevokeDevice` | device ID, authenticated user context | `MessageResponse` | Revokes the device and all its associated sessions. Validates ownership. |
| `RenameDevice` | device ID, new name, authenticated user context | `DeviceResponse` | Updates the friendly name. Validates ownership and input length. |

---

### 5.4 `RoleService`

**Responsibilities:** Role assignment, revocation, and permission resolution. Used internally by other services and by the authorization middleware.

**Operations**

| Operation | Input | Output | Business Guarantee |
|---|---|---|---|
| `AssignRole` | user ID, role name | — | Assigns role. Validates role exists. Signals `PermissionService` to invalidate the cached permission set for that user. |
| `RemoveRole` | user ID, role name | — | Removes role. Validates role exists and user holds it. Signals `PermissionService` to invalidate the cached permission set. |
| `GetUserRoles` | user ID | `[]Role` | Returns all roles held by the user. |

---

### 5.5 `PermissionService`

**Responsibilities:** The authoritative resolver of a user's effective permission set. Owns the caching layer for permission data so that no other service needs to know whether permissions come from the database or from Redis.

**Operations**

| Operation | Input | Output | Business Guarantee |
|---|---|---|---|
| `GetUserPermissions` | user ID | `[]string` | Returns the resolved, deduplicated set of permission names granted by all roles the user holds. Result is served from Redis cache when available; falls back to `RoleRepository.ResolvePermissions` on a cache miss and re-populates the cache. |
| `HasPermission` | user ID, permission name | bool | Returns `true` if the user's resolved permission set includes the given permission. Delegates to `GetUserPermissions` internally. |
| `InvalidateCache` | user ID | — | Evicts the cached permission set for the given user. Called by `RoleService` after any role assignment or removal. |

---

### 5.6 `PasswordService`

**Purpose:** Isolates password hashing and comparison behind a contract so that the specific algorithm (Argon2id) is not referenced outside the service implementation.

**Operations**

| Operation | Input | Output | Business Guarantee |
|---|---|---|---|
| `Hash` | plaintext password | hashed string | Applies Argon2id with configured parameters. Never returns the plaintext. |
| `Verify` | plaintext password, stored hash | bool | Constant-time comparison. Returns `true` only if the plaintext matches the hash. |
| `MeetsPolicy` | plaintext password | bool, policy violation description | Evaluates the password against the complexity rules defined in BR-11. |

**Note:** `PasswordService` is a dependency of `AuthService`, not a public-facing service. It has no HTTP handler. Its interface lives in `internal/auth/service` alongside the others.

---

## 6. Token Contracts

Token contracts live in `internal/auth/token`. This package defines what a token _means_ and what operations must be possible over tokens — not how they are implemented. The specific JWT library, signing algorithm, and key material are infrastructure details hidden behind this contract.

---

### 6.1 Access Token

**Nature:** A signed, stateless, short-lived bearer credential.

**Claims the token must carry**

| Claim | Description |
|---|---|
| Subject (`sub`) | The user's ID (UUID string). |
| Session ID | The session this token was issued under. Used for WebSocket authentication and optional server-side session validation. |
| Roles | Slice of role name strings held by the user at issuance time. |
| Issued At (`iat`) | Unix timestamp of issuance. |
| Expiry (`exp`) | Unix timestamp of expiry. |
| JWT ID (`jti`) | A unique identifier for this specific token. |

**Claims the token must NOT carry**

- Email address or username (PII).
- Password hash or any credential.
- Full permission set (too large; resolved at check time using the roles list).

**Signing contract**
- Must be signed with a key loaded from configuration — never hardcoded.
- The initial algorithm is **HS256** using the `Secret` string from `JWTConfig`. The `TokenService` interface is algorithm-agnostic: it accepts and returns only strings, so the implementation can be replaced with RS256/ES256 without any change to this contract or to any caller.
- `JWTConfig` must gain an `Algorithm` field (e.g., `"HS256"`, `"RS256"`) to make the algorithm explicitly declared rather than implied by which key material is present.
- Verification must check both signature validity and expiry.

**Issuer contract**
- Issued by the `TokenService.IssueAccessToken` operation.
- Input: `User` entity, `Session` entity, resolved `[]Role`.
- Output: signed token string and expiry timestamp.

**Verification contract**
- Parses and validates the token string.
- Returns a parsed `AccessTokenClaims` value type on success.
- Returns a distinct error for: malformed token, invalid signature, expired token.

---

### 6.2 Refresh Token

**Nature:** An opaque, cryptographically random string. Contains no embedded information about the user or session.

**Generation contract**
- Must use a cryptographically secure random number generator.
- Must have at least 256 bits of entropy.
- Must be URL-safe (base64-url encoded or hex encoded).

**Storage contract**
- The plaintext token is sent to the client exactly once and never stored server-side.
- The server stores only the SHA-256 hash of the plaintext.
- Lookup is always by hash: `SHA256(plaintextToken)`.

**Issuer contract**
- Issued by `TokenService.IssueRefreshToken`.
- Input: `User` entity, `Session` entity, `Device` entity, expiry duration.
- Output: plaintext token string (to be sent to the client) and the `RefreshToken` domain entity (to be persisted via the repository).

**Rotation contract**
- Consuming a refresh token is a single atomic operation: mark old token consumed, create new token, link them via `ReplacedByID`.
- If either step fails, neither must persist.

**Replay detection contract**
- Before consuming a token, the service checks whether `ConsumedAt` is already non-nil.
- If it is: immediately revoke the entire session and return an `AUTH_TOKEN_CONSUMED` error.
- This check must happen _before_ any write operations.

---

### 6.3 `AccessTokenClaims` (Value Type)

The parsed, validated result of access token verification. Used by the authorization middleware to populate the request context.

**Fields**

| Field | Type | Source |
|---|---|---|
| `UserID` | UUID | `sub` claim. |
| `SessionID` | UUID | Custom claim. |
| `Roles` | `[]string` | Custom claim. |
| `IssuedAt` | time.Time | `iat` claim. |
| `ExpiresAt` | time.Time | `exp` claim. |
| `TokenID` | UUID | `jti` claim. |

---

### 6.4 `TokenService`

**Responsibilities:** The single point of token issuance and verification for the auth domain.

**Operations**

| Operation | Description |
|---|---|
| `IssueAccessToken` | Creates and signs an access token for a given user and session. Returns the signed string and expiry. |
| `IssueRefreshToken` | Generates a cryptographically random opaque refresh token. Returns the plaintext (for the client) and the domain entity (for persistence). |
| `VerifyAccessToken` | Parses and validates an access token string. Returns `AccessTokenClaims` or an error. |
| `HashRefreshToken` | Computes the SHA-256 hash of a plaintext refresh token. Used before persistence and before lookup. |

---

### 6.5 Expiry & Rotation Policy

The expiry values come from `JWTConfig` and must be configurable:

| Token | Default Lifetime | Extended (`RememberMe`) |
|---|---|---|
| Access Token | 15 minutes | 15 minutes (unchanged) |
| Refresh Token | 7 days | 30 days |
| Session absolute | 7 days | 30 days |
| Session idle | 7 days | 30 days |
| Email Verification | 24 hours | — |
| Password Reset | 1 hour | — |

The `JWTConfig` struct currently holds `AccessTokenExpiry` and `RefreshTokenExpiry`. It must be extended to hold `RefreshTokenRememberMeExpiry`, `SessionAbsoluteTimeout`, and `Algorithm` (default: `"HS256"`).

---

## 7. Domain Errors

Domain errors live in `internal/auth/errors`. They extend the global `AppError` type defined in `internal/errors` by adding authentication-specific `Code` constants and pre-built constructors. Every error defined here must:
1. Map to a specific HTTP status code.
2. Carry a machine-readable `Code` constant (the `AUTH_` prefix namespace).
3. Carry a safe client-facing message that does not leak internal state.

---

### 7.1 Registration Errors

| Code | HTTP Status | When It Occurs |
|---|---|---|
| `AUTH_EMAIL_EXISTS` | 409 Conflict | Registration with an email already in use. |
| `AUTH_USERNAME_EXISTS` | 409 Conflict | Registration with a username already in use. |
| `AUTH_USERNAME_RESERVED` | 409 Conflict | Attempted username matches a platform deny-list entry (e.g., "admin", "support"). |
| `AUTH_PASSWORD_TOO_WEAK` | 400 Bad Request | Password fails any of the complexity rules in BR-11. The message may enumerate which rules failed (this is acceptable — it's not PII). |

---

### 7.2 Login & Authentication Errors

| Code | HTTP Status | When It Occurs |
|---|---|---|
| `AUTH_INVALID_CREDENTIALS` | 401 Unauthorized | Email not found, or password does not match. The message must be identical in both cases (no user enumeration). |
| `AUTH_EMAIL_NOT_VERIFIED` | 403 Forbidden | Login attempted on a `pending_verification` account. |
| `AUTH_ACCOUNT_SUSPENDED` | 403 Forbidden | Login attempted on a `suspended` account. |
| `AUTH_ACCOUNT_DELETED` | 403 Forbidden | Login attempted on a soft-deleted account. |

---

### 7.3 Device Errors

| Code | HTTP Status | When It Occurs |
|---|---|---|
| `AUTH_DEVICE_LIMIT_REACHED` | 403 Forbidden | Login from a new device when the user has reached their `MaxDevices` limit. |
| `AUTH_DEVICE_REVOKED` | 403 Forbidden | Login from a fingerprint that matches a previously revoked device record. |

---

### 7.4 Token & Session Errors

| Code | HTTP Status | When It Occurs |
|---|---|---|
| `AUTH_TOKEN_INVALID` | 401 Unauthorized | Refresh token string is malformed, unrecognizable, or has no matching hash in the database. |
| `AUTH_TOKEN_EXPIRED` | 401 Unauthorized | Refresh token exists but its `ExpiresAt` is in the past. |
| `AUTH_TOKEN_REVOKED` | 401 Unauthorized | Refresh token has been explicitly revoked. |
| `AUTH_TOKEN_CONSUMED` | 401 Unauthorized | Refresh token has already been consumed (replay detected). Triggers immediate session revocation. This error is security-significant and must be logged. |
| `AUTH_SESSION_EXPIRED` | 401 Unauthorized | The session associated with the refresh token has exceeded its absolute or idle timeout. |
| `AUTH_SESSION_REVOKED` | 401 Unauthorized | The session has been explicitly revoked (logout, password change, admin action, etc.). |
| `AUTH_ACCESS_TOKEN_INVALID` | 401 Unauthorized | The Bearer token in the Authorization header fails signature validation or is malformed. |
| `AUTH_ACCESS_TOKEN_EXPIRED` | 401 Unauthorized | The Bearer token in the Authorization header has expired. Distinct from the refresh token expiry error so clients can distinguish "refresh now" from "login again". |

---

### 7.5 Email Verification Errors

| Code | HTTP Status | When It Occurs |
|---|---|---|
| `AUTH_VERIFICATION_TOKEN_INVALID` | 400 Bad Request | Token string does not match any record. |
| `AUTH_VERIFICATION_TOKEN_EXPIRED` | 400 Bad Request | Token exists but is past its 24-hour expiry. |
| `AUTH_VERIFICATION_TOKEN_USED` | 400 Bad Request | Token was already consumed in a previous verification. |

---

### 7.6 Password Errors

| Code | HTTP Status | When It Occurs |
|---|---|---|
| `AUTH_RESET_TOKEN_INVALID` | 400 Bad Request | Password reset token string does not match any record. |
| `AUTH_RESET_TOKEN_EXPIRED` | 400 Bad Request | Reset token is past its 1-hour expiry. |
| `AUTH_RESET_TOKEN_USED` | 400 Bad Request | Reset token was already consumed. |
| `AUTH_SAME_PASSWORD` | 400 Bad Request | The new password submitted in a change-password or reset flow is identical to the current password. |
| `AUTH_WRONG_CURRENT_PASSWORD` | 400 Bad Request | The current-password field in a change-password request does not match the stored hash. |

---

### 7.7 Authorization Errors

| Code | HTTP Status | When It Occurs |
|---|---|---|
| `AUTH_UNAUTHORIZED` | 401 Unauthorized | A protected endpoint was accessed with no valid Bearer token. |
| `AUTH_FORBIDDEN` | 403 Forbidden | A valid session exists but the user lacks the required role or permission for the requested operation. |

---

## 8. Mappers

Mappers live in `internal/auth/mapper`. Their sole purpose is to transform data between layers without leaking layer-specific concerns across layer boundaries.

### Why Mappers Exist

- The domain entity `User` contains `PasswordHash` — which must never appear in any API response.
- The database row model may contain columns irrelevant to the domain entity.
- The domain entity contains timestamps as `time.Time` — the HTTP response needs RFC 3339 strings.
- DTOs carry validation tags that have no meaning inside the domain layer.
- Service return types should be stable domain entities, not HTTP-shaped structures.

Mappers make each layer's structure independently evolvable. A change to the database schema does not require changing the domain entity, and a change to the API response shape does not require changing the domain entity.

### 8.1 Defined Mapping Paths

```
RegisterRequest (DTO)   ──→  User (domain entity)
LoginRequest (DTO)      ──→  Device (domain entity, the inbound device context)

User (domain entity)    ──→  UserResponse (DTO)
Session (domain entity) ──→  SessionResponse (DTO)
Device (domain entity)  ──→  DeviceResponse (DTO)
RefreshToken (domain)   ──→  TokenPair (DTO)  [combined with token strings from TokenService]
```

### 8.2 Mapping Responsibilities by Mapper

| Mapper | Transforms | Notes |
|---|---|---|
| `UserMapper` | `RegisterRequest → User`, `User → UserResponse` | Must normalize email and username to lowercase. Must never copy `PasswordHash` into a response. |
| `SessionMapper` | `Session → SessionResponse` | Populates `IsCurrent` by comparing session ID against the caller's session context. |
| `DeviceMapper` | `LoginRequest → Device (partial)`, `Device → DeviceResponse` | Translates login request device metadata into a device entity suitable for fingerprint-based lookup or creation. |
| `TokenMapper` | `(AccessToken string, RefreshToken string, expiry times) → TokenPair` | Purely structural. Assembles the `TokenPair` DTO from the raw token strings and expiry values returned by `TokenService`. |
| `AuthMapper` | `(User, Session, TokenPair) → AuthResponse` | Composes the full login/refresh response from its constituent parts. |

---

## 9. Business Invariants

These rules must always hold. Violations are programming errors, not user errors. The service layer is the enforcement point.

| ID | Invariant | Enforcement Point |
|---|---|---|
| INV-01 | Email addresses are globally unique, case-insensitively. | Repository write (unique constraint) + service pre-check. |
| INV-02 | Usernames are globally unique, case-insensitively. | Repository write (unique constraint) + service pre-check. |
| INV-03 | Passwords are never stored in plaintext. | `PasswordService.Hash` must be called before any persistence. |
| INV-04 | Refresh token plaintext is never persisted. | Only the SHA-256 hash is written to the repository. |
| INV-05 | A refresh token is single-use. | `ConsumedAt` is checked before any rotation attempt. |
| INV-06 | Replay of a consumed refresh token revokes the entire session. | `AuthService.Refresh` detects non-nil `ConsumedAt` and triggers session revocation before returning. |
| INV-07 | One session belongs to one user and one device. | Non-nullable foreign keys + service creation logic. |
| INV-08 | One refresh token belongs to one session. | Non-nullable foreign key on the refresh token entity. |
| INV-09 | Only `active` users may create sessions. | `AuthService.Login` checks `User.Status == active` before proceeding. |
| INV-10 | A revoked session may not issue new refresh tokens. | `AuthService.Refresh` checks `Session.Revoked` and `Session.ExpiresAt` before issuing. |
| INV-11 | Every registered user holds at least the `user` role. | `AuthService.Register` assigns the role in the same transaction as user creation. |
| INV-12 | The maximum active device count per user is bounded by `AppConfig.MaxDevicesPerUser`. | `AuthService.Login` calls `DeviceRepository.CountActiveDevices` and compares against the config value before creating a new device record. |
| INV-13 | Revoking a device revokes all its sessions. | `DeviceService.RevokeDevice` calls `SessionRepository.RevokeAllByDevice` in the same transaction. |
| INV-14 | Password change invalidates **all** existing sessions including the one that triggered the change. | `AuthService.ChangePassword` calls `SessionRepository.RevokeAllByUser` in the same transaction. No session survives a password change — full re-authentication is always required. |
| INV-15 | At most one valid email verification token exists per user at any time. | `EmailVerificationTokenRepository.InvalidateAllForUser` is called before creating a new one. |
| INV-16 | At most one valid password reset token exists per user at any time. | `PasswordResetTokenRepository.InvalidateAllForUser` is called before creating a new one. |
| INV-17 | Access tokens contain only the minimum viable payload (user ID, session ID, roles). | Enforced by the `TokenService.IssueAccessToken` contract — the claims struct has a fixed, limited field set. |
| INV-18 | Soft-deleted users cannot authenticate. | `AuthService.Login` checks `User.Status != deleted`. |

---

## 10. Dependency Rules

### Allowed Dependency Directions

```
HTTP Handler
    ↓ (calls)
Service Interface
    ↓ (calls)
Repository Interface  ←→  TokenService  ←→  PasswordService
    ↓ (calls)
Infrastructure (pgx, Redis, JWT library, email client)
```

### Strict Prohibitions

| Layer | May NOT import |
|---|---|
| `domain` | Anything outside the Go standard library. No Fiber, no pgx, no Redis, no JWT, no email client, no uuid library. |
| `dto` | Domain entities. DTOs are independent data shapes. |
| `repository` (interface definitions) | Infrastructure packages. The interface is a pure Go interface. |
| `service` (interface definitions) | Infrastructure packages, Fiber types. |
| `token` (contracts) | Any JWT library. Contracts describe behavior, not implementation. |
| `errors` | Fiber. Uses only `net/http` for status codes and the existing `internal/errors` `AppError` type. |
| `mapper` | Fiber, pgx, Redis. Mappers translate between domain and DTO only. |

### Rationale

These prohibitions exist to make each layer independently testable and replaceable. The service layer can be tested with mock repositories without a database. The HTTP handler layer can be tested with mock services without running an HTTP server. Changing the JWT library requires touching only the `token` package's implementation file — the contracts and all callers remain unchanged.

---

## 11. Package Responsibilities

### `internal/auth/domain`

**Purpose:** The business vocabulary of authentication.  
**Owns:** Pure Go structs representing entities. Status enums. Platform enums.  
**Allowed dependencies:** Go standard library only.  
**Forbidden:** Everything else.  
**Extensibility:** New entity fields are added here first. New entities (e.g., `OAuthIdentity`, `Passkey`) are added here as pure structs.

---

### `internal/auth/dto`

**Purpose:** The communication vocabulary at layer boundaries.  
**Owns:** Request structs with validation tags. Response structs with JSON tags.  
**Allowed dependencies:** `go-playground/validator` tags (compile-time annotations only). Go standard library.  
**Forbidden:** Domain entities (by import). Infrastructure.  
**Extensibility:** New API endpoints add new DTOs. Existing DTOs may gain fields but must not remove or rename fields in ways that break the public API contract.

---

### `internal/auth/repository`

**Purpose:** Storage contracts for the authentication domain.  
**Owns:** Pure Go interfaces describing persistence operations.  
**Allowed dependencies:** `internal/auth/domain`. `context` package (for transaction propagation). `github.com/google/uuid`.  
**Forbidden:** pgx, Redis, Fiber, any HTTP type.  
**Extensibility:** New query patterns for new features are added as new methods on existing interfaces or as new interfaces.

---

### `internal/auth/service`

**Purpose:** Business operation contracts.  
**Owns:** Pure Go interfaces describing business operations. The `PasswordService` and `TokenService` interfaces also live here.  
**Allowed dependencies:** `internal/auth/domain`. `internal/auth/dto`. `internal/auth/errors`. `context`. `github.com/google/uuid`.  
**Forbidden:** Fiber, pgx, Redis, any JWT library.  
**Extensibility:** New flows (OAuth, MFA) are added as new service interfaces or as new methods on `AuthService`.

---

### `internal/auth/token`

**Purpose:** Token issuance and verification contracts.  
**Owns:** The `TokenService` interface contract. The `AccessTokenClaims` value type.  
**Allowed dependencies:** `internal/auth/domain`. `time`. `github.com/google/uuid`.  
**Forbidden:** Any JWT library (those are implementation details). Fiber. pgx.  
**Extensibility:** Support for asymmetric signing algorithms (RS256, ES256) is an implementation change behind this contract, not a contract change.

---

### `internal/auth/errors`

**Purpose:** Domain-specific error vocabulary.  
**Owns:** `AUTH_*` error code constants. Constructor functions for each error code.  
**Allowed dependencies:** `internal/errors` (the global `AppError` and `Code` types). `net/http` (for status codes).  
**Forbidden:** Fiber, pgx, Redis, domain entities.  
**Extensibility:** New error codes are added here as new constants and constructors.

---

### `internal/auth/mapper`

**Purpose:** Data translation between domain entities and DTOs.  
**Owns:** Pure transformation functions. No side effects.  
**Allowed dependencies:** `internal/auth/domain`. `internal/auth/dto`.  
**Forbidden:** Fiber, pgx, Redis, any infrastructure.  
**Extensibility:** New mappings for new response shapes or new domain entities are added as new functions.

---

## 12. Future Extensions

The contracts defined above are designed to accommodate the following capabilities. The extension points are identified here so that the implementation does not inadvertently close off these paths.

### 12.1 OAuth / Social Login

- The `AuthService` interface gains an `AuthenticateWithOAuth` operation.
- A new `OAuthIdentity` domain entity is added to `internal/auth/domain`.
- A new `OAuthIdentityRepository` interface is added to `internal/auth/repository`.
- The `LoginRequest` DTO gains an `OAuthProvider` and `OAuthCode` variant (or a separate `OAuthLoginRequest` DTO is defined).
- Once an OAuth identity resolves to a user, the session creation path is identical to the password-based path. No changes to `Session`, `Device`, `RefreshToken`, or `TokenService`.
- The `UserResponse` DTO gains a `LinkedProviders []string` field.

### 12.2 Passkeys (WebAuthn)

- A new `Passkey` domain entity is added.
- A new `PasskeyRepository` is added.
- The `AuthService` interface gains `InitiatePasskeyRegistration`, `CompletePasskeyRegistration`, `InitiatePasskeyAssertion`, and `CompletePasskeyAssertion` operations.
- `Session` and `Device` entities are unchanged — passkeys are an alternative authentication method, not a different session model.

### 12.3 Multi-Factor Authentication (MFA)

- A new `MfaFactor` domain entity is added.
- The `User` entity gains an `MfaEnabled` flag (backward-compatible addition).
- The `LoginRequest` gains an optional `MfaCode` field (validated only when the user has MFA enabled).
- A new `pending_mfa` transient state is introduced — but this is a transient state in the service layer, not a persisted entity state.
- The `Session` entity gains an `MfaVerified` flag.
- A new `MfaService` interface is defined in `internal/auth/service`.

### 12.4 Enterprise SSO (SAML / OIDC)

- A new `Organization` domain entity is added (owned by the organization domain, referenced here).
- A new `SSOConfig` entity associates an organization with a SAML/OIDC provider configuration.
- The role assignment model gains an optional `OrganizationID` scope field on the assignment.
- The `AuthService` gains an `AuthenticateWithSSO` operation.

### 12.5 Permission Overrides per User

- A new `UserPermission` entity is added representing a direct grant or deny on a specific user.
- `RoleService.GetUserPermissions` is updated to merge role-derived permissions with user-level overrides (denies take precedence over grants).
- No breaking change to any existing interface.

### 12.6 Account Reactivation (Grace Period)

- A new `ReactivateAccount` operation is added to `AuthService`.
- Input: email + password, or a reactivation token sent to the user's email.
- Output: transitions the account from `deleted` back to `active` if within the grace period window.

---

## Appendix: Resolved Design Decisions

All pre-implementation decisions have been resolved. The answers below are authoritative; the contract sections above have been updated to reflect them.

| # | Question | Decision | Downstream Impact |
|---|---|---|---|
| Q1 | Where is the refresh token delivered? | **Both, by client type.** Browser Extension and Web Dashboard → HttpOnly Secure cookie set by the handler. React Native → response body, stored in platform secure store (Android Keystore / iOS Keychain via `react-native-keychain`). | `RefreshRequest` note updated. Handler reads `Platform` from `LoginRequest` to choose delivery path. Service layer is delivery-agnostic. |
| Q2 | Does password change revoke all sessions or all except the current one? | **All sessions, including the current one.** The client must immediately re-authenticate. | `AuthService.ChangePassword` updated. `SessionRepository.RevokeAllByUser` is the correct operation. No `RevokeAllExcept` variant is needed. INV-14 updated. |
| Q3 | Rate-limit lockout — domain level or middleware only? | **Middleware only** (for now). No `LoginAttempt` entity or `LockoutPolicy` service is required in this iteration. | No domain changes. Rate-limiting is already handled by the existing `ratelimit` middleware. |
| Q4 | Should `GetUserPermissions` live on `RoleService` or a standalone `PermissionService`? | **Standalone `PermissionService`** (§5.5). Owns permission resolution and the Redis cache layer. `RoleService` signals it to invalidate the cache on role mutations. | `RoleService` table updated. `PermissionService` section added as §5.5. `PasswordService` renumbered to §5.6. |
| Q5 | JWT signing algorithm? | **HS256 to start, but the `TokenService` interface is algorithm-agnostic.** `JWTConfig` gains an `Algorithm` field. Swapping to RS256/ES256 requires only a new implementation behind the same interface. | Token signing contract updated. `JWTConfig` extension note updated. |
| Q6 | Is the device limit hardcoded or configurable? | **Configurable** via `AppConfig.MaxDevicesPerUser int`. | `AppConfig` must be extended. `DeviceRepository.CountActiveDevices` result is compared against the config value by the service. INV-12 updated. |
| Q7 | Should login accept email only, username only, or either? | **Either.** The `LoginRequest.Identifier` field accepts both. The repository detects the form (email contains `@`; otherwise username) and routes to the appropriate lookup. | `LoginRequest.Email` renamed to `LoginRequest.Identifier`. `UserRepository` gains a `FindByIdentifier` operation. `AuthService.Login` updated. |

---

> **Implementation order:** `domain` → `errors` → `dto` → `token` → `mapper` → `repository` → `service`  
> All design decisions are resolved. Implementation may begin.
