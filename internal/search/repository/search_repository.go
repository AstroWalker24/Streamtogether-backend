// Package repository provides the PostgreSQL-backed query implementation for
// the Search domain. All SQL is encapsulated here; callers interact only with
// the SearchRepository interface.
package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/search/domain"
)

// SearchRepository defines read-only user-discovery queries.
type SearchRepository interface {
	// Search returns discoverable users matching normalizedQuery, excluding the
	// authenticated actor. Pagination is supplied through repo.WithPagination.
	Search(ctx context.Context, actorUserID uuid.UUID, normalizedQuery string, opts ...repo.Option) ([]*domain.SearchResult, repo.PageMeta, error)
}

// --- implementation ----------------------------------------------------------

type searchRepository struct {
	repo.Base
}

// NewSearchRepository constructs a SearchRepository backed by db.
func NewSearchRepository(db *database.Database, log logger.Logger) SearchRepository {
	return &searchRepository{Base: repo.NewBase(db, log)}
}

// --- query fragments ---------------------------------------------------------

const searchRankedMatchesCTE = `
WITH base AS NOT MATERIALIZED (
    SELECT
        u.id AS user_id,
        u.username,
        p.display_name,
        p.avatar_url,
        lower(u.username) AS username_search,
        lower(p.display_name) AS display_name_search
    FROM users u
    JOIN profiles p ON p.user_id = u.id
    WHERE u.status = 'active'
      AND u.deleted_at IS NULL
      AND u.id <> $1
), candidates AS (
    SELECT *, 1 AS rank_tier
    FROM base
    WHERE username_search = $2

    UNION ALL

    SELECT *, 2 AS rank_tier
    FROM base
    WHERE display_name_search = $2

    UNION ALL

    SELECT *, 3 AS rank_tier
    FROM base
    WHERE username_search LIKE $3
      AND username_search <> $2

    UNION ALL

    SELECT *, 4 AS rank_tier
    FROM base
    WHERE display_name_search LIKE $3
      AND display_name_search <> $2

    UNION ALL

    SELECT *, 5 AS rank_tier
    FROM base
    WHERE username_search LIKE $4
      AND username_search NOT LIKE $3

    UNION ALL

    SELECT *, 6 AS rank_tier
    FROM base
    WHERE display_name_search LIKE $4
      AND display_name_search NOT LIKE $3
), best_match AS (
    SELECT DISTINCT ON (user_id)
        user_id,
        username,
        display_name,
        avatar_url,
        rank_tier
    FROM candidates
    ORDER BY user_id, rank_tier ASC
)`

const sqlCountSearchResults = searchRankedMatchesCTE + `
SELECT COUNT(*)
FROM   best_match`

const sqlSearchResults = searchRankedMatchesCTE + `
SELECT user_id, username, display_name, avatar_url
FROM   best_match
ORDER  BY rank_tier ASC, username ASC, user_id ASC
LIMIT  $5 OFFSET $6`

// --- scan helpers ------------------------------------------------------------

func scanSearchResultRows(rows pgx.Rows, capacity int) ([]*domain.SearchResult, error) {
	results := make([]*domain.SearchResult, 0, capacity)
	for rows.Next() {
		var result domain.SearchResult
		if err := rows.Scan(
			&result.UserID,
			&result.Username,
			&result.DisplayName,
			&result.AvatarURL,
		); err != nil {
			return nil, repo.MapError(err)
		}
		results = append(results, &result)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.MapError(err)
	}
	return results, nil
}

// --- read operations ---------------------------------------------------------

func (r *searchRepository) Search(ctx context.Context, actorUserID uuid.UUID, normalizedQuery string, opts ...repo.Option) ([]*domain.SearchResult, repo.PageMeta, error) {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)

	p := repo.Pagination{Page: repo.DefaultPage, PageSize: repo.DefaultPageSize}
	if op := o.Pagination(); op != nil {
		p = *op
		p.Normalize()
	}

	prefixPattern := normalizedQuery + "%"
	containsPattern := "%" + normalizedQuery + "%"

	var total int64
	if err := db.QueryRow(ctx, sqlCountSearchResults, actorUserID, normalizedQuery, prefixPattern, containsPattern).Scan(&total); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	rows, err := db.Query(ctx, sqlSearchResults, actorUserID, normalizedQuery, prefixPattern, containsPattern, p.Limit(), p.Offset())
	if err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}
	defer rows.Close()

	results, err := scanSearchResultRows(rows, p.PageSize)
	if err != nil {
		return nil, repo.PageMeta{}, err
	}

	return results, repo.NewPageMeta(p, total), nil
}
