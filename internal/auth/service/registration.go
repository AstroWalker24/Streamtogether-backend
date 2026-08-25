package service

import (
	"context"
	stderrors "errors"
	"regexp"
	"strings"
	"unicode"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
	autherrors "github.com/AstroWalker24/Streamtogether-backend/internal/auth/errors"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/mapper"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
)

// RegistrationService handles the user registration flow.
type RegistrationService interface {
	// Register validates the request, enforces business rules, and creates a new
	// user account in the pending_verification state.
	Register(ctx context.Context, req dto.RegisterRequest) (dto.UserResponse, error)
}

type registrationService struct {
	users UserService
}

// NewRegistrationService constructs a RegistrationService backed by the given UserService.
func NewRegistrationService(users UserService) RegistrationService {
	return &registrationService{users: users}
}

// usernamePattern enforces the domain contract: alphanumeric characters and underscores only.
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func (s *registrationService) Register(ctx context.Context, req dto.RegisterRequest) (dto.UserResponse, error) {
	req.Normalize()

	if !usernamePattern.MatchString(req.Username) {
		return dto.UserResponse{}, apperrors.NewBadRequest("username may only contain letters, numbers, and underscores")
	}

	if details := passwordPolicyViolation(req.Password); details != "" {
		return dto.UserResponse{}, autherrors.NewPasswordTooWeak(details)
	}

	available, err := s.users.IsEmailAvailable(ctx, req.Email)
	if err != nil {
		return dto.UserResponse{}, err
	}
	if !available {
		return dto.UserResponse{}, autherrors.NewEmailExists()
	}

	available, err = s.users.IsUsernameAvailable(ctx, req.Username)
	if err != nil {
		return dto.UserResponse{}, err
	}
	if !available {
		return dto.UserResponse{}, autherrors.NewUsernameExists()
	}

	u, err := s.users.Create(ctx, req.Email, req.Username, req.Password)
	if err != nil {
		return dto.UserResponse{}, translateCreateError(err)
	}

	return mapper.UserToResponse(u), nil
}

// passwordPolicyViolation enforces BR-11: uppercase, lowercase, digit, and special character.
// Returns a human-readable description of the first failing rule, or "" if all pass.
func passwordPolicyViolation(password string) string {
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}
	switch {
	case !hasUpper:
		return "password must contain at least one uppercase letter"
	case !hasLower:
		return "password must contain at least one lowercase letter"
	case !hasDigit:
		return "password must contain at least one digit"
	case !hasSpecial:
		return "password must contain at least one special character"
	default:
		return ""
	}
}

// translateCreateError maps UserService Conflict errors to registration-specific auth errors.
// This handles the race condition where two concurrent requests both pass the availability
// checks but only one succeeds at the database level.
func translateCreateError(err error) error {
	var appErr *apperrors.AppError
	if stderrors.As(err, &appErr) && appErr.Code == apperrors.CodeConflict {
		if strings.Contains(appErr.Message, "email") {
			return autherrors.NewEmailExists()
		}
		return autherrors.NewUsernameExists()
	}
	return err
}
