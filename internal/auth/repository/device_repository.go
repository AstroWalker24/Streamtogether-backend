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

// DeviceRepository defines the persistence contract for Device records.
// It does not interact with sessions or refresh tokens.
type DeviceRepository interface {
	// FindByID retrieves a non-revoked device by its UUID.
	FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.Device, error)

	// FindActiveByFingerprint retrieves a non-revoked device matching the given
	// user and fingerprint hash. Used to recognize returning devices at login.
	FindActiveByFingerprint(ctx context.Context, userID uuid.UUID, fingerprintHash string, opts ...repo.Option) (*domain.Device, error)

	// Register persists a new device record and returns it with DB-populated fields.
	Register(ctx context.Context, device *domain.Device, opts ...repo.Option) (*domain.Device, error)

	// UpdateMetadata replaces the mutable display fields: friendly_name, browser, os.
	UpdateMetadata(ctx context.Context, device *domain.Device, opts ...repo.Option) (*domain.Device, error)

	// UpdateLastActive refreshes last_active_at and last_ip_address.
	// Called on every successful token refresh. Returns ErrNotFound if the device
	// is revoked or does not exist.
	UpdateLastActive(ctx context.Context, id uuid.UUID, lastIP string, opts ...repo.Option) error

	// SetTrusted toggles the trusted flag (reserved for future reduced-friction flows).
	// Returns ErrNotFound if the device is revoked or does not exist.
	SetTrusted(ctx context.Context, id uuid.UUID, trusted bool, opts ...repo.Option) error

	// Revoke marks a non-revoked device as revoked, preventing new sessions.
	// Returns ErrNotFound if the device is already revoked or does not exist.
	Revoke(ctx context.Context, id uuid.UUID, opts ...repo.Option) error

	// RevokeAllForUser revokes all active devices belonging to userID.
	// A no-op (not an error) when the user has no active devices.
	RevokeAllForUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) error

	// ListActiveByUser returns a paginated slice of non-revoked devices for a user,
	// ordered by most recent activity descending.
	ListActiveByUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.Device, repo.PageMeta, error)

	// CountActiveByUser returns the number of non-revoked devices for a user.
	// Used to enforce the per-user device limit before registering a new device.
	CountActiveByUser(ctx context.Context, userID uuid.UUID) (int, error)
}

// ─── implementation ───────────────────────────────────────────────────────────

type deviceRepository struct {
	repo.Base
}

// NewDeviceRepository constructs a DeviceRepository backed by db.
func NewDeviceRepository(db *database.Database, log logger.Logger) DeviceRepository {
	return &deviceRepository{Base: repo.NewBase(db, log)}
}

// ─── column list ──────────────────────────────────────────────────────────────

// deviceCols casts last_ip_address to TEXT so the domain layer stays free of
// pgx-specific INET types.
const deviceCols = `id, user_id, fingerprint_hash, friendly_name, platform,
       browser, os, last_ip_address::TEXT, first_seen_at, last_active_at,
       trusted, revoked, revoked_at`

// ─── scan helper ──────────────────────────────────────────────────────────────

func scanDevice(row pgx.Row) (*domain.Device, error) {
	var d domain.Device
	err := row.Scan(
		&d.ID,
		&d.UserID,
		&d.FingerprintHash,
		&d.FriendlyName,
		(*string)(&d.Platform),
		&d.Browser,
		&d.OS,
		&d.LastIPAddress,
		&d.FirstSeenAt,
		&d.LastActiveAt,
		&d.Trusted,
		&d.Revoked,
		&d.RevokedAt,
	)
	if err != nil {
		return nil, repo.MapError(err)
	}
	return &d, nil
}

// ─── read operations ──────────────────────────────────────────────────────────

const sqlDeviceFindByID = `
SELECT ` + deviceCols + `
FROM   devices
WHERE  id = $1
  AND  revoked = FALSE`

func (r *deviceRepository) FindByID(ctx context.Context, id uuid.UUID, opts ...repo.Option) (*domain.Device, error) {
	o := repo.NewOptions(opts...)
	return scanDevice(r.Exec(o).QueryRow(ctx, sqlDeviceFindByID, id))
}

// sqlDeviceFindActiveByFingerprint uses the partial unique index
// uq_devices_user_fingerprint_active on (user_id, fingerprint_hash) WHERE revoked = FALSE.
const sqlDeviceFindActiveByFingerprint = `
SELECT ` + deviceCols + `
FROM   devices
WHERE  user_id          = $1
  AND  fingerprint_hash = $2
  AND  revoked          = FALSE`

func (r *deviceRepository) FindActiveByFingerprint(ctx context.Context, userID uuid.UUID, fingerprintHash string, opts ...repo.Option) (*domain.Device, error) {
	o := repo.NewOptions(opts...)
	return scanDevice(r.Exec(o).QueryRow(ctx, sqlDeviceFindActiveByFingerprint, userID, fingerprintHash))
}

const sqlCountActiveDevices = `
SELECT COUNT(*)
FROM   devices
WHERE  user_id  = $1
  AND  revoked  = FALSE`

