package repository

const (
	// DefaultPage is used when no page number is specified.
	DefaultPage = 1
	// DefaultPageSize is the number of items per page when none is specified.
	DefaultPageSize = 20
	// MaxPageSize caps the items per page to prevent oversized queries.
	MaxPageSize = 100
)

// Pagination holds page-based query parameters.
// The Cursor field is reserved for future cursor-based pagination.
type Pagination struct {
	Page     int
	PageSize int
	Cursor   string // reserved; not yet active
}

// PageMeta holds computed metadata returned alongside list responses.
type PageMeta struct {
	Page       int
	PageSize   int
	TotalItems int64
	TotalPages int
	HasNext    bool
	HasPrev    bool
}

// Normalize clamps Page and PageSize to valid ranges and applies defaults.
func (p *Pagination) Normalize() {
	if p.Page < 1 {
		p.Page = DefaultPage
	}
	if p.PageSize < 1 {
		p.PageSize = DefaultPageSize
	}
	if p.PageSize > MaxPageSize {
		p.PageSize = MaxPageSize
	}
}

// Offset returns the SQL OFFSET value derived from the current page.
func (p Pagination) Offset() int {
	return (p.Page - 1) * p.PageSize
}

// Limit returns the SQL LIMIT value (same as PageSize).
func (p Pagination) Limit() int {
	return p.PageSize
}

// NewPageMeta computes pagination metadata given a Pagination and the total
// number of matching items.
func NewPageMeta(p Pagination, totalItems int64) PageMeta {
	totalPages := 1
	if p.PageSize > 0 {
		totalPages = int((totalItems + int64(p.PageSize) - 1) / int64(p.PageSize))
	}
	if totalPages < 1 {
		totalPages = 1
	}
	return PageMeta{
		Page:       p.Page,
		PageSize:   p.PageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
		HasNext:    p.Page < totalPages,
		HasPrev:    p.Page > 1,
	}
}
