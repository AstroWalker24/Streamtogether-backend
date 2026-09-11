package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/AstroWalker24/Streamtogether-backend/internal/logger"
	"github.com/AstroWalker24/Streamtogether-backend/internal/presence/domain"
	presenceerrors "github.com/AstroWalker24/Streamtogether-backend/internal/presence/errors"
	"github.com/AstroWalker24/Streamtogether-backend/internal/redis"
)

// redisPresenceRepository implements EphemeralPresenceRepository using Redis.
type redisPresenceRepository struct {
	redis *redis.Redis
	log   logger.Logger
}

// NewEphemeralPresenceRepository constructs an EphemeralPresenceRepository backed by Redis.
func NewEphemeralPresenceRepository(r *redis.Redis, log logger.Logger) EphemeralPresenceRepository {
	return &redisPresenceRepository{
		redis: r,
		log:   log,
	}
}

// Key helper functions according to Phase 3.5.2 design.
func userKey(userID uuid.UUID) string {
	return fmt.Sprintf("presence:u:%s", userID.String())
}

func userLeasesKey(userID uuid.UUID) string {
	return fmt.Sprintf("presence:u:%s:leases", userID.String())
}

func userConnsKey(userID uuid.UUID) string {
	return fmt.Sprintf("presence:u:%s:conns", userID.String())
}

func userGraceKey(userID uuid.UUID) string {
	return fmt.Sprintf("presence:u:%s:grace", userID.String())
}

func nodeConnsKey(nodeID string) string {
	return fmt.Sprintf("presence:node:%s:conns", nodeID)
}

// connectionMetadataDTO defines the JSON serialization format for presence:u:{user_id}:conns.
type connectionMetadataDTO struct {
	ConnID        string `json:"conn_id"`
	UserID        string `json:"user_id"`
	SessionID     string `json:"session_id"`
	DeviceID      string `json:"device_id"`
	NodeID        string `json:"node_id"`
	Platform      string `json:"platform"`
	State         string `json:"state"`
	ConnectedAt   string `json:"connected_at"`
	LastHeartbeat string `json:"last_heartbeat"`
}

// ─── Lua Scripts ─────────────────────────────────────────────────────────────

// registerConnectionScript atomically:
// 1. Adds connectionID to leases with score (now + leaseDuration)
// 2. Adds connection metadata JSON to conns
// 3. Deletes any active grace key
// 4. Prunes expired leases
// 5. Recomputes effective status across remaining active connections
// 6. Updates presence:u:{userID} hash
// 7. Refreshes sliding TTL (e.g. 60s) on all user keys
//
// KEYS[1] = presence:u:{userID}
// KEYS[2] = presence:u:{userID}:leases
// KEYS[3] = presence:u:{userID}:conns
// KEYS[4] = presence:u:{userID}:grace
// ARGV[1] = connectionID (string)
// ARGV[2] = leaseExpiry (unix timestamp seconds float/int)
// ARGV[3] = connectionMetadataJSON (string)
// ARGV[4] = nowTimestamp (RFC3339 string)
// ARGV[5] = nowUnix (float/int)
// ARGV[6] = slidingTTL (seconds)
var registerConnectionScript = goredis.NewScript(`
local user_key = KEYS[1]
local leases_key = KEYS[2]
local conns_key = KEYS[3]
local grace_key = KEYS[4]

local conn_id = ARGV[1]
local lease_expiry = tonumber(ARGV[2])
local conn_json = ARGV[3]
local now_rfc = ARGV[4]
local now_unix = tonumber(ARGV[5])
local sliding_ttl = tonumber(ARGV[6])

-- 1. Add/update lease score
redis.call('ZADD', leases_key, lease_expiry, conn_id)

-- 2. Store metadata JSON
redis.call('HSET', conns_key, conn_id, conn_json)

-- 3. Delete grace key if present (aborts disconnect grace period)
redis.call('DEL', grace_key)

-- 4. Prune any expired leases
redis.call('ZREMRANGEBYSCORE', leases_key, '-inf', '(' .. now_unix)

-- 5. Inspect remaining valid connections and evaluate aggregate status
local valid_conn_ids = redis.call('ZRANGE', leases_key, 0, -1)
local conn_count = #valid_conn_ids

local effective_status = 'offline'
if conn_count > 0 then
    effective_status = 'away'
    -- Check if any connection is 'online'
    for _, cid in ipairs(valid_conn_ids) do
        local cjson = redis.call('HGET', conns_key, cid)
        if cjson then
            local decoded = cjson
            -- fast pattern match for "state":"online"
            if string.find(decoded, '"state"%s*:%s*"online"') then
                effective_status = 'online'
                break
            end
        end
    end
end

-- Check manual preference override
local manual_pref = redis.call('HGET', user_key, 'manual_pref')
if manual_pref == 'away' and conn_count > 0 then
    effective_status = 'away'
end

-- 6. Update user hash
redis.call('HSET', user_key,
    'status', effective_status,
    'last_seen', now_rfc,
    'updated_at', now_rfc,
    'conn_count', tostring(conn_count)
)

-- 7. Refresh sliding TTL on keys
redis.call('EXPIRE', user_key, sliding_ttl)
redis.call('EXPIRE', leases_key, sliding_ttl)
redis.call('EXPIRE', conns_key, sliding_ttl)

return conn_count
`)

