// Package domain contains the pure business entities and value objects of the
// Presence domain. No external framework, database, or transport dependencies
// are permitted here.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// PresenceState represents the availability state of a user or connection.
type PresenceState string

const (
	PresenceStateOnline  PresenceState = "online"
	PresenceStateAway    PresenceState = "away"
	PresenceStateOffline PresenceState = "offline"
)

// IsValid reports whether the presence state is one of the recognized states.
func (s PresenceState) IsValid() bool {
	switch s {
	case PresenceStateOnline, PresenceStateAway, PresenceStateOffline:
		return true
	default:
		return false
	}
}

// PresencePreference represents an explicit user-selected override state.
type PresencePreference string

const (
	PresencePreferenceNone          PresencePreference = "none"
	PresencePreferenceAway          PresencePreference = "away"
	PresencePreferenceAppearOffline PresencePreference = "appear_offline"
)

// UserPresence represents the aggregate availability state of a user.
// It reflects both ephemeral active connections and durable last-seen history.
type UserPresence struct {
	UserID            uuid.UUID
	Status            PresenceState
	LastSeenAt        *time.Time
	UpdatedAt         time.Time
	ManualPreference  PresencePreference
	ActiveConnections int
}

// DurablePresence represents the persistent presence record stored in PostgreSQL.
type DurablePresence struct {
	UserID     uuid.UUID
	LastSeenAt *time.Time
	UpdatedAt  time.Time
}

// PresenceConnection represents an active real-time connection instance.
type PresenceConnection struct {
	ConnectionID    uuid.UUID
	UserID          uuid.UUID
	SessionID       uuid.UUID
	DeviceID        uuid.UUID
	NodeID          string
	Platform        string
	State           PresenceState
	ConnectedAt     time.Time
	LastHeartbeatAt time.Time
}