func (r *deviceRepository) CountActiveByUser(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	if err := r.Pool().QueryRow(ctx, sqlCountActiveDevices, userID).Scan(&count); err != nil {
		return 0, repo.MapError(err)
	}
	return count, nil
}

const sqlCountActiveDevicesForList = `
SELECT COUNT(*)
FROM   devices
WHERE  user_id = $1
  AND  revoked = FALSE`

// sqlListActiveDevices uses idx_devices_user_id_active on (user_id, last_active_at DESC)
// WHERE revoked = FALSE.
const sqlListActiveDevices = `
SELECT ` + deviceCols + `
FROM   devices
WHERE  user_id = $1
  AND  revoked = FALSE
ORDER  BY last_active_at DESC
LIMIT  $2 OFFSET $3`

func (r *deviceRepository) ListActiveByUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) ([]*domain.Device, repo.PageMeta, error) {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)

	p := repo.Pagination{Page: repo.DefaultPage, PageSize: repo.DefaultPageSize}
	if op := o.Pagination(); op != nil {
		p = *op
		p.Normalize()
	}

	var total int64
	if err := db.QueryRow(ctx, sqlCountActiveDevicesForList, userID).Scan(&total); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	rows, err := db.Query(ctx, sqlListActiveDevices, userID, p.Limit(), p.Offset())
	if err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}
	defer rows.Close()

	devices := make([]*domain.Device, 0, p.PageSize)
	for rows.Next() {
		var d domain.Device
		if err := rows.Scan(
			&d.ID,
			&d.UserID,
			&d.FingerprintHash,
			&d.FriendlyName,
			(*string)(&d.Platform),
			&d.Browser,
			&d.OS,
			&d.LastIPAddress,
			&d.FirstSeenAt,
			&d.LastActiveAt,
			&d.Trusted,
			&d.Revoked,
			&d.RevokedAt,
		); err != nil {
			return nil, repo.PageMeta{}, repo.MapError(err)
		}
		devices = append(devices, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.PageMeta{}, repo.MapError(err)
	}

	return devices, repo.NewPageMeta(p, total), nil
}

// ─── write operations ─────────────────────────────────────────────────────────

// sqlDeviceRegister casts last_ip_address to INET on write; RETURNING uses ::TEXT.
const sqlDeviceRegister = `
INSERT INTO devices (id, user_id, fingerprint_hash, friendly_name, platform, browser, os, last_ip_address)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8::INET)
RETURNING ` + deviceCols

func (r *deviceRepository) Register(ctx context.Context, device *domain.Device, opts ...repo.Option) (*domain.Device, error) {
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlDeviceRegister,
		device.ID,
		device.UserID,
		device.FingerprintHash,
		device.FriendlyName,
		string(device.Platform),
		device.Browser,
		device.OS,
		device.LastIPAddress,
	)
	return scanDevice(row)
}

const sqlDeviceUpdateMetadata = `
UPDATE devices
SET    friendly_name = $2,
       browser       = $3,
       os            = $4
WHERE  id      = $1
  AND  revoked = FALSE
RETURNING ` + deviceCols

func (r *deviceRepository) UpdateMetadata(ctx context.Context, device *domain.Device, opts ...repo.Option) (*domain.Device, error) {
	o := repo.NewOptions(opts...)
	row := r.Exec(o).QueryRow(ctx, sqlDeviceUpdateMetadata,
		device.ID,
		device.FriendlyName,
		device.Browser,
		device.OS,
	)
	return scanDevice(row)
}

const sqlDeviceUpdateLastActive = `
UPDATE devices
SET    last_active_at  = NOW(),
       last_ip_address = $2::INET
WHERE  id      = $1
  AND  revoked = FALSE`

func (r *deviceRepository) UpdateLastActive(ctx context.Context, id uuid.UUID, lastIP string, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlDeviceUpdateLastActive, id, lastIP)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

const sqlDeviceSetTrusted = `
UPDATE devices
SET    trusted = $2
WHERE  id      = $1
  AND  revoked = FALSE`

func (r *deviceRepository) SetTrusted(ctx context.Context, id uuid.UUID, trusted bool, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlDeviceSetTrusted, id, trusted)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

const sqlDeviceRevoke = `
UPDATE devices
SET    revoked    = TRUE,
       revoked_at = NOW()
WHERE  id      = $1
  AND  revoked = FALSE`

func (r *deviceRepository) Revoke(ctx context.Context, id uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	db := r.Exec(o)
	tag, err := db.Exec(ctx, sqlDeviceRevoke, id)
	if err != nil {
		return repo.MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

const sqlDeviceRevokeAllForUser = `
UPDATE devices
SET    revoked    = TRUE,
       revoked_at = NOW()
WHERE  user_id = $1
  AND  revoked = FALSE`

func (r *deviceRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID, opts ...repo.Option) error {
	o := repo.NewOptions(opts...)
	_, err := r.Exec(o).Exec(ctx, sqlDeviceRevokeAllForUser, userID)
	return repo.MapError(err)
}
