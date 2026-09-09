// Package handler exposes the Friends domain over HTTP using Fiber.
package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/api"
	"github.com/AstroWalker24/Streamtogether-backend/internal/friends/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/friends/dto"
	friendsvc "github.com/AstroWalker24/Streamtogether-backend/internal/friends/service"
	"github.com/AstroWalker24/Streamtogether-backend/internal/middleware"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/validator"
)

// Handler handles HTTP requests for the Friends domain.
type Handler struct {
	svc       friendsvc.FriendshipService
	validator *validator.Validator
}

// NewHandler constructs a Handler backed by svc.
func NewHandler(svc friendsvc.FriendshipService) *Handler {
	return &Handler{svc: svc, validator: validator.New()}
}

// EstablishFriendship handles POST /me/friends.
func (h *Handler) EstablishFriendship(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}

	var req dto.EstablishFriendshipRequest
	if err := h.validator.BindAndValidate(c, &req); err != nil {
		return api.Error(c, err)
	}
	targetUserID, err := uuid.Parse(req.UserID)
	if err != nil {
		return api.BadRequest(c, "user_id must be a valid UUID")
	}

	friendship, err := h.svc.EstablishFriendship(c.UserContext(), claims.UserID, targetUserID)
	if err != nil {
		return api.Error(c, err)
	}
	return api.Created(c, toFriendshipResponse(friendship, claims.UserID))
}

// AreFriends handles GET /me/friends/:userId.
func (h *Handler) AreFriends(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}
	targetUserID, err := userIDParam(c)
	if err != nil {
		return api.BadRequest(c, "userId must be a valid UUID")
	}

	areFriends, err := h.svc.AreFriends(c.UserContext(), claims.UserID, targetUserID)
	if err != nil {
		return api.Error(c, err)
	}
	return api.Success(c, dto.FriendshipStatusResponse{AreFriends: areFriends})
}

// GetFriends handles GET /me/friends.
func (h *Handler) GetFriends(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}

	var query dto.FriendsQuery
	if err := h.validator.BindQuery(c, &query); err != nil {
		return api.Error(c, err)
	}

	friends, pageMeta, err := h.svc.GetFriends(c.UserContext(), claims.UserID,
		repo.WithPagination(repo.Pagination{Page: query.Page, PageSize: query.PageSize}),
	)
	if err != nil {
		return api.Error(c, err)
	}

	responses := make([]dto.FriendshipResponse, 0, len(friends))
	for _, friendship := range friends {
		responses = append(responses, toFriendshipResponse(friendship, claims.UserID))
	}
	return api.Success(c, responses, paginationMeta(pageMeta))
}

// RemoveFriendship handles DELETE /me/friends/:userId.
func (h *Handler) RemoveFriendship(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}
	targetUserID, err := userIDParam(c)
	if err != nil {
		return api.BadRequest(c, "userId must be a valid UUID")
	}

	if err := h.svc.RemoveFriendship(c.UserContext(), claims.UserID, targetUserID); err != nil {
		return api.Error(c, err)
	}
	return api.NoContent(c)
}

func userIDParam(c *fiber.Ctx) (uuid.UUID, error) {
	return uuid.Parse(c.Params("userId"))
}

func toFriendshipResponse(friendship *domain.Friendship, userID uuid.UUID) dto.FriendshipResponse {
	friendUserID := friendship.FirstUserID
	if friendUserID == userID {
		friendUserID = friendship.SecondUserID
	}
	return dto.FriendshipResponse{
		ID:        friendship.ID.String(),
		UserID:    friendUserID.String(),
		CreatedAt: friendship.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func paginationMeta(pageMeta repo.PageMeta) *api.Meta {
	return api.NewMeta().WithPagination(&api.Pagination{
		Page:       pageMeta.Page,
		PageSize:   pageMeta.PageSize,
		TotalItems: int(pageMeta.TotalItems),
		TotalPages: pageMeta.TotalPages,
	})
}
