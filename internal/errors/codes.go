package errors

// Code is a machine-readable error code.
type Code string

const (
    CodeInternalServerError Code = "INTERNAL_SERVER_ERROR"
    CodeValidationError     Code = "VALIDATION_ERROR"
    CodeBadRequest          Code = "BAD_REQUEST"
    CodeUnauthorized        Code = "UNAUTHORIZED"
    CodeForbidden           Code = "FORBIDDEN"
    CodeNotFound            Code = "NOT_FOUND"
    CodeConflict            Code = "CONFLICT"
    CodeDatabaseError       Code = "DATABASE_ERROR"
    CodeRedisError          Code = "REDIS_ERROR"
    CodeTimeout             Code = "TIMEOUT"
    CodeUnknownError        Code = "UNKNOWN_ERROR"
)