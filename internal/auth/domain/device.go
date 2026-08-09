package domain

import (
    "time"

    "github.com/google/uuid"
)

// DevicePlatform represents the client platform type of a registered device.
type DevicePlatform string

const (
    DevicePlatformWeb     DevicePlatform = "web"
    DevicePlatformIOS     DevicePlatform = "ios"
    DevicePlatformAndroid DevicePlatform = "android"
    DevicePlatformDesktop DevicePlatform = "desktop"
)

// Device is a persistent record of an endpoint from which a user has authenticated.
// Devices use Revoked rather than a deleted_at column for termination.
type Device struct {
    ID              uuid.UUID
    UserID          uuid.UUID
    FingerprintHash string
    FriendlyName    string
    Platform        DevicePlatform
    Browser         string
    OS              string
    LastIPAddress   string // stored as INET; held as text in the domain layer
    FirstSeenAt     time.Time
    LastActiveAt    time.Time
    Trusted         bool
    Revoked         bool
    RevokedAt       *time.Time
}

// IsRevoked reports whether the device has been revoked.
func (d *Device) IsRevoked() bool {
    return d.Revoked
}