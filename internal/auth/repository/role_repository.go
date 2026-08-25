package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// RoleRepository defines the persistence contract for Role records
// and user-role assignments. It has no knowledge of permissions or
// authorization logic.
type RoleRepository interface {
	// FindByID retrieves a role by its UUID.
	FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.Role, error)

	// FindByName retrieves a role by its exact system identifier.
	FindByName(ctx context.Context, name string, opts ...repo.Option) (*domain.Role, error)

	// ExistsByName reports whether a role with the given name exists.
	ExistsByName(ctx context.Context, name string) (bool, error)

	// Create inserts a new role and returns the record with DB-populated fields.
	Create(ctx context.Context, role *domain.Role, opts ...repo.Option) (*domain.Role, error)

	// Update replaces the mutable fields (label, description) of a role.
	// name and is_system are immutable and are never written here.
	Update(ctx context.Context, role *domain.Role, opts ...repo.Option) (*domain.Role, error)

	// Delete hard-deletes a non-system role. Returns ErrConstraintViolation
	// when users still hold the role (enforced by ON DELETE RESTRICT).
	Delete(ctx context.Context, id uuid.UUID, opts ...repo.Option) error

	// List returns a paginated, name-ordered slice of all roles.
	List(ctx context.Context, opts ...repo.Option) ([]*domain.Role, repo.PageMeta, error)

	// AssignToUser creates a user-role assignment. Idempotent: assigning a
	// role the user already holds is not an error.
	// Pass nil for assignedBy when the assignment is system-initiated.
	AssignToUser(ctx context.Context, userID, roleID uuid.UUID, assignedBy *uuid.UUID, opts ...repo.Option) error

	// RemoveFromUser deletes a user-role assignment.
	// Returns ErrNotFound when the assignment does not exist.
	RemoveFromUser(ctx context.Context, userID, roleID uuid.UUID, opts ...repo.Option) error

	// GetUserRoles returns all roles currently assigned to a user, ordered by name.
	GetUserRoles(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.Role, error)

	// HasRole reports whether a specific role is currently assigned to a user.
	HasRole(ctx context.Context, userID, roleID uuid.UUID) (bool, error)

	// GetUserPermissions returns the deduplicated set of permission names granted
	// to a user through all of their role assignments. It resolves the full
	// user → user_roles → roles → role_permissions → permissions chain in one query.
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// ─── implementation ───────────────────────────────────────────────────────────

type roleRepository struct {
	repo.Base
}

// NewRoleRepository constructs a RoleRepository backed by db.
func NewRoleRepository(db *database.Database, log logger.Logger) RoleRepository {
	return &roleRepository{Base: repo.NewBase(db, log)}
}

// ─── column list ──────────────────────────────────────────────────────────────

const roleCols = `id, name, label, description, is_system, created_at`

// ─── scan helper ──────────────────────────────────────────────────────────────

func scanRole(row pgx.Row) (*domain.Role, error) {
	var r domain.Role
	err := row.Scan(
		&r.ID,
		&r.Name,
		&r.Label,
		&r.Description,
		&r.IsSystem,
		&r.CreatedAt,
	)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &r, nil
}

// ─── read operations ──────────────────────────────────────────────────────────

const sqlRoleFindByID = `
SELECT ` + roleCols + `
FROM   roles
WHERE  id = $1`

func (r *roleRepository) FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.Role, error) {
	o := repo.NewOptions(opts...)
	return scanRole(r.Exec(o).QueryRow(ctx, sqlRoleFindByID, id))
}

const sqlRoleFindByName = `
SELECT ` + roleCols + `
FROM   roles
WHERE  name = $1`

func (r *roleRepository) FindByName(ctx context.Context, name string, opts ...repo.Option) (*domain.Role, error) {
	o := repo.NewOptions(opts...)
	return scanRole(r.Exec(o).QueryRow(ctx, sqlRoleFindByName, name))
}

const sqlRoleExistsByName = `
SELECT EXISTS (SELECT 1 FROM roles WHERE name = $1)`

func (r *roleRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var exists bool
	if err := r.Pool().QueryRow(ctx, sqlRoleExistsByName, name).Scan(&exists); err != nil {
		return false, repo.MapError(err)
	}
	return exists, nil
}

const sqlCountRoles = `SELECT COUNT(*) FROM roles`

const sqlListRoles = `
SELECT ` + roleCols + `
FROM   roles
ORDER  BY name ASC
LIMIT  $1 OFFSET $2`

