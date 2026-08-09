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

// PermissionRepository defines the persistence contract for Permission records
// and role-permission assignments. It has no knowledge of user-role assignments
// or authorization evaluation.
type PermissionRepository interface {
	// FindByID retrieves a permission by its UUID.
	FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.Permission, error)

	// FindByName retrieves a permission by its exact resource:action identifier.
	FindByName(ctx context.Context, name string, opts ...repo.Option) (*domain.Permission, error)

	// ExistsByName reports whether a permission with the given name exists.
	ExistsByName(ctx context.Context, name string) (bool, error)

	// Create inserts a new permission and returns the record with DB-populated fields.
	Create(ctx context.Context, perm *domain.Permission, opts ...repo.Option) (*domain.Permission, error)

	// Update replaces the mutable fields (description, category) of a permission.
	// name is immutable and is never written here.
	Update(ctx context.Context, perm *domain.Permission, opts ...repo.Option) (*domain.Permission, error)

	// Delete hard-deletes a permission. Returns ErrConstraintViolation when the
	// permission is still assigned to a role (enforced by ON DELETE RESTRICT).
	Delete(ctx context.Context, id uuid.UUID, opts ...repo.Option) error

	// List returns a paginated slice of all permissions ordered by category then name.
	List(ctx context.Context, opts ...repo.Option) ([]*domain.Permission, repo.PageMeta, error)

	// ListByCategory returns all permissions belonging to a category, ordered by name.
	ListByCategory(ctx context.Context, category string, opts ...repo.Option) ([]*domain.Permission, error)

	// AssignToRole creates a role-permission assignment. Idempotent: assigning a
	// permission the role already holds is not an error.
	AssignToRole(ctx context.Context, roleID, permissionID uuid.UUID, opts ...repo.Option) error

	// RemoveFromRole deletes a role-permission assignment.
	// Returns ErrNotFound when the assignment does not exist.
	RemoveFromRole(ctx context.Context, roleID, permissionID uuid.UUID, opts ...repo.Option) error

	// GetRolePermissions returns all permissions assigned to a role, ordered by
	// category then name.
	GetRolePermissions(ctx context.Context, roleID uuid.UUID, opts ...repo.Option) ([]*domain.Permission, error)

	// HasPermission reports whether a specific permission is assigned to a role.
	HasPermission(ctx context.Context, roleID, permissionID uuid.UUID) (bool, error)
}

// ─── implementation ───────────────────────────────────────────────────────────

type permissionRepository struct {
	repo.Base
}

// NewPermissionRepository constructs a PermissionRepository backed by db.
func NewPermissionRepository(db *database.Database, log logger.Logger) PermissionRepository {
	return &permissionRepository{Base: repo.NewBase(db, log)}
}

// ─── column list ──────────────────────────────────────────────────────────────

const permCols = `id, name, description, category, created_at`

// ─── scan helper ──────────────────────────────────────────────────────────────

func scanPermission(row pgx.Row) (*domain.Permission, error) {
	var p domain.Permission
	err := row.Scan(
		&p.ID,
		&p.Name,
		&p.Description,
		&p.Category,
		&p.CreatedAt,
	)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &p, nil
}

// ─── read operations ──────────────────────────────────────────────────────────

const sqlPermFindByID = `
SELECT ` + permCols + `
FROM   permissions
WHERE  id = $1`

func (r *permissionRepository) FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.Permission, error) {
	o := repo.NewOptions(opts...)
	return scanPermission(r.Exec(o).QueryRow(ctx, sqlPermFindByID, id))
}

const sqlPermFindByName = `
SELECT ` + permCols + `
FROM   permissions
WHERE  name = $1`

func (r *permissionRepository) FindByName(ctx context.Context, name string, opts ...repo.Option) (*domain.Permission, error) {
	o := repo.NewOptions(opts...)
	return scanPermission(r.Exec(o).QueryRow(ctx, sqlPermFindByName, name))
}

const sqlPermExistsByName = `
SELECT EXISTS (SELECT 1 FROM permissions WHERE name = $1)`

func (r *permissionRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var exists bool
	if err := r.Pool().QueryRow(ctx, sqlPermExistsByName, name).Scan(&exists); err != nil {
		return false, repo.MapError(err)
	}
	return exists, nil
}

const sqlCountPermissions = `SELECT COUNT(*) FROM permissions`

const sqlListPermissions = `
SELECT ` + permCols + `
FROM   permissions
ORDER  BY category ASC, name ASC
LIMIT  $1 OFFSET $2`

