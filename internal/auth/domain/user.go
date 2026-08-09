// Package domain contains the pure business entities of the authentication domain.
// No external framework, database, or transport dependencies are permitted here.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// UserStatus represents the lifecycle state of a user account.
type UserStatus string

const (
	UserStatusPendingVerification UserStatus = "pending_verification"
	UserStatusActive              UserStatus = "active"
	UserStatusSuspended           UserStatus = "suspended"
	UserStatusDeleted             UserStatus = "deleted"
)

// User is the root aggregate of the authentication domain.
// It represents a verified, persistent human identity on the platform.
type User struct {
	ID            uuid.UUID
	Email         string
	Username      string
	PasswordHash  string
	Status        UserStatus
	EmailVerified bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

// IsActive reports whether the user's account is currently active.
func (u *User) IsActive() bool {
	return u.Status == UserStatusActive
}

// IsDeleted reports whether the user has been soft-deleted.
func (u *User) IsDeleted() bool {
	return u.DeletedAt != nil
}