// heartbeatConnectionScript atomically:
// 1. Checks if connectionID exists in leases
// 2. Updates score in leases to (now + leaseDuration)
// 3. Updates last_heartbeat in conns JSON
// 4. Updates last_seen in user hash
// 5. Refreshes sliding TTL on user keys
//
// KEYS[1] = presence:u:{userID}
// KEYS[2] = presence:u:{userID}:leases
// KEYS[3] = presence:u:{userID}:conns
// ARGV[1] = connectionID (string)
// ARGV[2] = leaseExpiry (unix timestamp seconds)
// ARGV[3] = nowRFC3339 (string)
// ARGV[4] = slidingTTL (seconds)
var heartbeatConnectionScript = goredis.NewScript(`
local user_key = KEYS[1]
local leases_key = KEYS[2]
local conns_key = KEYS[3]

local conn_id = ARGV[1]
local lease_expiry = tonumber(ARGV[2])
local now_rfc = ARGV[3]
local sliding_ttl = tonumber(ARGV[4])

-- Check existence
local score = redis.call('ZSCORE', leases_key, conn_id)
if not score then
    return 0
end

-- Update lease expiry
redis.call('ZADD', leases_key, lease_expiry, conn_id)

-- Update last_heartbeat in conn metadata if exists
local conn_json = redis.call('HGET', conns_key, conn_id)
if conn_json then
    -- simple substitution for last_heartbeat
    local updated_json = string.gsub(conn_json, '"last_heartbeat"%s*:%s*"[^"]*"', '"last_heartbeat":"' .. now_rfc .. '"')
    redis.call('HSET', conns_key, conn_id, updated_json)
end

-- Update last_seen in user hash
redis.call('HSET', user_key, 'last_seen', now_rfc, 'updated_at', now_rfc)

-- Refresh sliding TTL
redis.call('EXPIRE', user_key, sliding_ttl)
redis.call('EXPIRE', leases_key, sliding_ttl)
redis.call('EXPIRE', conns_key, sliding_ttl)

return 1
`)

