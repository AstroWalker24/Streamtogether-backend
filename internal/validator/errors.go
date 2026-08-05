package validator

import (
    "fmt"
    "strings"

    govalidator "github.com/go-playground/validator/v10"

    apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// FieldError describes a single field-level validation failure.
type FieldError struct {
    Field   string `json:"field"`
    Rule    string `json:"rule"`
    Value   any    `json:"value"`
    Message string `json:"message"`
}

// toFieldErrors converts go-playground ValidationErrors into []FieldError.
func toFieldErrors(errs govalidator.ValidationErrors) []FieldError {
    out := make([]FieldError, 0, len(errs))
    for _, fe := range errs {
        field := toSnakeCase(fe.Field())
        message := fmt.Sprintf(messageFor(fe.Tag()), humanField(fe.Field()))
        out = append(out, FieldError{
            Field:   field,
            Rule:    fe.Tag(),
            Value:   fe.Value(),
            Message: message,
        })
    }
    return out
}

// toAppError converts any validation-related error into an *apperrors.AppError.
// Returns nil if err is nil.
func toAppError(err error) *apperrors.AppError {
    if err == nil {
        return nil
    }
    var ve govalidator.ValidationErrors
    if ok := asValidationErrors(err, &ve); ok {
        return apperrors.NewValidation("Validation failed.", toFieldErrors(ve))
    }
    return apperrors.NewBadRequest(err.Error())
}

// asValidationErrors unwraps err into govalidator.ValidationErrors.
func asValidationErrors(err error, target *govalidator.ValidationErrors) bool {
    ve, ok := err.(govalidator.ValidationErrors)
    if ok {
        *target = ve
    }
    return ok
}

// toSnakeCase converts a Go struct field name to snake_case for JSON output.
func toSnakeCase(s string) string {
    var b strings.Builder
    for i, r := range s {
        if r >= 'A' && r <= 'Z' {
            if i > 0 {
                b.WriteByte('_')
            }
            b.WriteRune(r + 32)
        } else {
            b.WriteRune(r)
        }
    }
    return b.String()
}

// humanField converts a Go field name like "EmailAddress" into "Email Address".
func humanField(s string) string {
    var b strings.Builder
    for i, r := range s {
        if r >= 'A' && r <= 'Z' && i > 0 {
            b.WriteByte(' ')
        }
        b.WriteRune(r)
    }
    return b.String()
}