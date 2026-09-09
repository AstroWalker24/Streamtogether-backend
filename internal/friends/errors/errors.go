// Package errors defines Friends-domain error codes and constructors that
// extend the global AppError type with FRIENDS_* codes.
package errors

import (
	"net/http"

	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

const (
	CodeFriendshipNotFound apperrors.Code = "FRIENDS_FRIENDSHIP_NOT_FOUND"
	CodeAlreadyFriends     apperrors.Code = "FRIENDS_ALREADY_FRIENDS"
	CodeCannotFriendSelf   apperrors.Code = "FRIENDS_CANNOT_FRIEND_SELF"
	CodeInvalidUserID      apperrors.Code = "FRIENDS_INVALID_USER_ID"
	CodeUserNotEligible    apperrors.Code = "FRIENDS_USER_NOT_ELIGIBLE"
)

// NewFriendshipNotFound returns a 404 error when no relationship exists for a pair.
func NewFriendshipNotFound() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeFriendshipNotFound,
		Message:    "friendship not found",
		HTTPStatus: http.StatusNotFound,
	}
}

// NewAlreadyFriends returns a Conflict error when a friendship already exists.
func NewAlreadyFriends() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeAlreadyFriends,
		Message:    "users are already friends",
		HTTPStatus: http.StatusConflict,
	}
}

// NewCannotFriendSelf returns a Bad Request error for a self-friendship attempt.
func NewCannotFriendSelf() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeCannotFriendSelf,
		Message:    "a user cannot be friends with themselves",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewInvalidUserID returns a Bad Request error for a nil user UUID.
func NewInvalidUserID() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeInvalidUserID,
		Message:    "user ID must not be the nil UUID",
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewUserNotEligible returns a Forbidden error when a user is not active.
func NewUserNotEligible() *apperrors.AppError {
	return &apperrors.AppError{
		Code:       CodeUserNotEligible,
		Message:    "user is not eligible to establish a friendship",
		HTTPStatus: http.StatusForbidden,
	}
}