// removeConnectionScript atomically:
// 1. Removes connectionID from leases and conns
// 2. Prunes any other expired leases
// 3. Counts remaining active connections
// 4. If remaining == 0:
//   - Sets grace key with graceDuration TTL
//   - Updates user hash status to 'offline' (or 'disconnecting')
//
// 5. If remaining > 0:
//   - Recomputes aggregate status (online > away)
//   - Updates user hash
//
// 6. Returns remaining connection count
//
// KEYS[1] = presence:u:{userID}
// KEYS[2] = presence:u:{userID}:leases
// KEYS[3] = presence:u:{userID}:conns
// KEYS[4] = presence:u:{userID}:grace
// ARGV[1] = connectionID (string)
// ARGV[2] = nowRFC3339 (string)
// ARGV[3] = nowUnix (float/int)
// ARGV[4] = graceDuration (seconds)
// ARGV[5] = slidingTTL (seconds)
var removeConnectionScript = goredis.NewScript(`
local user_key = KEYS[1]
local leases_key = KEYS[2]
local conns_key = KEYS[3]
local grace_key = KEYS[4]

local conn_id = ARGV[1]
local now_rfc = ARGV[2]
local now_unix = tonumber(ARGV[3])
local grace_seconds = tonumber(ARGV[4])
local sliding_ttl = tonumber(ARGV[5])

-- Remove target connection
redis.call('ZREM', leases_key, conn_id)
redis.call('HDEL', conns_key, conn_id)

-- Prune any expired leases
redis.call('ZREMRANGEBYSCORE', leases_key, '-inf', '(' .. now_unix)

-- Count remaining
local valid_conn_ids = redis.call('ZRANGE', leases_key, 0, -1)
local remaining_count = #valid_conn_ids

if remaining_count == 0 then
    -- Start grace period
    redis.call('SET', grace_key, now_rfc, 'EX', grace_seconds)

    -- Update user hash status to offline
    redis.call('HSET', user_key,
        'status', 'offline',
        'last_seen', now_rfc,
        'updated_at', now_rfc,
        'conn_count', '0'
    )
    redis.call('EXPIRE', user_key, sliding_ttl)
else
    -- Recompute status
    local effective_status = 'away'
    for _, cid in ipairs(valid_conn_ids) do
        local cjson = redis.call('HGET', conns_key, cid)
        if cjson and string.find(cjson, '"state"%s*:%s*"online"') then
            effective_status = 'online'
            break
        end
    end

    local manual_pref = redis.call('HGET', user_key, 'manual_pref')
    if manual_pref == 'away' then
        effective_status = 'away'
    end

    redis.call('HSET', user_key,
        'status', effective_status,
        'last_seen', now_rfc,
        'updated_at', now_rfc,
        'conn_count', tostring(remaining_count)
    )
    redis.call('EXPIRE', user_key, sliding_ttl)
    redis.call('EXPIRE', leases_key, sliding_ttl)
    redis.call('EXPIRE', conns_key, sliding_ttl)
end

return remaining_count
`)

// setManualPreferenceScript updates manual_pref and immediately recomputes aggregate status.
// KEYS[1] = presence:u:{userID}
// KEYS[2] = presence:u:{userID}:leases
// KEYS[3] = presence:u:{userID}:conns
// ARGV[1] = pref ("none", "away")
// ARGV[2] = nowRFC3339
// ARGV[3] = nowUnix
// ARGV[4] = slidingTTL
var setManualPreferenceScript = goredis.NewScript(`
local user_key = KEYS[1]
local leases_key = KEYS[2]
local conns_key = KEYS[3]

local pref = ARGV[1]
local now_rfc = ARGV[2]
local now_unix = tonumber(ARGV[3])
local sliding_ttl = tonumber(ARGV[4])

redis.call('HSET', user_key, 'manual_pref', pref, 'updated_at', now_rfc)

-- Prune expired
redis.call('ZREMRANGEBYSCORE', leases_key, '-inf', '(' .. now_unix)

local valid_conn_ids = redis.call('ZRANGE', leases_key, 0, -1)
local remaining_count = #valid_conn_ids

local effective_status = 'offline'
if remaining_count > 0 then
    if pref == 'away' then
        effective_status = 'away'
    else
        effective_status = 'away'
        for _, cid in ipairs(valid_conn_ids) do
            local cjson = redis.call('HGET', conns_key, cid)
            if cjson and string.find(cjson, '"state"%s*:%s*"online"') then
                effective_status = 'online'
                break
            end
        end
    end
end

redis.call('HSET', user_key,
    'status', effective_status,
    'conn_count', tostring(remaining_count)
)
redis.call('EXPIRE', user_key, sliding_ttl)

return effective_status
`)

// ─── Implementation ──────────────────────────────────────────────────────────

const (
	defaultSlidingTTL = 60 // 60 seconds sliding TTL for active keys
)

