// Package mapper provides pure transformation functions between domain entities
// and DTOs for the authentication domain. Mappers have no side effects.
package mapper

import (
	"time"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
)

// UserToResponse maps a domain User to a UserResponse DTO.
// PasswordHash and other sensitive fields are never copied.
func UserToResponse(u *domain.User) dto.UserResponse {
	return dto.UserResponse{
		ID:            u.ID.String(),
		Email:         u.Email,
		Username:      u.Username,
		EmailVerified: u.EmailVerified,
		Roles:         []string{},
		CreatedAt:     u.CreatedAt.UTC().Format(time.RFC3339),
	}
}
