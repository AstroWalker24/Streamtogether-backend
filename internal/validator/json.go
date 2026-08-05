package validator

import (
	"github.com/gofiber/fiber/v2"

	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// BindJSON reads and decodes the request body into dst (no validation).
// Use BindAndValidate when struct validation is also required.
func (val *Validator) BindJSON(c *fiber.Ctx, dst any) *apperrors.AppError {
	if err := c.BodyParser(dst); err != nil {
		return apperrors.NewBadRequest("Request body is invalid or malformed JSON.")
	}
	return nil
}
