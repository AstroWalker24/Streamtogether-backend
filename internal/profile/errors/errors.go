// Package errors defines profile-domain error codes and constructors
// that extend the global AppError type with PROFILE_* codes.
package errors

import (
	"net/http"

	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// Profile-domain error codes defined by the domain contracts.
const (
	CodeProfileNotFound           apperrors.Code = "PROFILE_NOT_FOUND"
	CodeProfileAlreadyExists      apperrors.Code = "PROFILE_ALREADY_EXISTS"
	CodeProfileUpdateForbidden    apperrors.Code = "PROFILE_UPDATE_FORBIDDEN"
	CodeProfileInvalidDisplayName apperrors.Code = "PROFILE_INVALID_DISPLAY_NAME"
	CodeProfileInvalidBio         apperrors.Code = "PROFILE_INVALID_BIO"
	CodeProfileInvalidAvatarURL   apperrors.Code = "PROFILE_INVALID_AVATAR_URL"
)

// NewProfileNotFound returns a 404 error when no profile exists for the given user.
func NewProfileNotFound() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeProfileNotFound,
		Message:    "profile not found",
		HTTPStatus: http.StatusNotFound,
	}
}

// NewProfileAlreadyExists returns a 409 error when a second profile is created for the same user.
func NewProfileAlreadyExists() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeProfileAlreadyExists,
		Message:    "a profile already exists for this user",
		HTTPStatus: http.StatusConflict,
	}
}

// NewProfileUpdateForbidden returns a 403 error when a user attempts to modify another user's profile.
func NewProfileUpdateForbidden() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeProfileUpdateForbidden,
		Message:    "you are not allowed to modify this profile",
		HTTPStatus: http.StatusForbidden,
	}
}

// NewProfileInvalidDisplayName returns a 400 error when display_name fails validation.
func NewProfileInvalidDisplayName(details string) *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeProfileInvalidDisplayName,
		Message:    "display name is invalid",
		HTTPStatus: http.StatusBadRequest,
		Details:    details,
	}
}

// NewProfileInvalidBio returns a 400 error when bio fails validation.
func NewProfileInvalidBio(details string) *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeProfileInvalidBio,
		Message:    "bio is invalid",
		HTTPStatus: http.StatusBadRequest,
		Details:    details,
	}
}

// NewProfileInvalidAvatarURL returns a 400 error when avatar_url fails validation.
func NewProfileInvalidAvatarURL(details string) *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeProfileInvalidAvatarURL,
		Message:    "avatar URL is invalid",
		HTTPStatus: http.StatusBadRequest,
		Details:    details,
	}
}
