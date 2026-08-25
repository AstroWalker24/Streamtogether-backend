package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/domain"
	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// RoleService manages role assignment, revocation, and role-membership queries.
// It is the authoritative service for mutating user-role relationships and
// signals PermissionService to invalidate cached permission sets on each mutation.
type RoleService interface {
	// AssignRole assigns the named role to a user. The role must exist.
	// Assigning a role the user already holds is idempotent.
	AssignRole(ctx context.Context, userID uuid.UUID, roleName string) error

	// RemoveRole revokes the named role from a user. The role must exist
	// and the user must currently hold it; otherwise ErrNotFound is returned.
	RemoveRole(ctx context.Context, userID uuid.UUID, roleName string) error

	// GetUserRoles returns all roles currently assigned to a user, ordered by name.
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]*domain.Role, error)

	// HasRole reports whether the user currently holds the named role.
	// Returns false (not an error) when the role name does not exist in the system.
	HasRole(ctx context.Context, userID uuid.UUID, roleName string) (bool, error)
}

type roleService struct {
	roles authrepo.RoleRepository
	perms PermissionService
}

// NewRoleService constructs a RoleService.
// perms is used to invalidate the permission cache after role mutations.
func NewRoleService(roles authrepo.RoleRepository, perms PermissionService) RoleService {
	return &roleService{roles: roles, perms: perms}
}

func (s *roleService) AssignRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	role, err := s.roles.FindByName(ctx, roleName)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return apperrors.NewNotFound("role")
		}
		return apperrors.NewDatabase(err)
	}
	if err := s.roles.AssignToUser(ctx, userID, role.ID, nil); err != nil {
		return apperrors.NewDatabase(err)
	}
	s.perms.InvalidateCache(ctx, userID)
	return nil
}

func (s *roleService) RemoveRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	role, err := s.roles.FindByName(ctx, roleName)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return apperrors.NewNotFound("role")
		}
		return apperrors.NewDatabase(err)
	}

	held, err := s.roles.HasRole(ctx, userID, role.ID)
	if err != nil {
		return apperrors.NewDatabase(err)
	}
	if !held {
		return apperrors.NewNotFound("role assignment")
	}

	if err := s.roles.RemoveFromUser(ctx, userID, role.ID); err != nil {
		return apperrors.NewDatabase(err)
	}
	s.perms.InvalidateCache(ctx, userID)
	return nil
}

func (s *roleService) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]*domain.Role, error) {
	roles, err := s.roles.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	return roles, nil
}

func (s *roleService) HasRole(ctx context.Context, userID uuid.UUID, roleName string) (bool, error) {
	role, err := s.roles.FindByName(ctx, roleName)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			// Non-existent role name is effectively not held.
			return false, nil
		}
		return false, apperrors.NewDatabase(err)
	}
	held, err := s.roles.HasRole(ctx, userID, role.ID)
	if err != nil {
		return false, apperrors.NewDatabase(err)
	}
	return held, nil
}
