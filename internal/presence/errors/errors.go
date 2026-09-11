// Package errors defines Presence-domain error codes and constructors that
// extend the global AppError type with PRESENCE_* codes.
package errors

import (
	"net/http"

	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

const (
	CodePresenceNotFound     apperrors.Code = "PRESENCE_NOT_FOUND"
	CodeInvalidConnectionID  apperrors.Code = "PRESENCE_INVALID_CONNECTION_ID"
	CodeInvalidUserID        apperrors.Code = "PRESENCE_INVALID_USER_ID"
	CodeInvalidPresenceState apperrors.Code = "PRESENCE_INVALID_STATE"
	CodeConnectionNotFound   apperrors.Code = "PRESENCE_CONNECTION_NOT_FOUND"
)

// NewPresenceNotFound returns a 404 error when no presence record exists.
func NewPresenceNotFound() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodePresenceNotFound,
		Message:    "presence record not found",
		HTTPStatus: http.StatusNotFound,
	}
}

// NewInvalidConnectionID returns a 400 error for a nil or malformed connection UUID.
func NewInvalidConnectionID() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeInvalidConnectionID,
		Message:    "connection ID must not be the nil UUID",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewInvalidUserID returns a 400 error for a nil or malformed user UUID.
func NewInvalidUserID() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeInvalidUserID,
		Message:    "user ID must not be the nil UUID",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewInvalidPresenceState returns a 400 error when an unrecognized presence state is supplied.
func NewInvalidPresenceState(state string) *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeInvalidPresenceState,
		Message:    "invalid presence state: " + state,
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewConnectionNotFound returns a 404 error when a referenced presence connection does not exist.
func NewConnectionNotFound() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeConnectionNotFound,
		Message:    "presence connection not found",
		HTTPStatus: http.StatusNotFound,
	}
}
