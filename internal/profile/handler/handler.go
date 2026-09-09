// Package handler exposes the profile domain over HTTP using Fiber.
package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/api"
	"github.com/AstroWalker24/Streamtogether-backend/internal/middleware"
	"github.com/AstroWalker24/Streamtogether-backend/internal/profile/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/profile/dto"
	profilesvc "github.com/AstroWalker24/Streamtogether-backend/internal/profile/service"
)

// Handler handles HTTP requests for the profile domain.
type Handler struct {
	svc profilesvc.ProfileService
}

// NewHandler constructs a Handler backed by svc.
func NewHandler(svc profilesvc.ProfileService) *Handler {
	return &Handler{svc: svc}
}

// GetMyProfile handles GET /me/profile.
func (h *Handler) GetMyProfile(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}
	p, err := h.svc.GetMyProfile(c.UserContext(), claims)
	if err != nil {
		return api.Error(c, err)
	}
	return api.Success(c, toProfileResponse(p))
}

// UpdateMyProfile handles PATCH /me/profile.
func (h *Handler) UpdateMyProfile(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}
	var req dto.UpdateProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return api.BadRequest(c, "invalid request body")
	}
	input := profilesvc.UpdateInput{
		DisplayName: req.DisplayName,
		Bio:         req.Bio,
		AvatarURL:   req.AvatarURL,
	}
	p, err := h.svc.UpdateMyProfile(c.UserContext(), claims, input)
	if err != nil {
		return api.Error(c, err)
	}
	return api.Success(c, toProfileResponse(p))
}

// GetUserProfile handles GET /users/:userId/profile.
func (h *Handler) GetUserProfile(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return api.BadRequest(c, "userId must be a valid UUID")
	}
	p, err := h.svc.GetByUserID(c.UserContext(), userID)
	if err != nil {
		return api.Error(c, err)
	}
	return api.Success(c, toProfileResponse(p))
}

// toProfileResponse maps a domain.Profile to its HTTP response DTO.
func toProfileResponse(p *domain.Profile) dto.ProfileResponse {
	return dto.ProfileResponse{
		DisplayName: p.DisplayName,
		Bio:         p.Bio,
		AvatarURL:   p.AvatarURL,
		UpdatedAt:   p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
