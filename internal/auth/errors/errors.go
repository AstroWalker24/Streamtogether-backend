// Package errors defines authentication-domain error codes and constructors
// that extend the global AppError type with AUTH_* codes.
package errors

import (
	"net/http"

	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// Authentication-domain error codes defined by the domain contracts (§7).
const (
	// Registration errors (§7.1)
	CodeEmailExists      apperrors.Code = "AUTH_EMAIL_EXISTS"
	CodeUsernameExists   apperrors.Code = "AUTH_USERNAME_EXISTS"
	CodeUsernameReserved apperrors.Code = "AUTH_USERNAME_RESERVED"
	CodePasswordTooWeak  apperrors.Code = "AUTH_PASSWORD_TOO_WEAK"

	// Login & authentication errors (§7.2)
	CodeInvalidCredentials apperrors.Code = "AUTH_INVALID_CREDENTIALS"
	CodeEmailNotVerified   apperrors.Code = "AUTH_EMAIL_NOT_VERIFIED"
	CodeAccountSuspended   apperrors.Code = "AUTH_ACCOUNT_SUSPENDED"
	CodeAccountDeleted     apperrors.Code = "AUTH_ACCOUNT_DELETED"

	// Device errors (§7.3)
	CodeDeviceLimitReached apperrors.Code = "AUTH_DEVICE_LIMIT_REACHED"
	CodeDeviceRevoked      apperrors.Code = "AUTH_DEVICE_REVOKED"

	// Token & session errors (§7.4)
	CodeTokenInvalid   apperrors.Code = "AUTH_TOKEN_INVALID"
	CodeTokenExpired   apperrors.Code = "AUTH_TOKEN_EXPIRED"
	CodeTokenRevoked   apperrors.Code = "AUTH_TOKEN_REVOKED"
	CodeTokenConsumed  apperrors.Code = "AUTH_TOKEN_CONSUMED"
	CodeSessionExpired apperrors.Code = "AUTH_SESSION_EXPIRED"
	CodeSessionRevoked apperrors.Code = "AUTH_SESSION_REVOKED"

	// Email verification errors (§7.5)
	CodeVerificationTokenInvalid  apperrors.Code = "AUTH_VERIFICATION_TOKEN_INVALID"
	CodeVerificationTokenConsumed apperrors.Code = "AUTH_VERIFICATION_TOKEN_CONSUMED"
	CodeEmailAlreadyVerified      apperrors.Code = "AUTH_EMAIL_ALREADY_VERIFIED"

	// Password reset errors (§7.6)
	CodeResetTokenInvalid  apperrors.Code = "AUTH_RESET_TOKEN_INVALID"
	CodeResetTokenConsumed apperrors.Code = "AUTH_RESET_TOKEN_CONSUMED"

	// OAuth errors (§12.1)
	CodeOAuthStateInvalid         apperrors.Code = "AUTH_OAUTH_STATE_INVALID"
	CodeOAuthProviderError        apperrors.Code = "AUTH_OAUTH_PROVIDER_ERROR"
	CodeOAuthIdentityInvalid      apperrors.Code = "AUTH_OAUTH_IDENTITY_INVALID"
	CodeOAuthEmailConflict        apperrors.Code = "AUTH_OAUTH_EMAIL_CONFLICT"
	CodeOAuthIdentityOwnedByOther apperrors.Code = "AUTH_OAUTH_IDENTITY_OWNED_BY_OTHER"
)

// NewEmailExists returns a Conflict error for a duplicate email address.
func NewEmailExists() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeEmailExists,
		Message:    "an account with this email already exists",
		HTTPStatus: http.StatusConflict,
	}
}

// NewUsernameExists returns a Conflict error for a duplicate username.
func NewUsernameExists() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeUsernameExists,
		Message:    "an account with this username already exists",
		HTTPStatus: http.StatusConflict,
	}
}

// NewPasswordTooWeak returns a Bad Request error when a password fails BR-11 complexity rules.
// details carries a human-readable description of which rule failed.
func NewPasswordTooWeak(details string) *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodePasswordTooWeak,
		Message:    "password does not meet complexity requirements",
		HTTPStatus: http.StatusBadRequest,
		Details:    details,
	}
}

// NewInvalidCredentials returns a 401 error for an incorrect identifier or password.
// The message is intentionally identical in both cases to prevent user enumeration.
func NewInvalidCredentials() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeInvalidCredentials,
		Message:    "invalid credentials",
		HTTPStatus: http.StatusUnauthorized,
	}
}

// NewEmailNotVerified returns a 403 error when login is attempted on an unverified account.
func NewEmailNotVerified() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeEmailNotVerified,
		Message:    "email address has not been verified",
		HTTPStatus: http.StatusForbidden,
	}
}

// NewAccountSuspended returns a 403 error when login is attempted on a suspended account.
func NewAccountSuspended() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeAccountSuspended,
		Message:    "account has been suspended",
		HTTPStatus: http.StatusForbidden,
	}
}

