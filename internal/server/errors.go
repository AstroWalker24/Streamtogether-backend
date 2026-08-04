package server

import (
    "errors"

    "github.com/AstroWalker24/Streamtogether-backend/internal/logger"
    "github.com/gofiber/fiber/v2"
)

type errorResponse struct {
    Success bool         `json:"success"`
    Error   errorPayload `json:"error"`
}

type errorPayload struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

// newErrorHandler returns a Fiber ErrorHandler that logs unexpected errors
// and always responds with a consistent JSON shape.
func newErrorHandler(log logger.Logger) fiber.ErrorHandler {
    return func(c *fiber.Ctx, err error) error {
        code := fiber.StatusInternalServerError
        errCode := "INTERNAL_ERROR"
        message := "an unexpected error occurred"

        var fe *fiber.Error
        if errors.As(err, &fe) {
            code = fe.Code
            message = fe.Message
            errCode = statusToCode(fe.Code)
        } else {
            log.Error("unhandled server error", logger.Err(err))
        }

        return c.Status(code).JSON(errorResponse{
            Success: false,
            Error: errorPayload{
                Code:    errCode,
                Message: message,
            },
        })
    }
}

func statusToCode(status int) string {
    switch status {
    case fiber.StatusBadRequest:
        return "BAD_REQUEST"
    case fiber.StatusUnauthorized:
        return "UNAUTHORIZED"
    case fiber.StatusForbidden:
        return "FORBIDDEN"
    case fiber.StatusNotFound:
        return "NOT_FOUND"
    case fiber.StatusMethodNotAllowed:
        return "METHOD_NOT_ALLOWED"
    case fiber.StatusConflict:
        return "CONFLICT"
    case fiber.StatusUnprocessableEntity:
        return "UNPROCESSABLE_ENTITY"
    case fiber.StatusTooManyRequests:
        return "TOO_MANY_REQUESTS"
    default:
        return "INTERNAL_ERROR"
    }
}