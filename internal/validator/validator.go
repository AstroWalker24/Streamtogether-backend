// Package validator provides a unified request validation layer for Fiber handlers.
// It wraps go-playground/validator and exposes a clean API for JSON, query,
// path parameter, and header binding with automatic error translation.
package validator

import (
	"reflect"

	govalidator "github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"

	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// Validator is the application-level validation instance.
// It is safe for concurrent use.
type Validator struct {
	v *govalidator.Validate
}

// New creates a new Validator with default settings.
func New() *Validator {
	v := govalidator.New()
	// Use the JSON field name in validation error output instead of the Go field name.
	v.RegisterTagNameFunc(jsonTagName)
	return &Validator{v: v}
}

// Validate validates a struct against its `validate` tags.
// Returns an *apperrors.AppError on failure, nil on success.
func (val *Validator) Validate(s any) *apperrors.AppError {
	if err := val.v.Struct(s); err != nil {
		return toAppError(err)
	}
	return nil
}

// BindAndValidate binds the JSON body into dst, normalises it (if dst implements
// Normalizer), then validates the struct. This is the primary handler entry point.
func (val *Validator) BindAndValidate(c *fiber.Ctx, dst any) *apperrors.AppError {
	if err := val.BindJSON(c, dst); err != nil {
		return err
	}
	if n, ok := dst.(Normalizer); ok {
		n.Normalize()
	}
	return val.Validate(dst)
}

// RegisterCustom registers a custom validation function under the given tag name.
func (val *Validator) RegisterCustom(tag string, fn govalidator.Func) error {
	return val.v.RegisterValidation(tag, fn)
}

// jsonTagName extracts the first JSON tag segment so validation errors
// report the JSON field name rather than the Go struct field name.
func jsonTagName(fld reflect.StructField) string {
	name := fld.Tag.Get("json")
	if name == "" || name == "-" {
		return fld.Name
	}
	for i, c := range name {
		if c == ',' {
			if i == 0 {
				return fld.Name
			}
			return name[:i]
		}
	}
	return name
}
