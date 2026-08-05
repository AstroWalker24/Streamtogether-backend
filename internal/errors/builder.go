package errors

import "net/http"

func NewValidation(message string, details any) *AppError {
	return &AppError{
		Code:       CodeValidationError,
		Message:    message,
		HTTPStatus: http.StatusBadRequest,
		Details:    details,
	}
}

func NewBadRequest(message string) *AppError {
	return &AppError{
		Code:       CodeBadRequest,
		Message:    message,
		HTTPStatus: http.StatusBadRequest,
	}
}

func NewUnauthorized(message string) *AppError {
	return &AppError{
		Code:       CodeUnauthorized,
		Message:    message,
		HTTPStatus: http.StatusUnauthorized,
	}
}

func NewForbidden(message string) *AppError {
	return &AppError{
		Code:       CodeForbidden,
		Message:    message,
		HTTPStatus: http.StatusForbidden,
	}
}

func NewNotFound(resource string) *AppError {
	return &AppError{
		Code:       CodeNotFound,
		Message:    resource + " not found",
		HTTPStatus: http.StatusNotFound,
	}
}

func NewConflict(message string) *AppError {
	return &AppError{
		Code:       CodeConflict,
		Message:    message,
		HTTPStatus: http.StatusConflict,
	}
}

func NewInternal(message string, cause ...error) *AppError {
	var c error
	if len(cause) > 0 {
		c = cause[0]
	}
	return &AppError{
		Code:       CodeInternalServerError,
		Message:    message,
		HTTPStatus: http.StatusInternalServerError,
		Cause:      c,
	}
}

func NewDatabase(cause error) *AppError {
	return &AppError{
		Code:       CodeDatabaseError,
		Message:    "database error",
		HTTPStatus: http.StatusInternalServerError,
		Cause:      cause,
	}
}

func NewRedis(cause error) *AppError {
	return &AppError{
		Code:       CodeRedisError,
		Message:    "redis error",
		HTTPStatus: http.StatusInternalServerError,
		Cause:      cause,
	}
}

func Wrap(err error, message string) *AppError {
	if appErr, ok := err.(*AppError); ok {
		return appErr
	}
	return &AppError{
		Code:       CodeUnknownError,
		Message:    message,
		HTTPStatus: http.StatusInternalServerError,
		Cause:      err,
	}
}
