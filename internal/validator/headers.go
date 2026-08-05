package validator

import (
    "github.com/gofiber/fiber/v2"

    apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// BindHeaders parses request headers into dst using ReqHeaderParser,
// then validates the struct.
func (val *Validator) BindHeaders(c *fiber.Ctx, dst any) *apperrors.AppError {
    if err := c.ReqHeaderParser(dst); err != nil {
        return apperrors.NewBadRequest("Request headers are invalid.")
    }
    return val.Validate(dst)
}