---
title: Profile Domain Design
description: Architecture design for the Profile domain of the StreamTogether backend. Covers the social-facing identity model, entity definition, lifecycle, ownership, privacy boundary, read models, validation, security, domain errors, and interactions with future Social Graph modules.
ms.date: 2026-08-31
status: draft
phase: 3 — Social Graph
step: 3.1.1
depends-on: authentication-domain-design.md, authentication-domain-contracts.md
---

# Profile Domain Design

## Table of Contents

1. [Domain Purpose](#1-domain-purpose)
2. [Profile Entity](#2-profile-entity)
3. [Initial Profile Fields](#3-initial-profile-fields)
4. [Username](#4-username)
5. [Display Name](#5-display-name)
6. [Bio](#6-bio)
7. [Avatar](#7-avatar)
8. [Profile Lifecycle](#8-profile-lifecycle)
9. [User Deletion / Account Deactivation](#9-user-deletion--account-deactivation)
10. [Ownership](#10-ownership)
11. [Privacy Boundary](#11-privacy-boundary)
12. [Profile Read Model](#12-profile-read-model)
13. [Validation Rules](#13-validation-rules)
14. [Security](#14-security)
15. [Future Module Interactions](#15-future-module-interactions)
16. [Domain Errors](#16-domain-errors)
17. [Open Decisions](#17-open-decisions)

---

## 1. Domain Purpose

### What Profile Represents

A Profile is the **social-facing identity** of a user on StreamTogether. It contains the information a user presents to others on the platform — how they appear in friend lists, party rooms, and search results.

A Profile answers the question: _"Who does this user want to be seen as on the platform?"_

### User vs Profile

The distinction between `User` and `Profile` is strict and intentional:

| Concept | Owner | Contains |
|---|---|---|
| **User** | `internal/auth/` | Email, password hash, username, account status, email verification state, sessions, devices, OAuth identities, RBAC roles. |
| **Profile** | `internal/profile/` | Display name, bio, avatar URL, social presentation timestamps. |

**What belongs in User (must NOT be duplicated in Profile):**
- Email address — an authentication credential, not a social attribute.
- Password hash — authentication secret.
- Username — the unique, immutable-by-convention identifier used in @-mentions and lookup. Lives in `User`.
- Account status (`active`, `suspended`, `deleted`) — governs platform access, not social presentation.
- Sessions, devices, refresh tokens — authentication infrastructure.
- Roles and permissions — authorization infrastructure.

**What belongs in Profile (must NOT be placed in User):**
- Display name — the friendly name a user chooses to appear as to others.
- Bio — a short self-description visible to other users.
- Avatar URL — a reference to the user's profile picture.
- Timestamps for social data mutations.

### Separation Rationale

Mixing social presentation into the authentication domain would:
- Force the auth domain to change for social product decisions.
- Expose social data through the authentication layer.
- Make the auth domain responsible for content validation (bio text, image URLs) that is outside its scope.

The Profile domain references the User by `user_id`. It does not replicate authentication data.

---

## 2. Profile Entity

The Profile entity lives in `internal/profile/domain`. It is a pure Go struct with no serialization tags, no database tags, and no framework dependencies.

### Entity Summary

```
Profile
├── id           UUID        — surrogate primary key, system-assigned
├── user_id      UUID        — FK to auth.User, unique (1:1 relationship)
├── display_name string?     — user-chosen presentational name
├── bio          string?     — short self-description
├── avatar_url   string?     — HTTPS reference to profile picture
├── created_at   time.Time   — immutable creation timestamp
└── updated_at   time.Time   — updated on every mutation
```

### Field Specification

| Field | Type | Required | Default | Purpose | User-Editable | Publicly Visible |
|---|---|---|---|---|---|---|
| `ID` | `uuid.UUID` | Yes | System-assigned | Surrogate primary key. | No | No |
| `UserID` | `uuid.UUID` | Yes | From registration | Immutable FK to `auth.User`. Enforces 1:1 profile–user relationship. | No | No |
| `DisplayName` | `*string` | No | `nil` | User-chosen name displayed in social contexts. Falls back to username in read models when nil. | Yes | Yes |
| `Bio` | `*string` | No | `nil` | Short self-description. Empty/nil means no bio set. | Yes | Yes (conditionally) |
| `AvatarURL` | `*string` | No | `nil` | HTTPS URL referencing the user's profile picture. Nil means no avatar. | Yes | Yes (conditionally) |
| `CreatedAt` | `time.Time` | Yes | Set at profile creation | Immutable. Records when the profile was first created. | No | No |
| `UpdatedAt` | `time.Time` | Yes | Set at profile creation, updated on mutation | Tracks last profile modification. | No | No |

### Invariants

- `UserID` is set once at creation and never mutated.
- `ID` is set once at creation and never mutated.
- `CreatedAt` is set once at creation and never mutated.
- `UpdatedAt` must be updated on every field mutation.
- A `nil` `DisplayName` is a valid state meaning "user has not set a display name."
- A `nil` `Bio` is a valid state meaning "user has no bio."
- A `nil` `AvatarURL` is a valid state meaning "user has no avatar."
- There is exactly one Profile per User. The `user_id` column carries a unique constraint.

---

## 3. Initial Profile Fields

The following analysis evaluates which fields are required for the Social Graph.

### Included

| Field | Justification |
|---|---|
| `id` | Every entity requires a stable primary key. |
| `user_id` | The link to the identity aggregate. Without it, a Profile is meaningless. |
| `display_name` | Required for friends lists, party rooms, and presence display. |
| `bio` | Required for the profile page view. Users expect to describe themselves. |
| `avatar_url` | Required for avatar display in party rooms, friend lists, and chat. |
| `created_at` | Audit and ordering. |
| `updated_at` | Audit and cache invalidation. |

### Excluded

| Field | Reason for Exclusion |
|---|---|
| `username` | Lives in `auth.User`. Must not be duplicated. See §4. |
| `email` | Authentication credential. Never in Profile. |
| `online_status` / `presence` | Belongs to the Presence module (Phase 3.5). Dynamic state, not profile data. |
| `friend_count` | Derived aggregate. Belongs to the Friends module, not stored on Profile. |
| `social_links` | Speculative. No product requirement exists for this yet. |
| `location` | Speculative. No product requirement exists for this yet. |
| `banner_url` | Speculative. Adds image storage complexity without a confirmed product need. |

---

## 4. Username

### Decision

**Username remains exclusively in `auth.User`. Profile does NOT store or duplicate username.**

### Rationale

1. `username` is the unique, canonical, case-insensitive identifier used for authentication lookups and @-mentions. It is owned by the auth domain.
2. Duplicating username in Profile would create a write-consistency problem: a username change (if ever allowed) would require updating two tables atomically across two domains.
3. No domain boundary principle justifies copying username into Profile.

### Username in Response Models

When a response model requires both username and profile data (e.g., a public profile view), the **service layer** or **mapper** resolves username from `auth.User` and assembles it into a composite read model. The Profile entity itself does not store username.

This is a **read-only projection** concern, handled at the application service or query layer — not a Profile data model concern.

### Cross-Domain Query Strategy

The profile service may call into the auth domain (or its repository layer) to retrieve username when building composite read models. Alternatively, a dedicated profile read query can join `users` and `profiles` tables at the database layer, keeping the result typed at the DTO level without merging the domain entities.

---

## 5. Display Name

### Definition

`display_name` is the **user-chosen presentational name** shown to other users in social contexts: friend lists, party rooms, chat, and search results.

### Relationship to Username

| Attribute | `username` | `display_name` |
|---|---|---|
| Location | `auth.User` | `profile.Profile` |
| Uniqueness | Globally unique | Not required to be unique |
| Mutability | Effectively immutable (treated as a stable handle) | Freely editable by the user |
| Purpose | Platform-wide unique handle, @-mention identifier | Friendly presentational name |
| Display fallback | Primary identifier when display_name is nil | Shown when set; otherwise username is shown |

A user whose `display_name` is `nil` is displayed using their `username`. A user may set a `display_name` identical to their `username`. A user may change `display_name` at any time without affecting `username`.

### Validation Constraints

| Rule | Value |
|---|---|
| Minimum length | 2 characters (after trimming) |
| Maximum length | 50 characters |
| Allowed characters | Unicode letters, Unicode digits, spaces, hyphens (`-`), underscores (`_`), periods (`.`) |
| Prohibited patterns | Leading/trailing whitespace (stripped on input), consecutive internal spaces (collapsed to single space) |
| Case | Preserved as-entered (display name is presentational, not an identifier) |
| Empty string | Treated as `nil` (clears the display name) |

### Uniqueness

Display names are **not required to be unique**. Multiple users may share the same display name. Unique identification is the role of `username`.

---

## 6. Bio

### Definition

`bio` is a short, plain-text self-description that a user may optionally set on their profile.

### Semantics

| Attribute | Value |
|---|---|
| Data type | `*string` (nullable) |
| Maximum length | 300 characters |
| Minimum length | 0 (empty string treated as nil — no bio set) |
| Format | Plain text only. No Markdown, no HTML. |
| Whitespace handling | Leading and trailing whitespace stripped on input. Internal whitespace preserved as-entered. Consecutive newlines collapsed to a maximum of two (one blank line). |
| Empty/nil | A nil bio means the user has not set one. An empty string submitted by the user clears the bio (stored as nil). |
| User-editable | Yes. The user may update or clear their bio at any time. |

### Formatting

No Markdown or rich-text support is defined. The bio is rendered as plain text. Future product decisions may introduce formatting; that will be handled at that time.

### Encoding

The bio is stored as UTF-8. Unicode is fully supported. Characters are counted in Unicode code points, not bytes or UTF-16 code units.

---

## 7. Avatar

### Definition

`avatar_url` is an HTTPS URL referencing the user's profile picture. The Profile domain stores only the **reference** (URL string). Binary image data is never stored in PostgreSQL.

### Storage Strategy

Image files are not stored by the Profile domain. The avatar URL points to an externally hosted image (CDN, object storage, or future image service). The Profile domain is responsible only for storing, validating, and retrieving the URL string.

Image upload and storage are **out of scope for this design document** and will be addressed when the media/upload infrastructure is designed.

### Field Semantics

| Attribute | Value |
|---|---|
| Data type | `*string` (nullable) |
| Nullable | Yes. Nil means the user has no avatar. The client renders a default/placeholder. |
| Scheme | HTTPS only. HTTP URLs are rejected. |
| Maximum URL length | 2048 characters |
| Minimum URL length | 10 characters (sanity bound) |
| User-editable | Yes. The user may update or clear their avatar URL. |

### Update Semantics

- Setting `avatar_url` to a non-empty, valid HTTPS URL replaces the current avatar reference.
- Submitting an empty string or explicit null clears the avatar (stored as nil).
- No validation of whether the URL resolves to an actual image is performed at write time. URL format validation only.

### Default Avatar

No default avatar URL is stored in the Profile record. A nil `avatar_url` means "no avatar." Clients are responsible for rendering a fallback (e.g., initials-based placeholder).

---

## 8. Profile Lifecycle

### Creation

A Profile is created **atomically at user registration**, in the same database transaction that creates the `auth.User` record.

**Rationale:**
- Social Graph features are a core product capability. Every registered user is a potential social participant.
- Lazy creation (creating the profile on first social access) introduces a failure mode where a user exists without a profile, requiring guards throughout every social feature.
- Creating the profile eagerly eliminates a class of "profile not found" errors for authenticated users.

**At creation time:**
- `id` is system-assigned (new UUID).
- `user_id` is set to the newly created User's ID.
- `display_name` is `nil` (user has not set one yet).
- `bio` is `nil`.
- `avatar_url` is `nil`.
- `created_at` and `updated_at` are set to the current UTC timestamp.

**Registration paths:**

| Path | Profile Creation Behavior |
|---|---|
| Password-based registration | Profile created atomically with User in the same transaction. |
| OAuth provisioning (new user) | Profile created atomically with User during OAuth callback processing. |
| Admin-created account | Profile created atomically with User. |

### Update

- A user may update `display_name`, `bio`, and `avatar_url` at any time while their account is `active`.
- `updated_at` is set to the current UTC timestamp on every successful update.
- Partial updates are supported: a user may update only `bio` without providing `display_name` or `avatar_url`.
- Updating a field to its existing value is a no-op that still updates `updated_at`.

### Retrieval

- A profile may be retrieved by `user_id` (primary lookup path).
- A profile may be retrieved by `id` (secondary lookup path, useful for internal joins).
- No profile may be retrieved by email or password — those are authentication domain fields.

### Deletion / Deactivation

Profile deletion is not independent of user account deletion. See §9.

---

## 9. User Deletion / Account Deactivation

Profile follows the `auth.User` lifecycle exactly. There is no independent Profile lifecycle.

| User Status | Profile Behavior |
|---|---|
| `pending_verification` | Profile exists (created at registration). Content is not publicly accessible until the account is active. |
| `active` | Profile is fully accessible per privacy settings. |
| `suspended` | Profile data is preserved but the profile is not publicly accessible. Social operations (friend requests, party joins) are blocked at the service layer. |
| `soft-deleted` | Profile data is preserved for the duration of the grace period. Public access is blocked. |
| `hard-deleted` | Profile record is permanently deleted in the same purge job that removes User data. |

**Key constraints:**
- Profile suspension and reactivation are driven by `auth.User.Status`. The Profile domain does not store its own status field.
- Profile deletion is triggered by User deletion. There is no endpoint to delete a profile independently of the user account.
- During the soft-delete grace period, profile data is retained so that account reactivation restores the full social identity.

---

## 10. Ownership

### Ownership Rule

A Profile is owned by exactly one User. The `user_id` field on Profile is the immutable ownership binding.

**A user may only modify their own Profile.** Any service method that performs a profile update must verify that the authenticated user's ID matches the `user_id` of the Profile being modified.

This check is enforced at the **service layer**, not in the domain entity itself. The service receives the acting user's ID from the authentication middleware and compares it to `Profile.UserID` before performing any mutation.

### Ownership vs Authorization

Ownership (can I modify this?) is distinct from authorization (am I allowed to read this?).

- **Ownership** governs writes. Only the profile owner may update their profile.
- **Authorization** governs reads. Whether User A can read User B's profile is controlled by the future Privacy module.

These concerns must not be conflated. The Profile domain enforces ownership on writes. Privacy rules on reads are deferred to the Privacy module.

### Administrative Override

Administrators (users with the appropriate RBAC role) may be granted the ability to modify or remove profile content as a moderation action. This is an authorization policy enforced at the handler layer using the existing RBAC infrastructure, not a Profile domain concern.

---

## 11. Privacy Boundary

### Separation of Profile Data from Privacy Policy

This document defines which Profile fields are **potentially public** or **potentially private**. It does not implement privacy rules. Visibility enforcement is the responsibility of the future Privacy module (Phase 3.7).

### Field Privacy Classification

| Field | Default Visibility | Privacy Module Control |
|---|---|---|
| `display_name` | Public | May be restricted (e.g., friends-only display name) |
| `bio` | Public | May be restricted by user privacy settings |
| `avatar_url` | Public | May be restricted by user privacy settings |
| `created_at` | Private | Not relevant to social consumers |
| `updated_at` | Private | Not relevant to social consumers |
| `user_id` | Internal | Never exposed in public API responses |
| `id` | Internal | Never exposed in public API responses |

### What "Public" Means Here

"Public" in this document means the field is conceptually intended to be visible to other users on the platform, subject to future privacy configuration. It does NOT mean the field is currently unrestricted.

Until the Privacy module is implemented, profile fields should be treated as **visible only to authenticated users** (not anonymous visitors).

---

## 12. Profile Read Model

Two conceptual read models are defined. These are not entity definitions — they describe what information a response should contain. DTOs will be defined in `internal/profile/dto/` during implementation.

### 12.1 Own Profile View

Returned when an authenticated user reads their **own** profile. Contains all editable fields plus the username resolved from `auth.User`.

```
OwnProfileView
├── username       string    — from auth.User (read-only projection)
├── display_name   string?   — nil if not set
├── bio            string?   — nil if not set
├── avatar_url     string?   — nil if not set
└── updated_at     time.Time — last profile modification time
```

### 12.2 Public Profile View

Returned when an authenticated User A reads User B's profile. Does not include timestamps or internal identifiers. Subject to future privacy filtering.

```
PublicProfileView
├── username       string    — from auth.User (read-only projection)
├── display_name   string?   — nil displays as username on the client
├── bio            string?   — nil if not set
└── avatar_url     string?   — nil displays as default placeholder on the client
```

### 12.3 Minimal Profile Projection

A compact representation used by other domains (Friends, Party, Presence, Chat) that need to render a user identity without loading the full profile.

```
ProfileSummary
├── user_id        UUID      — for correlation with other domain entities
├── username       string    — from auth.User
├── display_name   string?   — nil if not set
└── avatar_url     string?   — nil if not set
```

`ProfileSummary` is a DTO composed from both `auth.User` and `profile.Profile`. It is assembled at the service or query layer.

---

## 13. Validation Rules

All validation applies to user-submitted input. System-set fields (`id`, `user_id`, `created_at`, `updated_at`) are never validated on user input.

### Display Name

| Rule | Constraint |
|---|---|
| Minimum length | 2 Unicode code points (after whitespace trimming) |
| Maximum length | 50 Unicode code points |
| Allowed characters | Unicode letters (`\p{L}`), Unicode digits (`\p{N}`), spaces (` `), hyphens (`-`), underscores (`_`), periods (`.`) |
| Normalization | Trim leading/trailing whitespace; collapse consecutive internal spaces to a single space |
| Empty string | Treated as nil (clears the display name); not rejected as an error |
| Null submission | Accepted; clears the display name |
| Encoding | UTF-8 |

### Bio

| Rule | Constraint |
|---|---|
| Maximum length | 300 Unicode code points |
| Format | Plain text. HTML and Markdown tags must be stripped or rejected. |
| Whitespace | Trim leading/trailing whitespace; collapse runs of more than 2 consecutive newlines to 2 |
| Empty string | Treated as nil (clears the bio) |
| Null submission | Accepted; clears the bio |
| Encoding | UTF-8 |

### Avatar URL

| Rule | Constraint |
|---|---|
| Scheme | Must be `https://`. HTTP is rejected. |
| Format | Must parse as a valid URL per RFC 3986 |
| Maximum length | 2048 characters |
| Minimum length | 10 characters (sanity bound; prevents trivially invalid strings) |
| Empty string | Treated as nil (clears the avatar) |
| Null submission | Accepted; clears the avatar |
| Reachability | Not validated at write time. No DNS or HTTP probe is performed. |
| Encoding | ASCII-only for scheme/host/path; percent-encoding for non-ASCII path segments |

### Normalization Summary

Before persistence, the service layer must apply:

1. Trim leading and trailing whitespace from `display_name` and `bio`.
2. Collapse consecutive internal spaces in `display_name`.
3. Collapse consecutive newlines (> 2) in `bio`.
4. Convert empty strings to `nil` for nullable fields.

---

## 14. Security

The following security concerns are identified. Mechanisms are not implemented here.

### XSS via Profile Content

`display_name` and `bio` are rendered in HTML contexts (web extension, mobile WebView panels). Unsanitized output will execute injected scripts.

**Requirement:** All profile text content must be HTML-escaped at render time. The backend must reject or strip HTML tags at the validation layer.

### Malicious Avatar URLs

A stored avatar URL could point to:
- A tracker or analytics pixel.
- A URL that changes its target after storage (link hijacking).
- An external server that logs requestor IPs.
- A non-image resource used to exploit image parsers on the client.

**Requirement:**
- Scheme must be `https://` (rejects data URIs and http).
- URL domain should eventually be validated against an allow-list of trusted CDN/storage domains when the image storage infrastructure is defined.
- The client must fetch avatar images through a proxied or allowlisted path, not by directly resolving arbitrary URLs.

### Oversized Input

Unbounded input can cause excessive memory allocation and storage costs.

**Requirement:** Enforce all length limits at the service layer before persistence. The HTTP handler must also enforce a maximum request body size (handled by existing middleware).

### Authorization / Ownership Enforcement

A user must not be able to modify another user's profile by submitting a different user ID.

**Requirement:** The service layer must resolve the acting user's ID from the authenticated JWT claims and compare it to `Profile.UserID`. User-submitted `user_id` values in request bodies must be ignored; the acting user's ID is taken from the validated token only.

### Sensitive Information Leakage

The `UserID` and Profile `ID` (internal UUIDs) must not be returned in public-facing API responses. These are internal correlation identifiers.

**Requirement:** Public response DTOs must not include `UserID` or Profile `ID`. The username serves as the public identifier.

### Profile Enumeration

An unauthenticated API endpoint returning profiles by sequential or predictable identifiers could be used to enumerate all registered users.

**Requirement:**
- Profile lookup must require authentication.
- Profile lookup by `user_id` (UUID) is not guessable by enumeration.
- Any future public profile endpoint (e.g., `/u/:username`) must be rate-limited and must not reveal whether a username exists for unauthenticated requests (consistent timing / uniform responses).

---

## 15. Future Module Interactions

The following describes conceptual dependencies between Profile and future Social Graph modules. None of these modules are implemented here.

### Friends (Phase 3.2)

- Friend list rendering requires: `username`, `display_name`, `avatar_url` per friend.
- The Friends module will consume `ProfileSummary` (§12.3) to display friend identities.
- The Profile domain does not store friend relationships. Friends is a separate module with its own entities.

### Friend Requests (Phase 3.3)

- Friend request notifications display the sender's `display_name` and `avatar_url`.
- The Friend Requests module references `user_id` and resolves `ProfileSummary` at read time.

### Search (Phase 3.4)

- Search operates primarily on `username` (from `auth.User`) and `display_name` (from `profile.Profile`).
- The Search module will need read access to both tables (or a denormalized search index).
- Profile must expose `display_name` as a searchable field.
- `bio` is not indexed for search in the initial implementation (open decision).

### Presence (Phase 3.5)

- Presence associates an online/offline/busy status with a `user_id`.
- Presence is rendered alongside profile data: `display_name`, `avatar_url`, `username`.
- Presence reads `ProfileSummary` when building presence responses.
- Profile does not store presence state. Presence state is dynamic and belongs to the Presence module.

### Blocking (Phase 3.6)

- Blocking operates on `user_id` pairs (blocker → blocked).
- When a block is active, the blocked user's profile data must not be returned to the blocker, and vice versa.
- The Profile service must be aware of (or filter based on) block status, or the Blocking module must filter at the API response layer.
- The exact enforcement point is a decision for the Blocking module design.

### Privacy (Phase 3.7)

- The Privacy module will control per-field visibility for Profile fields.
- Fields `display_name`, `bio`, and `avatar_url` are subject to privacy policy.
- The Privacy module is the enforcement layer; the Profile domain is the data source.
- Profile must not implement privacy rules itself. It provides data; Privacy decides what is visible.

### User Preferences (Phase 3.8)

- User Preferences stores per-user configuration (notification settings, theme, language, etc.).
- Preferences are not part of Profile. They are a separate entity with separate lifecycle concerns.
- Profile domain has no dependency on User Preferences.

---

## 16. Domain Errors

Profile-specific error codes follow the existing pattern established in `internal/auth/errors/errors.go`. All codes extend `apperrors.Code` (string type from `internal/errors`).

Domain errors live in `internal/profile/errors/`.

### Error Code Catalogue

| Code | HTTP Status | Trigger Condition |
|---|---|---|
| `PROFILE_NOT_FOUND` | 404 Not Found | No profile exists for the given `user_id`. |
| `PROFILE_ALREADY_EXISTS` | 409 Conflict | Attempt to create a second profile for a user who already has one. |
| `PROFILE_UPDATE_FORBIDDEN` | 403 Forbidden | Authenticated user attempts to modify a profile they do not own. |
| `PROFILE_INVALID_DISPLAY_NAME` | 400 Bad Request | `display_name` fails length or character validation. |
| `PROFILE_INVALID_BIO` | 400 Bad Request | `bio` exceeds maximum length. |
| `PROFILE_INVALID_AVATAR_URL` | 400 Bad Request | `avatar_url` is not a valid HTTPS URL or exceeds maximum length. |

### Reuse of Global Errors

The following global errors from `internal/errors/builder.go` are reused by the Profile domain without defining new codes:

| Scenario | Reused Error |
|---|---|
| Database query failure | `NewDatabase(cause)` |
| Authentication required (no token) | `NewUnauthorized(message)` — emitted by auth middleware, not Profile service |
| Invalid request body format | `NewBadRequest(message)` |
| Validation failure (field-level) | `NewValidation(message, details)` |

---

## 17. Open Decisions

The following decisions affect the implementation of the Profile domain and must be resolved before or during Phase 3.1 implementation. These are genuine uncertainties, not design gaps.

| # | Decision | Options | Impact |
|---|---|---|---|
| OD-1 | **Are display names unique across the platform?** | (a) Not unique — multiple users can share a display name. (b) Unique — requires a unique index and conflict error. | Recommendation: **Not unique.** Username is the unique handle. Display names are presentational. Uniqueness adds friction with minimal benefit for a watch-party platform. |
| OD-2 | **Should profiles be created automatically (eagerly) at registration, or lazily on first social access?** | (a) Eager — created in the same transaction as User creation. (b) Lazy — created on first profile read or explicit setup. | Recommendation: **Eager.** Eliminates "profile not found for active user" edge cases throughout the Social Graph. This document assumes eager creation. |
| OD-3 | **What avatar storage service will be used?** | (a) AWS S3, (b) Cloudflare R2, (c) GCS, (d) self-hosted MinIO, (e) third-party CDN. | Affects URL domain allow-listing and the future upload flow. No decision needed yet, but affects avatar URL validation constraints. |
| OD-4 | **Should users be allowed to have an empty bio?** | (a) Yes — bio is optional; nil and empty are treated the same. (b) No — once set, bio must contain meaningful content. | Recommendation: **Yes, bio is optional.** Many users will not fill it in. This document assumes nil is always valid. |
| OD-5 | **Should profile deletion be triggered independently of account deletion?** | (a) No — profile is deleted only when the user account is deleted. (b) Yes — users may wipe profile content while keeping their account. | Recommendation: **No independent profile deletion.** A "reset profile" operation (clearing all editable fields) is sufficient and simpler. |
| OD-6 | **Should bio content be searchable?** | (a) Yes — include in the search index. (b) No — search is limited to username and display name. | Affects the Search module design. Bio search adds noise for a watch-party platform. Recommendation: **Not searched initially.** |
| OD-7 | **What is the maximum bio length?** | This document proposes **300 characters**. | Needs product confirmation. 300 is conservative. Instagram uses 150, Twitter uses 160, Discord uses 190. |
| OD-8 | **Should users be able to change their username?** | Relevant because display_name is the alternative for username changes. (a) Username is immutable after registration. (b) Username changes are allowed (limited frequency). | Affects whether display_name truly solves the "I want to be called something different" use case. This is an auth domain decision, not a profile domain decision. Documented here as a dependency. |
| OD-9 | **Should profile content (display_name, bio) be moderated?** | (a) Reactive moderation only (report + admin action). (b) Proactive filtering (profanity filter, length, etc.). | Affects whether the Profile service needs to integrate with a moderation pipeline at write time. |
