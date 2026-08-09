package domain

import (
	"time"

	"github.com/google/uuid"
)

// Role is a named, stable collection of permissions assigned to users.
// System roles (IsSystem = true) are seeded at deployment and must never
// be deleted by the application.
type Role struct {
	ID          uuid.UUID
	Name        string
	Label       string
	Description string
	IsSystem    bool
	CreatedAt   time.Time
}

// UserRole represents a single row in the user_roles join table.
// AssignedBy is nil when the role was granted by the system (e.g. at registration).
type UserRole struct {
	UserID     uuid.UUID
	RoleID     uuid.UUID
	AssignedAt time.Time
	AssignedBy *uuid.UUID
}
