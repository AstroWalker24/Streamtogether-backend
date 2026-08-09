package repository

// SortOrder represents the direction of a sort clause.
type SortOrder string

const (
    SortAsc  SortOrder = "ASC"
    SortDesc SortOrder = "DESC"
)

// SortField pairs a column name with its sort direction.
type SortField struct {
    Field string
    Order SortOrder
}

// Filters holds generic query-filtering parameters shared across all repositories.
type Filters struct {
    // Search is a free-text search term applied by each repository as appropriate.
    Search string
    // SortFields defines the ORDER BY clause; earlier entries take precedence.
    SortFields []SortField
    // Fields holds exact-match predicates keyed by column name.
    Fields map[string]any
}

// NewFilters returns a zero-value Filters with an initialised Fields map.
func NewFilters() Filters {
    return Filters{Fields: make(map[string]any)}
}

// WithSearch returns a copy of f with the search term set.
func (f Filters) WithSearch(q string) Filters {
    f.Search = q
    return f
}

// WithSort returns a copy of f with the given sort field appended.
func (f Filters) WithSort(field string, order SortOrder) Filters {
    f.SortFields = append(f.SortFields, SortField{Field: field, Order: order})
    return f
}

// WithField returns a copy of f with an exact-match predicate for the given column.
func (f Filters) WithField(key string, val any) Filters {
    if f.Fields == nil {
        f.Fields = make(map[string]any)
    }
    f.Fields[key] = val
    return f
}