func (r *redisPresenceRepository) RegisterConnection(ctx context.Context, conn *domain.PresenceConnection, leaseDuration time.Duration) error {
	if conn == nil || conn.UserID == uuid.Nil || conn.ConnectionID == uuid.Nil {
		return presenceerrors.NewInvalidConnectionID()
	}

	now := time.Now().UTC()
	leaseExpiry := float64(now.Add(leaseDuration).Unix())
	nowUnix := float64(now.Unix())
	nowRFC := now.Format(time.RFC3339)

	dto := connectionMetadataDTO{
		ConnID:        conn.ConnectionID.String(),
		UserID:        conn.UserID.String(),
		SessionID:     conn.SessionID.String(),
		DeviceID:      conn.DeviceID.String(),
		NodeID:        conn.NodeID,
		Platform:      conn.Platform,
		State:         string(conn.State),
		ConnectedAt:   conn.ConnectedAt.UTC().Format(time.RFC3339),
		LastHeartbeat: nowRFC,
	}

	jsonBytes, err := json.Marshal(dto)
	if err != nil {
		return fmt.Errorf("failed to marshal connection metadata: %w", err)
	}

	keys := []string{
		userKey(conn.UserID),
		userLeasesKey(conn.UserID),
		userConnsKey(conn.UserID),
		userGraceKey(conn.UserID),
	}

	args := []interface{}{
		conn.ConnectionID.String(),
		leaseExpiry,
		string(jsonBytes),
		nowRFC,
		nowUnix,
		defaultSlidingTTL,
	}

	err = registerConnectionScript.Run(ctx, r.redis.Client(), keys, args...).Err()
	if err != nil {
		return fmt.Errorf("redis RegisterConnection failed: %w", err)
	}

	return nil
}

func (r *redisPresenceRepository) RecordHeartbeat(ctx context.Context, userID, connectionID uuid.UUID, leaseDuration time.Duration) error {
	if userID == uuid.Nil {
		return presenceerrors.NewInvalidUserID()
	}
	if connectionID == uuid.Nil {
		return presenceerrors.NewInvalidConnectionID()
	}

	now := time.Now().UTC()
	leaseExpiry := float64(now.Add(leaseDuration).Unix())
	nowRFC := now.Format(time.RFC3339)

	keys := []string{
		userKey(userID),
		userLeasesKey(userID),
		userConnsKey(userID),
	}

	args := []interface{}{
		connectionID.String(),
		leaseExpiry,
		nowRFC,
		defaultSlidingTTL,
	}

	res, err := heartbeatConnectionScript.Run(ctx, r.redis.Client(), keys, args...).Result()
	if err != nil {
		return fmt.Errorf("redis RecordHeartbeat failed: %w", err)
	}

	count, ok := res.(int64)
	if !ok || count == 0 {
		return presenceerrors.NewConnectionNotFound()
	}

	return nil
}

func (r *redisPresenceRepository) RemoveConnection(ctx context.Context, userID, connectionID uuid.UUID, graceDuration time.Duration) (int, error) {
	if userID == uuid.Nil {
		return 0, presenceerrors.NewInvalidUserID()
	}
	if connectionID == uuid.Nil {
		return 0, presenceerrors.NewInvalidConnectionID()
	}

	now := time.Now().UTC()
	nowRFC := now.Format(time.RFC3339)
	nowUnix := float64(now.Unix())
	graceSeconds := int64(graceDuration.Seconds())
	if graceSeconds <= 0 {
		graceSeconds = 10
	}

	keys := []string{
		userKey(userID),
		userLeasesKey(userID),
		userConnsKey(userID),
		userGraceKey(userID),
	}

	args := []interface{}{
		connectionID.String(),
		nowRFC,
		nowUnix,
		graceSeconds,
		defaultSlidingTTL,
	}

	res, err := removeConnectionScript.Run(ctx, r.redis.Client(), keys, args...).Result()
	if err != nil {
		return 0, fmt.Errorf("redis RemoveConnection failed: %w", err)
	}

	remaining, _ := res.(int64)
	return int(remaining), nil
}

func (r *redisPresenceRepository) GetAggregatePresence(ctx context.Context, userID uuid.UUID) (*domain.UserPresence, error) {
	if userID == uuid.Nil {
		return nil, presenceerrors.NewInvalidUserID()
	}

	key := userKey(userID)
	res, err := r.redis.Client().HMGet(ctx, key, "status", "last_seen", "updated_at", "manual_pref", "conn_count").Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("redis GetAggregatePresence failed: %w", err)
	}

	// If key does not exist or status is nil, return nil (offline / not in cache)
	if len(res) < 5 || res[0] == nil {
		return nil, nil
	}

	return parseUserPresenceFromFields(userID, res)
}

