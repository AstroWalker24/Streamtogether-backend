// Package handler exposes the Search domain over HTTP using Fiber.
package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/AstroWalker24/Streamtogether-backend/internal/api"
	"github.com/AstroWalker24/Streamtogether-backend/internal/middleware"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/search/api/dto"
	"github.com/AstroWalker24/Streamtogether-backend/internal/search/domain"
	searchsvc "github.com/AstroWalker24/Streamtogether-backend/internal/search/service"
	"github.com/AstroWalker24/Streamtogether-backend/internal/validator"
)

// Handler handles HTTP requests for the Search domain.
type Handler struct {
	svc       searchsvc.SearchService
	validator *validator.Validator
}

// NewHandler constructs a Handler backed by svc.
func NewHandler(svc searchsvc.SearchService) *Handler {
	return &Handler{svc: svc, validator: validator.New()}
}

// Search handles GET /me/search/users.
func (h *Handler) Search(c *fiber.Ctx) error {
	claims, ok := middleware.ClaimsFrom(c)
	if !ok {
		return api.Unauthorized(c, "authentication required")
	}

	var query dto.SearchQuery
	if err := h.validator.BindQuery(c, &query); err != nil {
		return api.Error(c, err)
	}

	results, pageMeta, err := h.svc.Search(
		c.UserContext(),
		claims.UserID,
		query.Query,
		repo.WithPagination(repo.Pagination{Page: query.Page, PageSize: query.PageSize}),
	)
	if err != nil {
		return api.Error(c, err)
	}

	responses := make([]dto.SearchUserResponse, 0, len(results))
	for _, result := range results {
		responses = append(responses, toSearchUserResponse(result))
	}
	return api.Success(c, responses, paginationMeta(pageMeta))
}

func toSearchUserResponse(result *domain.SearchResult) dto.SearchUserResponse {
	return dto.SearchUserResponse{
		UserID:      result.UserID.String(),
		Username:    result.Username,
		DisplayName: result.DisplayName,
		AvatarURL:   result.AvatarURL,
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
