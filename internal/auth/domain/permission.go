package domain

import (
	"time"

	"github.com/google/uuid"
)

// Permission is a single, granular capability following the resource:action
// naming convention (e.g. "party:create", "chat:send").
// Permission names are immutable after creation.
type Permission struct {
	ID          uuid.UUID
	Name        string
	Description string
	Category    string
	CreatedAt   time.Time
}

// RolePermission represents a single row in the role_permissions join table.
type RolePermission struct {
	RoleID       uuid.UUID
	PermissionID uuid.UUID
	AssignedAt   time.Time
}
