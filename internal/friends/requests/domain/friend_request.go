// Package domain contains the pure business entities of the Friend Requests domain.
// No external framework, database, or transport dependencies are permitted here.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// FriendRequestStatus represents the lifecycle state of a Friend Request.
type FriendRequestStatus string

const (
	FriendRequestStatusPending   FriendRequestStatus = "pending"
	FriendRequestStatusAccepted  FriendRequestStatus = "accepted"
	FriendRequestStatusRejected  FriendRequestStatus = "rejected"
	FriendRequestStatusCancelled FriendRequestStatus = "cancelled"
)

// FriendRequest is a directed request from one user to another.
type FriendRequest struct {
	ID              uuid.UUID
	RequesterUserID uuid.UUID
	RecipientUserID uuid.UUID
	Status          FriendRequestStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
	RespondedAt     *time.Time
}
