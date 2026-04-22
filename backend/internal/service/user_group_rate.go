package service

import "context"

// UserGroupRateEntry 分组下用户专属倍率条目
type UserGroupRateEntry struct {
	UserID         int64    `json:"user_id"`
	UserName       string   `json:"user_name"`
	UserEmail      string   `json:"user_email"`
	UserNotes      string   `json:"user_notes"`
	UserStatus     string   `json:"user_status"`
	RateMultiplier *float64 `json:"rate_multiplier,omitempty"`
	RPMOverride    *int     `json:"rpm_override,omitempty"`
}

// GroupRateMultiplierInput 批量设置分组费率倍率的输入条目
type GroupRateMultiplierInput struct {
	UserID         int64   `json:"user_id"`
	RateMultiplier float64 `json:"rate_multiplier"`
}

// GroupRPMOverrideInput 批量设置分组 RPM 覆盖的输入条目
type GroupRPMOverrideInput struct {
	UserID      int64 `json:"user_id"`
	RPMOverride int   `json:"rpm_override"`
}

// UserGroupRateRepository 用户专属分组倍率仓储接口
// 允许管理员为特定用户设置分组的专属计费倍率和 RPM 上限，覆盖分组默认值
type UserGroupRateRepository interface {
	// GetByUserID 获取用户的所有专属分组倍率
	// 返回 map[groupID]rateMultiplier（仅包含 rate 非 NULL 的条目）
	GetByUserID(ctx context.Context, userID int64) (map[int64]float64, error)

	// GetByUserIDs 批量获取多个用户的专属分组倍率
	GetByUserIDs(ctx context.Context, userIDs []int64) (map[int64]map[int64]float64, error)

	// GetByUserAndGroup 获取用户在特定分组的专属倍率
	// 如果未设置专属倍率（或 rate_multiplier 为 NULL），返回 nil
	GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error)

	// GetRPMOverrideByUserAndGroup 获取用户在特定分组的专属 RPM 上限
	// 返回 nil 表示无覆盖（使用分组默认）；返回 *int 指针（0 表示该用户在此分组不限流）
	GetRPMOverrideByUserAndGroup(ctx context.Context, userID, groupID int64) (*int, error)

	// GetByGroupID 获取指定分组下所有用户的专属倍率
	GetByGroupID(ctx context.Context, groupID int64) ([]UserGroupRateEntry, error)

	// SyncUserGroupRates 同步用户的分组专属倍率
	// rates: map[groupID]*rateMultiplier，nil 表示删除该分组的 rate_multiplier（但保留 rpm_override 行）
	SyncUserGroupRates(ctx context.Context, userID int64, rates map[int64]*float64) error

	// SyncGroupRateMultipliers 批量同步分组下用户的 rate_multiplier（替换整组 rate 数据）。
	// 仅操作 rate_multiplier 列：不在 entries 中的 (user, group) 行会被清空 rate_multiplier；
	// 行本身会保留以维持 rpm_override（若存在）。
	SyncGroupRateMultipliers(ctx context.Context, groupID int64, entries []GroupRateMultiplierInput) error

	// SyncGroupRPMOverrides 批量同步分组下用户的 rpm_override（替换整组 rpm 数据）。
	// 仅操作 rpm_override 列：不在 entries 中的 (user, group) 行会被清空 rpm_override；
	// 行本身会保留以维持 rate_multiplier（若存在）。
	SyncGroupRPMOverrides(ctx context.Context, groupID int64, entries []GroupRPMOverrideInput) error

	// DeleteByGroupID 删除指定分组的所有用户专属配置（分组删除时调用）
	DeleteByGroupID(ctx context.Context, groupID int64) error

	// DeleteByUserID 删除指定用户的所有专属配置（用户删除时调用）
	DeleteByUserID(ctx context.Context, userID int64) error

	// ClearGroupRPMOverrides 清空指定分组的所有 RPM 覆盖（保留 rate_multiplier）
	ClearGroupRPMOverrides(ctx context.Context, groupID int64) error
}