func (r *roleRepository) List(ctx context.Context, opts ...repo.Option) ([]*domain.Role, repo.PageMeta, error) {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)

	p := repo.Pagination{Page: repo.DefaultPage, PageSize: repo.DefaultPageSize}
	if op := o.Pagination(); op != nil {
		p = *op
		p.Normalize()
	}

	var total int64
	if err := db.QueryRow(ctx, sqlCountRoles).Scan(&total); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	rows, err := db.Query(ctx, sqlListRoles, p.Limit(), p.Offset())
	if err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}
	defer rows.Close()

	roles := make([]*domain.Role, 0, p.PageSize)
	for rows.Next() {
		var role domain.Role
		if err := rows.Scan(
			&role.ID,
			&role.Name,
			&role.Label,
			&role.Description,
			&role.IsSystem,
			&role.CreatedAt,
		); err != nil {
			return nil, repo.PageMeta{}, repo.MapError(err)
		}
		roles = append(roles, &role)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	return roles, repo.NewPageMeta(p, total), nil
}

// ─── write operations ─────────────────────────────────────────────────────────

const sqlRoleCreate = `
INSERT INTO roles (id, name, label, description, is_system)
VALUES ($1, $2, $3, $4, $5)
RETURNING ` + roleCols

func (r *roleRepository) Create(ctx context.Context, role *domain.Role, opts ...repo.Option) (*domain.Role, error) {
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlRoleCreate,
		role.ID,
		role.Name,
		role.Label,
		role.Description,
		role.IsSystem,
	)
	return scanRole(row)
}

// sqlRoleUpdate writes only label and description — name and is_system are immutable.
const sqlRoleUpdate = `
UPDATE roles
SET    label       = $2,
       description = $3
WHERE  id = $1
RETURNING ` + roleCols

func (r *roleRepository) Update(ctx context.Context, role *domain.Role, opts ...repo.Option) (*domain.Role, error) {
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlRoleUpdate,
		role.ID,
		role.Label,
		role.Description,
	)
	return scanRole(row)
}

// sqlRoleDelete guards against deleting system roles at the query level.
const sqlRoleDelete = `
DELETE FROM roles WHERE id = $1 AND is_system = FALSE`

func (r *roleRepository) Delete(ctx context.Context, id uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlRoleDelete, id)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

// ─── user-role assignments ────────────────────────────────────────────────────

// sqlAssignRole is idempotent: re-assigning an existing role silently succeeds.
const sqlAssignRole = `
INSERT INTO user_roles (user_id, role_id, assigned_by)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, role_id) DO NOTHING`

func (r *roleRepository) AssignToUser(ctx context.Context, userID, roleID uuid.UUID, assignedBy *uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	_, err := r.Exec(o).Exec(ctx, sqlAssignRole, userID, roleID, assignedBy)
	return repo.MapError(err)
}

const sqlRemoveRole = `
DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`

func (r *roleRepository) RemoveFromUser(ctx context.Context, userID, roleID uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlRemoveRole, userID, roleID)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

const sqlGetUserRoles = `
SELECT r.` + roleCols + `
FROM   roles r
JOIN   user_roles ur ON ur.role_id = r.id
WHERE  ur.user_id = $1
ORDER  BY r.name ASC`

func (r *roleRepository) GetUserRoles(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.Role, error) {
	o := repo.NewOptions(opts...)
	rows, err := r.Exec(o).Query(ctx, sqlGetUserRoles, userID)
	if err != nil {
		return nil, repo.MapError(err)
	}
	defer rows.Close()

	var roles []*domain.Role
	for rows.Next() {
		var role domain.Role
		if err := rows.Scan(
			&role.ID,
			&role.Name,
			&role.Label,
			&role.Description,
			&role.IsSystem,
			&role.CreatedAt,
		); err != nil {
			return nil, repo.MapError(err)
		}
		roles = append(roles, &role)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.MapError(err)
	}

	return roles, nil
}

const sqlHasRole = `
SELECT EXISTS (
    SELECT 1 FROM user_roles WHERE user_id = $1 AND role_id = $2
)`

func (r *roleRepository) HasRole(ctx context.Context, userID, roleID uuid.UUID) (bool, error) {
	var exists bool
	if err := r.Pool().QueryRow(ctx, sqlHasRole, userID, roleID).Scan(&exists); err != nil {
		return false, repo.MapError(err)
	}
	return exists, nil
}

// sqlGetUserPermissions resolves the full user → roles → permissions chain in
// a single query and returns distinct permission names, ordered alphabetically.
const sqlGetUserPermissions = `
SELECT DISTINCT p.name
FROM   permissions p
JOIN   role_permissions rp ON rp.permission_id = p.id
JOIN   user_roles ur       ON ur.role_id        = rp.role_id
WHERE  ur.user_id = $1
ORDER  BY p.name ASC`

func (r *roleRepository) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := r.Pool().Query(ctx, sqlGetUserPermissions, userID)
	if err != nil {
		return nil, repo.MapError(err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, repo.MapError(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.MapError(err)
	}

	return names, nil
}
