package mapper

import (
	"time"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
)

// DeviceToResponse maps a domain Device to a DeviceResponse DTO.
func DeviceToResponse(d *domain.Device) dto.DeviceResponse {
	return dto.DeviceResponse{
		ID:            d.ID.String(),
		FriendlyName:  d.FriendlyName,
		Platform:      string(d.Platform),
		Browser:       d.Browser,
		OS:            d.OS,
		LastIPAddress: d.LastIPAddress,
		LastActiveAt:  d.LastActiveAt.UTC().Format(time.RFC3339),
		Trusted:       d.Trusted,
	}
}

// SessionToResponse maps a domain Session and its Device to a SessionResponse DTO.
// Set isCurrent true when this session belongs to the caller making the request.
func SessionToResponse(s *domain.Session, d *domain.Device, isCurrent bool) dto.SessionResponse {
	return dto.SessionResponse{
		ID:           s.ID.String(),
		Device:       DeviceToResponse(d),
		IPAddress:    s.IPAddress,
		CreatedAt:    s.CreatedAt.UTC().Format(time.RFC3339),
		LastActiveAt: s.LastActiveAt.UTC().Format(time.RFC3339),
		ExpiresAt:    s.ExpiresAt.UTC().Format(time.RFC3339),
		RememberMe:   s.RememberMe,
		IsCurrent:    isCurrent,
	}
}
