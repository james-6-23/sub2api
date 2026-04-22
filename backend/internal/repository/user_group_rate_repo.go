package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type userGroupRateRepository struct {
	sql sqlExecutor
}

// NewUserGroupRateRepository 创建用户专属分组倍率仓储
func NewUserGroupRateRepository(sqlDB *sql.DB) service.UserGroupRateRepository {
	return &userGroupRateRepository{sql: sqlDB}
}

// GetByUserID 获取用户的所有专属分组 rate_multiplier（仅返回非 NULL 的条目）
func (r *userGroupRateRepository) GetByUserID(ctx context.Context, userID int64) (map[int64]float64, error) {
	query := `SELECT group_id, rate_multiplier FROM user_group_rate_multipliers WHERE user_id = $1 AND rate_multiplier IS NOT NULL`
	rows, err := r.sql.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64]float64)
	for rows.Next() {
		var groupID int64
		var rate float64
		if err := rows.Scan(&groupID, &rate); err != nil {
			return nil, err
		}
		result[groupID] = rate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetByUserIDs 批量获取多个用户的专属分组 rate_multiplier。
func (r *userGroupRateRepository) GetByUserIDs(ctx context.Context, userIDs []int64) (map[int64]map[int64]float64, error) {
	result := make(map[int64]map[int64]float64, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	uniqueIDs := make([]int64, 0, len(userIDs))
	seen := make(map[int64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		uniqueIDs = append(uniqueIDs, userID)
		result[userID] = make(map[int64]float64)
	}
	if len(uniqueIDs) == 0 {
		return result, nil
	}

	rows, err := r.sql.QueryContext(ctx, `
		SELECT user_id, group_id, rate_multiplier
		FROM user_group_rate_multipliers
		WHERE user_id = ANY($1) AND rate_multiplier IS NOT NULL
	`, pq.Array(uniqueIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var userID int64
		var groupID int64
		var rate float64
		if err := rows.Scan(&userID, &groupID, &rate); err != nil {
			return nil, err
		}
		if _, ok := result[userID]; !ok {
			result[userID] = make(map[int64]float64)
		}
		result[userID][groupID] = rate
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetByGroupID 获取指定分组下所有用户的专属配置（rate 与 rpm_override 任一非 NULL 即返回）
func (r *userGroupRateRepository) GetByGroupID(ctx context.Context, groupID int64) ([]service.UserGroupRateEntry, error) {
	query := `
		SELECT ugr.user_id, u.username, u.email, COALESCE(u.notes, ''), u.status, ugr.rate_multiplier, ugr.rpm_override
		FROM user_group_rate_multipliers ugr
		JOIN users u ON u.id = ugr.user_id AND u.deleted_at IS NULL
		WHERE ugr.group_id = $1
		ORDER BY ugr.user_id
	`
	rows, err := r.sql.QueryContext(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []service.UserGroupRateEntry
	for rows.Next() {
		var entry service.UserGroupRateEntry
		var rate sql.NullFloat64
		var rpm sql.NullInt32
		if err := rows.Scan(&entry.UserID, &entry.UserName, &entry.UserEmail, &entry.UserNotes, &entry.UserStatus, &rate, &rpm); err != nil {
			return nil, err
		}
		if rate.Valid {
			v := rate.Float64
			entry.RateMultiplier = &v
		}
		if rpm.Valid {
			v := int(rpm.Int32)
			entry.RPMOverride = &v
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetByUserAndGroup 获取用户在特定分组的专属 rate_multiplier
func (r *userGroupRateRepository) GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error) {
	query := `SELECT rate_multiplier FROM user_group_rate_multipliers WHERE user_id = $1 AND group_id = $2`
	var rate sql.NullFloat64
	err := scanSingleRow(ctx, r.sql, query, []any{userID, groupID}, &rate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !rate.Valid {
		return nil, nil
	}
	v := rate.Float64
	return &v, nil
}

// GetRPMOverrideByUserAndGroup 获取用户在特定分组的专属 RPM 上限
func (r *userGroupRateRepository) GetRPMOverrideByUserAndGroup(ctx context.Context, userID, groupID int64) (*int, error) {
	query := `SELECT rpm_override FROM user_group_rate_multipliers WHERE user_id = $1 AND group_id = $2`
	var rpm sql.NullInt32
	err := scanSingleRow(ctx, r.sql, query, []any{userID, groupID}, &rpm)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !rpm.Valid {
		return nil, nil
	}
	v := int(rpm.Int32)
	return &v, nil
}

// SyncUserGroupRates 同步用户的分组专属 rate_multiplier。
// rates: map[groupID]*rateMultiplier。
//   - 值为 nil  → 清空 rate_multiplier 列；若该行 rpm_override 也为 NULL 则整行删除。
//   - 值非 nil → upsert rate_multiplier，保留现有 rpm_override。
func (r *userGroupRateRepository) SyncUserGroupRates(ctx context.Context, userID int64, rates map[int64]*float64) error {
	if len(rates) == 0 {
		// 传入空 map：清空该用户所有 rate_multiplier，同时删除 rpm_override 也为 NULL 的行
		_, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW()
			WHERE user_id = $1
		`, userID)
		if err != nil {
			return err
		}
		_, err = r.sql.ExecContext(ctx,
			`DELETE FROM user_group_rate_multipliers WHERE user_id = $1 AND rate_multiplier IS NULL AND rpm_override IS NULL`,
			userID)
		return err
	}

	var clearGroupIDs []int64
	upsertGroupIDs := make([]int64, 0, len(rates))
	upsertRates := make([]float64, 0, len(rates))
	for groupID, rate := range rates {
		if rate == nil {
			clearGroupIDs = append(clearGroupIDs, groupID)
		} else {
			upsertGroupIDs = append(upsertGroupIDs, groupID)
			upsertRates = append(upsertRates, *rate)
		}
	}

	if len(clearGroupIDs) > 0 {
		if _, err := r.sql.ExecContext(ctx, `
			UPDATE user_group_rate_multipliers
			SET rate_multiplier = NULL, updated_at = NOW()
			WHERE user_id = $1 AND group_id = ANY($2)
		`, userID, pq.Array(clearGroupIDs)); err != nil {
			return err
		}
		if _, err := r.sql.ExecContext(ctx, `
			DELETE FROM user_group_rate_multipliers
			WHERE user_id = $1 AND group_id = ANY($2) AND rate_multiplier IS NULL AND rpm_override IS NULL
		`, userID, pq.Array(clearGroupIDs)); err != nil {
			return err
		}
	}

	if len(upsertGroupIDs) > 0 {
		now := time.Now()
		_, err := r.sql.ExecContext(ctx, `
			INSERT INTO user_group_rate_multipliers (user_id, group_id, rate_multiplier, created_at, updated_at)
			SELECT
				$1::bigint,
				data.group_id,
				data.rate_multiplier,
				$2::timestamptz,
				$2::timestamptz
			FROM unnest($3::bigint[], $4::double precision[]) AS data(group_id, rate_multiplier)
			ON CONFLICT (user_id, group_id)
			DO UPDATE SET
				rate_multiplier = EXCLUDED.rate_multiplier,
				updated_at = EXCLUDED.updated_at
		`, userID, now, pq.Array(upsertGroupIDs), pq.Array(upsertRates))
		if err != nil {
			return err
		}
	}

	return nil
}

// SyncGroupRateMultipliers 批量同步分组下的用户 rate_multiplier。
// 只操作 rate_multiplier 列：
//   - entries 中的 (user, group) 行：upsert rate_multiplier（保留 rpm_override）
//   - 不在 entries 中的 (user, group) 行：清空 rate_multiplier；若 rpm_override 也为 NULL 则删除整行
func (r *userGroupRateRepository) SyncGroupRateMultipliers(ctx context.Context, groupID int64, entries []service.GroupRateMultiplierInput) error {
	now := time.Now()

	keepUserIDs := make([]int64, 0, len(entries))
	for _, e := range entries {
		keepUserIDs = append(keepUserIDs, e.UserID)
	}

	// 清空未保留用户的 rate_multiplier
	if _, err := r.sql.ExecContext(ctx, `
		UPDATE user_group_rate_multipliers
		SET rate_multiplier = NULL, updated_at = $1
		WHERE group_id = $2 AND (CARDINALITY($3::bigint[]) = 0 OR user_id <> ALL($3::bigint[]))
	`, now, groupID, pq.Array(keepUserIDs)); err != nil {
		return err
	}
	// 删除 rate、rpm 都为 NULL 的空行
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM user_group_rate_multipliers
		WHERE group_id = $1 AND rate_multiplier IS NULL AND rpm_override IS NULL
	`, groupID); err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	userIDs := make([]int64, len(entries))
	rates := make([]float64, len(entries))
	for i, e := range entries {
		userIDs[i] = e.UserID
		rates[i] = e.RateMultiplier
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO user_group_rate_multipliers (user_id, group_id, rate_multiplier, created_at, updated_at)
		SELECT data.user_id, $1::bigint, data.rate_multiplier, $2::timestamptz, $2::timestamptz
		FROM unnest($3::bigint[], $4::double precision[]) AS data(user_id, rate_multiplier)
		ON CONFLICT (user_id, group_id)
		DO UPDATE SET rate_multiplier = EXCLUDED.rate_multiplier, updated_at = EXCLUDED.updated_at
	`, groupID, now, pq.Array(userIDs), pq.Array(rates))
	return err
}

// SyncGroupRPMOverrides 批量同步分组下的用户 rpm_override。
// 只操作 rpm_override 列：
//   - entries 中的 (user, group) 行：upsert rpm_override（保留 rate_multiplier）
//   - 不在 entries 中的 (user, group) 行：清空 rpm_override；若 rate_multiplier 也为 NULL 则删除整行
func (r *userGroupRateRepository) SyncGroupRPMOverrides(ctx context.Context, groupID int64, entries []service.GroupRPMOverrideInput) error {
	now := time.Now()

	keepUserIDs := make([]int64, 0, len(entries))
	for _, e := range entries {
		keepUserIDs = append(keepUserIDs, e.UserID)
	}

	if _, err := r.sql.ExecContext(ctx, `
		UPDATE user_group_rate_multipliers
		SET rpm_override = NULL, updated_at = $1
		WHERE group_id = $2 AND (CARDINALITY($3::bigint[]) = 0 OR user_id <> ALL($3::bigint[]))
	`, now, groupID, pq.Array(keepUserIDs)); err != nil {
		return err
	}
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM user_group_rate_multipliers
		WHERE group_id = $1 AND rate_multiplier IS NULL AND rpm_override IS NULL
	`, groupID); err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	userIDs := make([]int64, len(entries))
	rpms := make([]int64, len(entries))
	for i, e := range entries {
		userIDs[i] = e.UserID
		rpms[i] = int64(e.RPMOverride)
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO user_group_rate_multipliers (user_id, group_id, rpm_override, created_at, updated_at)
		SELECT data.user_id, $1::bigint, data.rpm_override::integer, $2::timestamptz, $2::timestamptz
		FROM unnest($3::bigint[], $4::bigint[]) AS data(user_id, rpm_override)
		ON CONFLICT (user_id, group_id)
		DO UPDATE SET rpm_override = EXCLUDED.rpm_override, updated_at = EXCLUDED.updated_at
	`, groupID, now, pq.Array(userIDs), pq.Array(rpms))
	return err
}

// ClearGroupRPMOverrides 清空分组所有用户的 rpm_override（保留 rate_multiplier）
func (r *userGroupRateRepository) ClearGroupRPMOverrides(ctx context.Context, groupID int64) error {
	if _, err := r.sql.ExecContext(ctx, `
		UPDATE user_group_rate_multipliers
		SET rpm_override = NULL, updated_at = NOW()
		WHERE group_id = $1
	`, groupID); err != nil {
		return err
	}
	_, err := r.sql.ExecContext(ctx, `
		DELETE FROM user_group_rate_multipliers
		WHERE group_id = $1 AND rate_multiplier IS NULL AND rpm_override IS NULL
	`, groupID)
	return err
}

// DeleteByGroupID 删除指定分组的所有用户专属倍率
func (r *userGroupRateRepository) DeleteByGroupID(ctx context.Context, groupID int64) error {
	_, err := r.sql.ExecContext(ctx, `DELETE FROM user_group_rate_multipliers WHERE group_id = $1`, groupID)
	return err
}

// DeleteByUserID 删除指定用户的所有专属倍率
func (r *userGroupRateRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	_, err := r.sql.ExecContext(ctx, `DELETE FROM user_group_rate_multipliers WHERE user_id = $1`, userID)
	return err
}
