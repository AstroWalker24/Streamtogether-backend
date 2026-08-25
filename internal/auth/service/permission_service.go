package service

import (
	"context"

	"github.com/google/uuid"

	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// PermissionService resolves the effective permission set for a user and
// evaluates authorization predicates. It is the authoritative query path for
// "can this user do X?" checks across all application domains.
//
// Permission resolution follows: user → user_roles → roles → role_permissions → permissions.
// The effective permission set is the deduplicated union of all permissions
// granted through every role the user currently holds.
type PermissionService interface {
	// GetUserPermissions returns the deduplicated set of permission names
	// granted to a user through all of their assigned roles.
	// Returns an empty slice (not an error) for a user with no roles.
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error)

	// HasPermission reports whether the user's effective permission set
	// includes permissionName.
	HasPermission(ctx context.Context, userID uuid.UUID, permissionName string) (bool, error)

	// HasAnyPermission reports whether the user holds at least one of the
	// named permissions. Returns false for an empty permissionNames list.
	HasAnyPermission(ctx context.Context, userID uuid.UUID, permissionNames ...string) (bool, error)

	// HasAllPermissions reports whether the user holds every named permission.
	// Returns true for an empty permissionNames list (vacuous truth).
	HasAllPermissions(ctx context.Context, userID uuid.UUID, permissionNames ...string) (bool, error)

	// InvalidateCache evicts the cached permission set for userID.
	// Called by RoleService after any role assignment or removal.
	// This is a no-op until Redis permission caching is introduced.
	InvalidateCache(ctx context.Context, userID uuid.UUID)
}

type permissionService struct {
	roles authrepo.RoleRepository
}

// NewPermissionService constructs a PermissionService backed by the role repository.
func NewPermissionService(roles authrepo.RoleRepository) PermissionService {
	return &permissionService{roles: roles}
}

func (s *permissionService) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	names, err := s.roles.GetUserPermissions(ctx, userID)
	if err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	if names == nil {
		return []string{}, nil
	}
	return names, nil
}

func (s *permissionService) HasPermission(ctx context.Context, userID uuid.UUID, permissionName string) (bool, error) {
	perms, err := s.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, p := range perms {
		if p == permissionName {
			return true, nil
		}
	}
	return false, nil
}

func (s *permissionService) HasAnyPermission(ctx context.Context, userID uuid.UUID, permissionNames ...string) (bool, error) {
	if len(permissionNames) == 0 {
		return false, nil
	}
	perms, err := s.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, err
	}
	held := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		held[p] = struct{}{}
	}
	for _, wanted := range permissionNames {
		if _, ok := held[wanted]; ok {
			return true, nil
		}
	}
	return false, nil
}

func (s *permissionService) HasAllPermissions(ctx context.Context, userID uuid.UUID, permissionNames ...string) (bool, error) {
	if len(permissionNames) == 0 {
		return true, nil
	}
	perms, err := s.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, err
	}
	held := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		held[p] = struct{}{}
	}
	for _, wanted := range permissionNames {
		if _, ok := held[wanted]; !ok {
			return false, nil
		}
	}
	return true, nil
}

// InvalidateCache is a no-op until Redis permission caching is introduced.
// RoleService calls this after every role mutation so the wiring is established.
func (s *permissionService) InvalidateCache(_ context.Context, _ uuid.UUID) {}
