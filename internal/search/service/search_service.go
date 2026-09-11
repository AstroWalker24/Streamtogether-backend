// Package service contains the business operation layer for the Search domain.
package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/search/domain"
	searcherrors "github.com/AstroWalker24/Streamtogether-backend/internal/search/errors"
	searchrepo "github.com/AstroWalker24/Streamtogether-backend/internal/search/repository"
)

const (
	minQueryLength = 2
	maxQueryLength = 100
)

// SearchService defines user-discovery operations.
type SearchService interface {
	// Search validates and normalizes rawQuery, then returns matching
	// discoverable users. Pagination is supplied through repo.WithPagination.
	Search(ctx context.Context, actorUserID uuid.UUID, rawQuery string, opts ...repo.Option) ([]*domain.SearchResult, repo.PageMeta, error)
}

type searchService struct {
	search searchrepo.SearchRepository
}

// NewSearchService constructs a SearchService backed by search.
func NewSearchService(search searchrepo.SearchRepository) SearchService {
	return &searchService{search: search}
}

// Search validates caller input and delegates matching, account eligibility,
// self-exclusion, ranking, and pagination execution to the repository.
func (s *searchService) Search(ctx context.Context, actorUserID uuid.UUID, rawQuery string, opts ...repo.Option) ([]*domain.SearchResult, repo.PageMeta, error) {
	if actorUserID == uuid.Nil {
		return nil, repo.PageMeta{}, searcherrors.NewInvalidActorID()
	}

	normalizedQuery, err := normalizeQuery(rawQuery)
	if err != nil {
		return nil, repo.PageMeta{}, err
	}

	results, pageMeta, err := s.search.Search(ctx, actorUserID, normalizedQuery, opts...)
	if err != nil {
		return nil, repo.PageMeta{}, apperrors.NewDatabase(err)
	}
	return results, pageMeta, nil
}

func normalizeQuery(rawQuery string) (string, error) {
	normalizedQuery := strings.ToLower(strings.Join(strings.Fields(rawQuery), " "))
	queryLength := utf8.RuneCountInString(normalizedQuery)
	if queryLength < minQueryLength {
		return "", searcherrors.NewQueryTooShort()
	}
	if queryLength > maxQueryLength {
		return "", searcherrors.NewQueryTooLong()
	}
	return normalizedQuery, nil
}
