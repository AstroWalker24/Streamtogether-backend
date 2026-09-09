// Package service contains the business operation layer for the profile domain.
package service

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/token"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	"github.com/AstroWalker24/Streamtogether-backend/internal/profile/domain"
	profileerrors "github.com/AstroWalker24/Streamtogether-backend/internal/profile/errors"
	profilerepo "github.com/AstroWalker24/Streamtogether-backend/internal/profile/repository"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// UpdateInput carries the user-editable presentation fields for a profile update.
// All fields are optional: a nil pointer clears the field; a non-nil pointer sets it.
type UpdateInput struct {
	DisplayName *string
	Bio         *string
	AvatarURL   *string
}

// ProfileService defines the business operations for the profile domain.
type ProfileService interface {
	// Create provisions an empty profile for the given user.
	// Called during user registration and OAuth provisioning.
	// Returns an error wrapping CodeProfileAlreadyExists when a profile
	// already exists for userID.
	Create(ctx context.Context, userID uuid.UUID) (*domain.Profile, error)

	// GetByUserID retrieves the profile belonging to userID.
	// Returns an error wrapping CodeProfileNotFound when no profile exists.
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.Profile, error)

	// GetMyProfile retrieves the authenticated caller's own profile.
	// Returns an error wrapping CodeProfileNotFound when no profile exists.
	GetMyProfile(ctx context.Context, claims token.AccessTokenClaims) (*domain.Profile, error)

	// UpdateMyProfile validates, normalises, and applies input to the
	// authenticated caller's profile. Callers may only update their own profile.
	UpdateMyProfile(ctx context.Context, claims token.AccessTokenClaims, input UpdateInput) (*domain.Profile, error)
}

// ─── compiled validation helpers ─────────────────────────────────────────────

// displayNamePattern allows Unicode letters, digits, spaces, hyphens, underscores, and periods.
var displayNamePattern = regexp.MustCompile(`^[\p{L}\p{N} \-_.]+$`)

// consecutiveSpaces matches two or more consecutive space characters.
var consecutiveSpaces = regexp.MustCompile(` {2,}`)

// excessiveNewlines matches three or more consecutive newline characters.
var excessiveNewlines = regexp.MustCompile(`\n{3,}`)

// ─── implementation ───────────────────────────────────────────────────────────

type profileService struct {
	profiles profilerepo.ProfileRepository
}

// NewProfileService constructs a ProfileService backed by profiles.
func NewProfileService(profiles profilerepo.ProfileRepository) ProfileService {
	return &profileService{profiles: profiles}
}

func (s *profileService) Create(ctx context.Context, userID uuid.UUID) (*domain.Profile, error) {
	p := &domain.Profile{
		ID:     uuid.New(),
		UserID: userID,
	}
	created, err := s.profiles.Create(ctx, p)
	if err != nil {
		if errors.Is(err, repo.ErrDuplicateKey) {
			return nil, profileerrors.NewProfileAlreadyExists()
		}
		return nil, apperrors.NewDatabase(err)
	}
	return created, nil
}

func (s *profileService) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.Profile, error) {
	p, err := s.profiles.FindByUserID(ctx, userID)
	if err != nil {
		return nil, mapProfileRepoError(err)
	}
	return p, nil
}

func (s *profileService) GetMyProfile(ctx context.Context, claims token.AccessTokenClaims) (*domain.Profile, error) {
	return s.GetByUserID(ctx, claims.UserID)
}

func (s *profileService) UpdateMyProfile(ctx context.Context, claims token.AccessTokenClaims, input UpdateInput) (*domain.Profile, error) {
	// Validate and normalise all fields before touching the database.
	displayName, err := validateDisplayName(input.DisplayName)
	if err != nil {
		return nil, err
	}
	bio, err := validateBio(input.Bio)
	if err != nil {
		return nil, err
	}
	avatarURL, err := validateAvatarURL(input.AvatarURL)
	if err != nil {
		return nil, err
	}

	// Fetch the existing profile to obtain its surrogate ID for the UPDATE.
	p, err := s.profiles.FindByUserID(ctx, claims.UserID)
	if err != nil {
		return nil, mapProfileRepoError(err)
	}

	p.DisplayName = displayName
	p.Bio = bio
	p.AvatarURL = avatarURL

	updated, err := s.profiles.Update(ctx, p)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, profileerrors.NewProfileNotFound()
		}
		return nil, apperrors.NewDatabase(err)
	}
	return updated, nil
}

// ─── field validation ─────────────────────────────────────────────────────────

func validateDisplayName(v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil, nil
	}
	s = consecutiveSpaces.ReplaceAllString(s, " ")

	n := utf8.RuneCountInString(s)
	if n < 2 {
		return nil, profileerrors.NewProfileInvalidDisplayName("display name must be at least 2 characters")
	}
	if n > 50 {
		return nil, profileerrors.NewProfileInvalidDisplayName("display name must not exceed 50 characters")
	}
	if !displayNamePattern.MatchString(s) {
		return nil, profileerrors.NewProfileInvalidDisplayName(
			"display name may only contain letters, numbers, spaces, hyphens, underscores, and periods",
		)
	}
	return &s, nil
}

func validateBio(v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil, nil
	}
	s = excessiveNewlines.ReplaceAllString(s, "\n\n")

	if utf8.RuneCountInString(s) > 300 {
		return nil, profileerrors.NewProfileInvalidBio("bio must not exceed 300 characters")
	}
	return &s, nil
}

func validateAvatarURL(v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil, nil
	}
	if len(s) > 2048 {
		return nil, profileerrors.NewProfileInvalidAvatarURL("avatar URL must not exceed 2048 characters")
	}
	u, err := url.ParseRequestURI(s)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, profileerrors.NewProfileInvalidAvatarURL("avatar URL must be a valid HTTPS URL")
	}
	return &s, nil
}

// mapProfileRepoError translates repository sentinel errors into profile domain errors.
func mapProfileRepoError(err error) error {
	if errors.Is(err, repo.ErrNotFound) {
		return profileerrors.NewProfileNotFound()
	}
	if errors.Is(err, repo.ErrDuplicateKey) {
		return profileerrors.NewProfileAlreadyExists()
	}
	return apperrors.NewDatabase(err)
}
