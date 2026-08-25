package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/dto"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/mapper"
	authrepo "github.com/AstroWalker24/Streamtogether-backend/internal/auth/repository"
	"github.com/AstroWalker24/Streamtogether-backend/internal/auth/token"
	"github.com/AstroWalker24/Streamtogether-backend/internal/database"
	apperrors "github.com/AstroWalker24/Streamtogether-backend/internal/errors"
	repo "github.com/AstroWalker24/Streamtogether-backend/internal/repository"
)

// DeviceService manages the lifecycle of registered devices (§5.3).
type DeviceService interface {
	// ListDevices returns all active (non-revoked) devices owned by the caller.
	ListDevices(ctx context.Context, claims token.AccessTokenClaims) ([]dto.DeviceResponse, error)

	// RevokeDevice revokes the named device and all its associated sessions (INV-13).
	// Validates that the device belongs to the caller.
	RevokeDevice(ctx context.Context, claims token.AccessTokenClaims, deviceID uuid.UUID) (dto.MessageResponse, error)

	// RenameDevice updates the friendly name of a device owned by the caller.
	RenameDevice(ctx context.Context, claims token.AccessTokenClaims, deviceID uuid.UUID, req dto.RenameDeviceRequest) (dto.DeviceResponse, error)
}

type deviceService struct {
	devices  authrepo.DeviceRepository
	sessions authrepo.SessionRepository
	db       *database.Database
}

// NewDeviceService constructs a DeviceService with explicit dependencies.
func NewDeviceService(
	devices authrepo.DeviceRepository,
	sessions authrepo.SessionRepository,
	db *database.Database,
) DeviceService {
	return &deviceService{
		devices:  devices,
		sessions: sessions,
		db:       db,
	}
}

func (s *deviceService) ListDevices(ctx context.Context, claims token.AccessTokenClaims) ([]dto.DeviceResponse, error) {
	devs, _, err := s.devices.ListActiveByUser(ctx, claims.UserID)
	if err != nil {
		return nil, apperrors.NewDatabase(err)
	}
	out := make([]dto.DeviceResponse, 0, len(devs))
	for _, d := range devs {
		out = append(out, mapper.DeviceToResponse(d))
	}
	return out, nil
}

func (s *deviceService) RevokeDevice(ctx context.Context, claims token.AccessTokenClaims, deviceID uuid.UUID) (dto.MessageResponse, error) {
	// Verify existence and ownership before entering the transaction.
	device, err := s.devices.FindByID(ctx, deviceID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return dto.MessageResponse{}, apperrors.NewNotFound("device")
		}
		return dto.MessageResponse{}, apperrors.NewDatabase(err)
	}

	if device.UserID != claims.UserID {
		return dto.MessageResponse{}, apperrors.NewForbidden("not authorized to revoke this device")
	}

	// INV-13: device revocation and session revocation are one atomic operation.
	txErr := repo.RunInTransaction(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		opt := repo.WithTransaction(tx)

		if err := s.devices.Revoke(ctx, deviceID, opt); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				// Revoked concurrently between the ownership check and the transaction.
				return nil
			}
			return apperrors.NewDatabase(err)
		}
		return s.sessions.RevokeAllForDevice(ctx, deviceID, opt)
	})
	if txErr != nil {
		return dto.MessageResponse{}, txErr
	}

	return dto.MessageResponse{Message: "device revoked"}, nil
}

func (s *deviceService) RenameDevice(ctx context.Context, claims token.AccessTokenClaims, deviceID uuid.UUID, req dto.RenameDeviceRequest) (dto.DeviceResponse, error) {
	device, err := s.devices.FindByID(ctx, deviceID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return dto.DeviceResponse{}, apperrors.NewNotFound("device")
		}
		return dto.DeviceResponse{}, apperrors.NewDatabase(err)
	}

	if device.UserID != claims.UserID {
		return dto.DeviceResponse{}, apperrors.NewForbidden("not authorized to rename this device")
	}

	device.FriendlyName = strings.TrimSpace(req.Name)
	updated, err := s.devices.UpdateMetadata(ctx, device)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return dto.DeviceResponse{}, apperrors.NewNotFound("device")
		}
		return dto.DeviceResponse{}, apperrors.NewDatabase(err)
	}

	return mapper.DeviceToResponse(updated), nil
}