// NewAccountDeleted returns a 403 error when login is attempted on a deleted account.
func NewAccountDeleted() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeAccountDeleted,
		Message:    "account no longer exists",
		HTTPStatus: http.StatusForbidden,
	}
}

// NewDeviceLimitReached returns a 403 error when the per-user device limit is exceeded.
func NewDeviceLimitReached() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeDeviceLimitReached,
		Message:    "device limit reached; revoke an existing device before logging in from a new one",
		HTTPStatus: http.StatusForbidden,
	}
}

// NewTokenInvalid returns a 401 error when the refresh token is unrecognized.
func NewTokenInvalid() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeTokenInvalid,
		Message:    "refresh token is invalid",
		HTTPStatus: http.StatusUnauthorized,
	}
}

// NewTokenExpired returns a 401 error when the refresh token is past its expiry.
func NewTokenExpired() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeTokenExpired,
		Message:    "refresh token has expired",
		HTTPStatus: http.StatusUnauthorized,
	}
}

// NewTokenRevoked returns a 401 error when the refresh token has been explicitly revoked.
func NewTokenRevoked() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeTokenRevoked,
		Message:    "refresh token has been revoked",
		HTTPStatus: http.StatusUnauthorized,
	}
}

// NewTokenConsumed returns a 401 error when a previously consumed token is reused (replay).
func NewTokenConsumed() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeTokenConsumed,
		Message:    "refresh token has already been used",
		HTTPStatus: http.StatusUnauthorized,
	}
}

// NewSessionExpired returns a 401 error when the session has exceeded its absolute timeout.
func NewSessionExpired() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeSessionExpired,
		Message:    "session has expired",
		HTTPStatus: http.StatusUnauthorized,
	}
}

// NewSessionRevoked returns a 401 error when the session has been explicitly revoked.
func NewSessionRevoked() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeSessionRevoked,
		Message:    "session has been revoked",
		HTTPStatus: http.StatusUnauthorized,
	}
}

// NewVerificationTokenInvalid returns a 400 when the submitted token does not
// match any active (unconsumed, unexpired) record.
func NewVerificationTokenInvalid() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeVerificationTokenInvalid,
		Message:    "verification token is invalid or has expired",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewVerificationTokenConsumed returns a 400 when a previously used token is
// submitted again (concurrent-submission race).
func NewVerificationTokenConsumed() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeVerificationTokenConsumed,
		Message:    "verification token has already been used",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewEmailAlreadyVerified returns a 409 when the account's email is already verified.
func NewEmailAlreadyVerified() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeEmailAlreadyVerified,
		Message:    "email address is already verified",
		HTTPStatus: http.StatusConflict,
	}
}

// NewResetTokenInvalid returns a 400 when the submitted reset token does not
// match any active (unconsumed, unexpired) record.
func NewResetTokenInvalid() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeResetTokenInvalid,
		Message:    "password reset token is invalid or has expired",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewResetTokenConsumed returns a 400 when a previously used reset token is
// submitted again (concurrent-submission race).
func NewResetTokenConsumed() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeResetTokenConsumed,
		Message:    "password reset token has already been used",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewOAuthStateInvalid returns a 400 when the callback state does not match
// the state generated at flow initiation (CSRF protection failure).
func NewOAuthStateInvalid() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeOAuthStateInvalid,
		Message:    "invalid or missing OAuth state",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewOAuthProviderError returns a 502 when the provider returns an error
// or an unexpected response during code exchange.
func NewOAuthProviderError(cause error) *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeOAuthProviderError,
		Message:    "OAuth provider returned an error",
		HTTPStatus: http.StatusBadGateway,
		Cause:      cause,
	}
}

// NewOAuthIdentityInvalid returns a 422 when the provider identity is missing
// required fields (e.g. ProviderUserID is empty).
func NewOAuthIdentityInvalid(details string) *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeOAuthIdentityInvalid,
		Message:    "provider identity is invalid",
		HTTPStatus: http.StatusUnprocessableEntity,
		Details:    details,
	}
}

// NewOAuthEmailConflict returns a 409 when an account with the provider's email
// already exists but is not linked to this OAuth identity. The user must log in
// with their existing credentials to link the provider.
func NewOAuthEmailConflict() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeOAuthEmailConflict,
		Message:    "an account with this email already exists; please log in with your password to link your Google account",
		HTTPStatus: http.StatusConflict,
	}
}

// NewOAuthIdentityOwnedByOther returns a 409 when an OAuth identity (provider+sub)
// is already linked to a different application user. The identity cannot be reassigned.
func NewOAuthIdentityOwnedByOther() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeOAuthIdentityOwnedByOther,
		Message:    "this OAuth identity is already linked to another account",
		HTTPStatus: http.StatusConflict,
	}
}
