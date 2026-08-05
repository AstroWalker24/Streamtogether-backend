package api

import (
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"

	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
)

// envelope is the standard success response body.
type envelope struct {
	Success bool  `json:"success"`
	Data    any   `json:"data,omitempty"`
	Meta    *Meta `json:"meta,omitempty"`
}

// errorBody is the standard error response body.
type errorBody struct {
	Success bool        `json:"success"`
	Error   errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Success writes a 200 JSON response.
func Success(c *fiber.Ctx, data any, meta ...*Meta) error {
	return send(c, http.StatusOK, data, firstMeta(meta))
}

// Created writes a 201 JSON response.
func Created(c *fiber.Ctx, data any) error {
	return send(c, http.StatusCreated, data, nil)
}

// Accepted writes a 202 JSON response.
func Accepted(c *fiber.Ctx, data any) error {
	return send(c, http.StatusAccepted, data, nil)
}

// NoContent writes a 204 response with no body.
func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(http.StatusNoContent)
}

// Error writes an error response derived from an AppError. Internal errors (>=500) are logged.
func Error(c *fiber.Ctx, err error) error {
	appErr := toAppError(err)
	if appErr.HTTPStatus >= http.StatusInternalServerError {
		logger.FromContext(c.UserContext()).Error("internal error",
			logger.String("code", string(appErr.Code)),
			logger.String("message", appErr.Message),
			logger.Err(appErr.Cause),
		)
	}
	return sendError(c, appErr)
}

func BadRequest(c *fiber.Ctx, message string) error {
	return sendError(c, apperrors.NewBadRequest(message))
}

func Unauthorized(c *fiber.Ctx, message string) error {
	return sendError(c, apperrors.NewUnauthorized(message))
}

func Forbidden(c *fiber.Ctx, message string) error {
	return sendError(c, apperrors.NewForbidden(message))
}

func NotFound(c *fiber.Ctx, resource string) error {
	return sendError(c, apperrors.NewNotFound(resource))
}

func Conflict(c *fiber.Ctx, message string) error {
	return sendError(c, apperrors.NewConflict(message))
}

func ValidationError(c *fiber.Ctx, details any) error {
	return sendError(c, apperrors.NewValidation("validation failed", details))
}

func InternalServerError(c *fiber.Ctx, err error) error {
	appErr := apperrors.NewInternal("internal server error", err)
	logger.FromContext(c.UserContext()).Error("internal server error", logger.Err(err))
	return sendError(c, appErr)
}

func send(c *fiber.Ctx, status int, data any, meta *Meta) error {
	if meta != nil && meta.Timestamp.IsZero() {
		meta.Timestamp = time.Now().UTC()
	}
	return c.Status(status).JSON(envelope{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

func sendError(c *fiber.Ctx, appErr *apperrors.AppError) error {
	return c.Status(appErr.HTTPStatus).JSON(errorBody{
		Success: false,
		Error: errorDetail{
			Code:    string(appErr.Code),
			Message: appErr.Message,
			Details: appErr.Details,
		},
	})
}

func toAppError(err error) *apperrors.AppError {
	if err == nil {
		return apperrors.NewInternal("internal server error")
	}
	if appErr, ok := err.(*apperrors.AppError); ok {
		return appErr
	}
	return apperrors.NewInternal("internal server error", err)
}

func firstMeta(m []*Meta) *Meta {
	if len(m) > 0 {
		return m[0]
	}
	return nil
}