func (r *permissionRepository) List(ctx context.Context, opts ...repo.Option) ([]*domain.Permission, repo.PageMeta, error) {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)

	p := repo.Pagination{Page: repo.DefaultPage, PageSize: repo.DefaultPageSize}
	if op := o.Pagination(); op != nil {
		p = *op
		p.Normalize()
	}

	var total int64
	if err := db.QueryRow(ctx, sqlCountPermissions).Scan(&total); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	rows, err := db.Query(ctx, sqlListPermissions, p.Limit(), p.Offset())
	if err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}
	defer rows.Close()

	perms := make([]*domain.Permission, 0, p.PageSize)
	for rows.Next() {
		var perm domain.Permission
		if err := rows.Scan(
			&perm.ID,
			&perm.Name,
			&perm.Description,
			&perm.Category,
			&perm.CreatedAt,
		); err != nil {
			return nil, repo.PageMeta{}, repo.MapError(err)
		}
		perms = append(perms, &perm)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	return perms, repo.NewPageMeta(p, total), nil
}

const sqlListPermsByCategory = `
SELECT ` + permCols + `
FROM   permissions
WHERE  category = $1
ORDER  BY name ASC`

func (r *permissionRepository) ListByCategory(ctx context.Context, category string, opts ...repo.Option) ([]*domain.Permission, error) {
	o := repo.NewOptions(opts...)
	rows, err := r.Exec(o).Query(ctx, sqlListPermsByCategory, category)
	if err != nil {
		return nil, repo.MapError(err)
	}
	defer rows.Close()

	var perms []*domain.Permission
	for rows.Next() {
		var perm domain.Permission
		if err := rows.Scan(
			&perm.ID,
			&perm.Name,
			&perm.Description,
			&perm.Category,
			&perm.CreatedAt,
		); err != nil {
			return nil, repo.MapError(err)
		}
		perms = append(perms, &perm)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.MapError(err)
	}

	return perms, nil
}

// ─── write operations ─────────────────────────────────────────────────────────

const sqlPermCreate = `
INSERT INTO permissions (id, name, description, category)
VALUES ($1, $2, $3, $4)
RETURNING ` + permCols

func (r *permissionRepository) Create(ctx context.Context, perm *domain.Permission, opts ...repo.Option) (*domain.Permission, error) {
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlPermCreate,
		perm.ID,
		perm.Name,
		perm.Description,
		perm.Category,
	)
	return scanPermission(row)
}

// sqlPermUpdate writes only description and category — name is immutable.
const sqlPermUpdate = `
UPDATE permissions
SET    description = $2,
       category    = $3
WHERE  id = $1
RETURNING ` + permCols

func (r *permissionRepository) Update(ctx context.Context, perm *domain.Permission, opts ...repo.Option) (*domain.Permission, error) {
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlPermUpdate,
		perm.ID,
		perm.Description,
		perm.Category,
	)
	return scanPermission(row)
}

const sqlPermDelete = `
DELETE FROM permissions WHERE id = $1`

func (r *permissionRepository) Delete(ctx context.Context, id uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlPermDelete, id)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

// ─── role-permission assignments ──────────────────────────────────────────────

// sqlAssignPermission is idempotent: re-assigning an existing permission silently succeeds.
const sqlAssignPermission = `
INSERT INTO role_permissions (role_id, permission_id)
VALUES ($1, $2)
ON CONFLICT (role_id, permission_id) DO NOTHING`

func (r *permissionRepository) AssignToRole(ctx context.Context, roleID, permissionID uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	_, err := r.Exec(o).Exec(ctx, sqlAssignPermission, roleID, permissionID)
	return repo.MapError(err)
}

const sqlRemovePermission = `
DELETE FROM role_permissions WHERE role_id = $1 AND permission_id = $2`

func (r *permissionRepository) RemoveFromRole(ctx context.Context, roleID, permissionID uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlRemovePermission, roleID, permissionID)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

const sqlGetRolePermissions = `
SELECT p.` + permCols + `
FROM   permissions p
JOIN   role_permissions rp ON rp.permission_id = p.id
WHERE  rp.role_id = $1
ORDER  BY p.category ASC, p.name ASC`

func (r *permissionRepository) GetRolePermissions(ctx context.Context, roleID uuid.UUID, opts ...repo.Option) ([]*domain.Permission, error) {
	o := repo.NewOptions(opts...)
	rows, err := r.Exec(o).Query(ctx, sqlGetRolePermissions, roleID)
	if err != nil {
		return nil, repo.MapError(err)
	}
	defer rows.Close()

	var perms []*domain.Permission
	for rows.Next() {
		var perm domain.Permission
		if err := rows.Scan(
			&perm.ID,
			&perm.Name,
			&perm.Description,
			&perm.Category,
			&perm.CreatedAt,
		); err != nil {
			return nil, repo.MapError(err)
		}
		perms = append(perms, &perm)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.MapError(err)
	}

	return perms, nil
}

const sqlHasPermission = `
SELECT EXISTS (
    SELECT 1 FROM role_permissions WHERE role_id = $1 AND permission_id = $2
)`

func (r *permissionRepository) HasPermission(ctx context.Context, roleID, permissionID uuid.UUID) (bool, error) {
	var exists bool
	if err := r.Pool().QueryRow(ctx, sqlHasPermission, roleID, permissionID).Scan(&exists); err != nil {
		return false, repo.MapError(err)
	}
	return exists, nil
}
