package validator

import (
    "github.com/gofiber/fiber/v2"

    apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// BindParams parses Fiber route parameters into dst using ParamsParser,
// then validates the struct.
func (val *Validator) BindParams(c *fiber.Ctx, dst any) *apperrors.AppError {
    if err := c.ParamsParser(dst); err != nil {
        return apperrors.NewBadRequest("Path parameters are invalid.")
    }
    return val.Validate(dst)
}