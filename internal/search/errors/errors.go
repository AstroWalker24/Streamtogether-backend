// Package errors defines Search-domain error codes and constructors that
// extend the global AppError type with SEARCH_* codes.
package errors

import (
	"net/http"

	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

const (
	CodeInvalidActorID apperrors.Code = "SEARCH_INVALID_ACTOR_ID"
	CodeQueryTooShort  apperrors.Code = "SEARCH_QUERY_TOO_SHORT"
	CodeQueryTooLong   apperrors.Code = "SEARCH_QUERY_TOO_LONG"
)

// NewInvalidActorID returns a Bad Request error for a nil actor UUID.
func NewInvalidActorID() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeInvalidActorID,
		Message:    "actor user ID must not be the nil UUID",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewQueryTooShort returns a Bad Request error for an empty or undersized query.
func NewQueryTooShort() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeQueryTooShort,
		Message:    "search query must be at least 2 characters",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewQueryTooLong returns a Bad Request error for an oversized query.
func NewQueryTooLong() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeQueryTooLong,
		Message:    "search query must not exceed 100 characters",
		HTTPStatus: http.StatusBadRequest,
	}
}
