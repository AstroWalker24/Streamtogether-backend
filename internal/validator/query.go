package validator

import (
    "github.com/gofiber/fiber/v2"

    apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// BindQuery parses URL query parameters into dst using Fiber's QueryParser,
// then validates the struct.
func (val *Validator) BindQuery(c *fiber.Ctx, dst any) *apperrors.AppError {
    if err := c.QueryParser(dst); err != nil {
        return apperrors.NewBadRequest("Query parameters are invalid.")
    }
    return val.Validate(dst)
}