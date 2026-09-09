// Package handler exposes the Friend Requests domain over HTTP using Fiber.
package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/api"
	"github.com/AstroWalker24/Streamtogether-backend/internal/friends/requests/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/friends/requests/dto"
	requestsvc "github.com/AstroWalker24/Streamtogether-backend/internal/friends/requests/service"
	"github.com/AstroWalker24/Streamtogether-backend/internal/middleware"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/validator"
)

// Handler handles HTTP requests for the Friend Requests domain.
type Handler struct {
	svc       requestsvc.FriendRequestService
	validator *validator.Validator
}

// NewHandler constructs a Handler backed by svc.
func NewHandler(svc requestsvc.FriendRequestService) *Handler {
	return &Handler{svc: svc, validator: validator.New()}
}

func (h *Handler) SendFriendRequest(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}
	var req dto.SendFriendRequest
	if err := h.validator.BindAndValidate(c, &req); err != nil {
		return api.Error(c, err)
	}
	recipientUserID, err := uuid.Parse(req.UserID)
	if err != nil {
		return api.BadRequest(c, "user_id must be a valid UUID")
	}
	request, err := h.svc.SendFriendRequest(c.UserContext(), claims.UserID, recipientUserID)
	if err != nil {
		return api.Error(c, err)
	}
	return api.Created(c, toFriendRequestResponse(request))
}

func (h *Handler) GetIncomingRequests(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}
	requests, pageMeta, err := h.listQuery(c, claims.UserID, true)
	if err != nil {
		return api.Error(c, err)
	}
	return api.Success(c, toFriendRequestResponses(requests), paginationMeta(pageMeta))
}

func (h *Handler) GetOutgoingRequests(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}
	requests, pageMeta, err := h.listQuery(c, claims.UserID, false)
	if err != nil {
		return api.Error(c, err)
	}
	return api.Success(c, toFriendRequestResponses(requests), paginationMeta(pageMeta))
}

func (h *Handler) listQuery(c *fiber.Ctx, userID uuid.UUID, incoming bool) ([]*domain.FriendRequest, repo.PageMeta, error) {
	var query dto.FriendRequestsQuery
	if err := h.validator.BindQuery(c, &query); err != nil {
		return nil, repo.PageMeta{}, err
	}
	options := repo.WithPagination(repo.Pagination{Page: query.Page, PageSize: query.PageSize})
	if incoming {
		return h.svc.GetIncomingRequests(c.UserContext(), userID, options)
	}
	return h.svc.GetOutgoingRequests(c.UserContext(), userID, options)
}

func (h *Handler) GetFriendRequestByID(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}
	requestID, err := requestIDParam(c)
	if err != nil {
		return api.BadRequest(c, "requestId must be a valid UUID")
	}
	request, err := h.svc.GetFriendRequestByID(c.UserContext(), claims.UserID, requestID)
	if err != nil {
		return api.Error(c, err)
	}
	return api.Success(c, toFriendRequestResponse(request))
}

func (h *Handler) AcceptFriendRequest(c *fiber.Ctx) error {
	return h.transition(c, func(userID, requestID uuid.UUID) (*domain.FriendRequest, error) {
		return h.svc.AcceptFriendRequest(c.UserContext(), userID, requestID)
	})
}

func (h *Handler) RejectFriendRequest(c *fiber.Ctx) error {
	return h.transition(c, func(userID, requestID uuid.UUID) (*domain.FriendRequest, error) {
		return h.svc.RejectFriendRequest(c.UserContext(), userID, requestID)
	})
}

func (h *Handler) transition(c *fiber.Ctx, operation func(uuid.UUID, uuid.UUID) (*domain.FriendRequest, error)) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}
	requestID, err := requestIDParam(c)
	if err != nil {
		return api.BadRequest(c, "requestId must be a valid UUID")
	}
	request, err := operation(claims.UserID, requestID)
	if err != nil {
		return api.Error(c, err)
	}
	return api.Success(c, toFriendRequestResponse(request))
}

func (h *Handler) CancelFriendRequest(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}
	requestID, err := requestIDParam(c)
	if err != nil {
		return api.BadRequest(c, "requestId must be a valid UUID")
	}
	if _, err := h.svc.CancelFriendRequest(c.UserContext(), claims.UserID, requestID); err != nil {
		return api.Error(c, err)
	}
	return api.NoContent(c)
}

func requestIDParam(c *fiber.Ctx) (uuid.UUID, error) {
	return uuid.Parse(c.Params("requestId"))
}

func toFriendRequestResponse(request *domain.FriendRequest) dto.FriendRequestResponse {
	response := dto.FriendRequestResponse{
		ID:              request.ID.String(),
		RequesterUserID: request.RequesterUserID.String(),
		RecipientUserID: request.RecipientUserID.String(),
		Status:          string(request.Status),
		CreatedAt:       request.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       request.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if request.RespondedAt != nil {
		respondedAt := request.RespondedAt.UTC().Format(time.RFC3339)
		response.RespondedAt = &respondedAt
	}
	return response
}

func toFriendRequestResponses(requests []*domain.FriendRequest) []dto.FriendRequestResponse {
	responses := make([]dto.FriendRequestResponse, 0, len(requests))
	for _, request := range requests {
		responses = append(responses, toFriendRequestResponse(request))
	}
	return responses
}

func paginationMeta(pageMeta repo.PageMeta) *api.Meta {
	return api.NewMeta().WithPagination(&api.Pagination{
		Page:       pageMeta.Page,
		PageSize:   pageMeta.PageSize,
		TotalItems: int(pageMeta.TotalItems),
		TotalPages: pageMeta.TotalPages,
	})
}
