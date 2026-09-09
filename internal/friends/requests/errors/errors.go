// Package errors defines Friend Requests-domain error codes and constructors
// that extend the global AppError type with FRIEND_REQUESTS_* codes.
package errors

import (
	"net/http"

	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

const (
	CodeRequestNotFound        apperrors.Code = "FRIEND_REQUESTS_REQUEST_NOT_FOUND"
	CodeDuplicatePending       apperrors.Code = "FRIEND_REQUESTS_DUPLICATE_PENDING"
	CodeCannotRequestSelf      apperrors.Code = "FRIEND_REQUESTS_CANNOT_REQUEST_SELF"
	CodeInvalidUserID          apperrors.Code = "FRIEND_REQUESTS_INVALID_USER_ID"
	CodeUserNotEligible        apperrors.Code = "FRIEND_REQUESTS_USER_NOT_ELIGIBLE"
	CodeUnauthorizedTransition apperrors.Code = "FRIEND_REQUESTS_UNAUTHORIZED_TRANSITION"
	CodeRequestNotPending      apperrors.Code = "FRIEND_REQUESTS_REQUEST_NOT_PENDING"
)

func NewRequestNotFound() *apperrors.AppError {
	return &apperrors.AppError{Code: CodeRequestNotFound, Message: "friend request not found", HTTPStatus: http.StatusNotFound}
}

func NewDuplicatePending() *apperrors.AppError {
	return &apperrors.AppError{Code: CodeDuplicatePending, Message: "a pending friend request already exists", HTTPStatus: http.StatusConflict}
}

func NewCannotRequestSelf() *apperrors.AppError {
	return &apperrors.AppError{Code: CodeCannotRequestSelf, Message: "a user cannot send a friend request to themselves", HTTPStatus: http.StatusBadRequest}
}

func NewInvalidUserID() *apperrors.AppError {
	return &apperrors.AppError{Code: CodeInvalidUserID, Message: "user ID must not be the nil UUID", HTTPStatus: http.StatusBadRequest}
}

func NewUserNotEligible() *apperrors.AppError {
	return &apperrors.AppError{Code: CodeUserNotEligible, Message: "user is not eligible for friend request operations", HTTPStatus: http.StatusForbidden}
}

func NewUnauthorizedTransition() *apperrors.AppError {
	return &apperrors.AppError{Code: CodeUnauthorizedTransition, Message: "user is not authorized to transition this friend request", HTTPStatus: http.StatusForbidden}
}

func NewRequestNotPending() *apperrors.AppError {
	return &apperrors.AppError{Code: CodeRequestNotPending, Message: "friend request is not pending", HTTPStatus: http.StatusConflict}
}