// GetAggregatePresenceBatch implements the pipelined bulk read across multiple user IDs.
// It executes a single pipeline of HMGET commands in one network round-trip.
func (r *redisPresenceRepository) GetAggregatePresenceBatch(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]*domain.UserPresence, error) {
	if len(userIDs) == 0 {
		return make(map[uuid.UUID]*domain.UserPresence), nil
	}

	pipe := r.redis.Client().Pipeline()
	cmds := make(map[uuid.UUID]*goredis.SliceCmd, len(userIDs))

	for _, uid := range userIDs {
		key := userKey(uid)
		cmds[uid] = pipe.HMGet(ctx, key, "status", "last_seen", "updated_at", "manual_pref", "conn_count")
	}

	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, goredis.Nil) {
		return nil, fmt.Errorf("redis pipeline Exec failed in GetAggregatePresenceBatch: %w", err)
	}

	results := make(map[uuid.UUID]*domain.UserPresence, len(userIDs))
	for _, uid := range userIDs {
		cmd := cmds[uid]
		sliceRes, cmdErr := cmd.Result()
		if cmdErr != nil || len(sliceRes) < 5 || sliceRes[0] == nil {
			// Missing or error on this key -> nil entry (offline / not in redis)
			results[uid] = nil
			continue
		}

		presence, parseErr := parseUserPresenceFromFields(uid, sliceRes)
		if parseErr != nil {
			results[uid] = nil
			continue
		}
		results[uid] = presence
	}

	return results, nil
}

func parseUserPresenceFromFields(userID uuid.UUID, fields []interface{}) (*domain.UserPresence, error) {
	statusStr, _ := fields[0].(string)
	if statusStr == "" {
		return nil, nil
	}

	var lastSeenAt *time.Time
	if lastSeenStr, ok := fields[1].(string); ok && lastSeenStr != "" {
		if t, err := time.Parse(time.RFC3339, lastSeenStr); err == nil {
			lastSeenAt = &t
		}
	}

	updatedAt := time.Now().UTC()
	if updatedStr, ok := fields[2].(string); ok && updatedStr != "" {
		if t, err := time.Parse(time.RFC3339, updatedStr); err == nil {
			updatedAt = t
		}
	}

	manualPref := domain.PresencePreferenceNone
	if prefStr, ok := fields[3].(string); ok && prefStr != "" {
		manualPref = domain.PresencePreference(prefStr)
	}

	connCount := 0
	if ccStr, ok := fields[4].(string); ok && ccStr != "" {
		if c, err := strconv.Atoi(ccStr); err == nil {
			connCount = c
		}
	}

	return &domain.UserPresence{
		UserID:            userID,
		Status:            domain.PresenceState(statusStr),
		LastSeenAt:        lastSeenAt,
		UpdatedAt:         updatedAt,
		ManualPreference:  manualPref,
		ActiveConnections: connCount,
	}, nil
}

func (r *redisPresenceRepository) SetManualPreference(ctx context.Context, userID uuid.UUID, pref domain.PresencePreference) error {
	if userID == uuid.Nil {
		return presenceerrors.NewInvalidUserID()
	}

	now := time.Now().UTC()
	nowRFC := now.Format(time.RFC3339)
	nowUnix := float64(now.Unix())

	keys := []string{
		userKey(userID),
		userLeasesKey(userID),
		userConnsKey(userID),
	}

	args := []interface{}{
		string(pref),
		nowRFC,
		nowUnix,
		defaultSlidingTTL,
	}

	err := setManualPreferenceScript.Run(ctx, r.redis.Client(), keys, args...).Err()
	if err != nil {
		return fmt.Errorf("redis SetManualPreference failed: %w", err)
	}

	return nil
}

func (r *redisPresenceRepository) GetConnectionMetadata(ctx context.Context, userID, connectionID uuid.UUID) (*domain.PresenceConnection, error) {
	if userID == uuid.Nil {
		return nil, presenceerrors.NewInvalidUserID()
	}
	if connectionID == uuid.Nil {
		return nil, presenceerrors.NewInvalidConnectionID()
	}

	val, err := r.redis.Client().HGet(ctx, userConnsKey(userID), connectionID.String()).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, presenceerrors.NewConnectionNotFound()
		}
		return nil, fmt.Errorf("redis GetConnectionMetadata failed: %w", err)
	}

	var dto connectionMetadataDTO
	if err := json.Unmarshal([]byte(val), &dto); err != nil {
		return nil, fmt.Errorf("failed to unmarshal connection metadata: %w", err)
	}

	return mapDtoToPresenceConnection(dto)
}

func (r *redisPresenceRepository) ListUserConnections(ctx context.Context, userID uuid.UUID) ([]*domain.PresenceConnection, error) {
	if userID == uuid.Nil {
		return nil, presenceerrors.NewInvalidUserID()
	}

	vals, err := r.redis.Client().HGetAll(ctx, userConnsKey(userID)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis ListUserConnections failed: %w", err)
	}

	connections := make([]*domain.PresenceConnection, 0, len(vals))
	for _, raw := range vals {
		var dto connectionMetadataDTO
		if err := json.Unmarshal([]byte(raw), &dto); err != nil {
			continue
		}
		conn, err := mapDtoToPresenceConnection(dto)
		if err == nil {
			connections = append(connections, conn)
		}
	}

	return connections, nil
}

func mapDtoToPresenceConnection(dto connectionMetadataDTO) (*domain.PresenceConnection, error) {
	cid, err := uuid.Parse(dto.ConnID)
	if err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(dto.UserID)
	if err != nil {
		return nil, err
	}
	sid, _ := uuid.Parse(dto.SessionID)
	did, _ := uuid.Parse(dto.DeviceID)

	var connectedAt time.Time
	if t, err := time.Parse(time.RFC3339, dto.ConnectedAt); err == nil {
		connectedAt = t
	}

	var lastHeartbeatAt time.Time
	if t, err := time.Parse(time.RFC3339, dto.LastHeartbeat); err == nil {
		lastHeartbeatAt = t
	}

	return &domain.PresenceConnection{
		ConnectionID:    cid,
		UserID:          uid,
		SessionID:       sid,
		DeviceID:        did,
		NodeID:          dto.NodeID,
		Platform:        dto.Platform,
		State:           domain.PresenceState(dto.State),
		ConnectedAt:     connectedAt,
		LastHeartbeatAt: lastHeartbeatAt,
	}, nil
}

func (r *redisPresenceRepository) IsInGracePeriod(ctx context.Context, userID uuid.UUID) (bool, error) {
	if userID == uuid.Nil {
		return false, presenceerrors.NewInvalidUserID()
	}

	exists, err := r.redis.Client().Exists(ctx, userGraceKey(userID)).Result()
	if err != nil {
		return false, fmt.Errorf("redis IsInGracePeriod failed: %w", err)
	}

	return exists > 0, nil
}

func (r *redisPresenceRepository) TrackNodeConnection(ctx context.Context, nodeID string, userID, connectionID uuid.UUID) error {
	if nodeID == "" {
		return errors.New("nodeID cannot be empty")
	}
	member := fmt.Sprintf("%s:%s", connectionID.String(), userID.String())
	err := r.redis.Client().SAdd(ctx, nodeConnsKey(nodeID), member).Err()
	if err != nil {
		return fmt.Errorf("redis TrackNodeConnection failed: %w", err)
	}
	return nil
}

func (r *redisPresenceRepository) UntrackNodeConnection(ctx context.Context, nodeID string, userID, connectionID uuid.UUID) error {
	if nodeID == "" {
		return errors.New("nodeID cannot be empty")
	}
	member := fmt.Sprintf("%s:%s", connectionID.String(), userID.String())
	err := r.redis.Client().SRem(ctx, nodeConnsKey(nodeID), member).Err()
	if err != nil {
		return fmt.Errorf("redis UntrackNodeConnection failed: %w", err)
	}
	return nil
}

func (r *redisPresenceRepository) ListClusterNodeConnections(ctx context.Context, nodeID string) ([]string, error) {
	if nodeID == "" {
		return nil, errors.New("nodeID cannot be empty")
	}
	members, err := r.redis.Client().SMembers(ctx, nodeConnsKey(nodeID)).Result()
	if err != nil {
		return nil, fmt.Errorf("redis ListClusterNodeConnections failed: %w", err)
	}
	return members, nil
}
