package service

import "context"

// RPMCache RPM 计数器缓存接口
// 用于 Anthropic OAuth/SetupToken 账号的每分钟请求数限制
type RPMCache interface {
	// IncrementRPM 原子递增并返回当前分钟的计数
	// 使用 Redis 服务器时间确定 minute key，避免多实例时钟偏差
	IncrementRPM(ctx context.Context, accountID int64) (count int, err error)

	// GetRPM 获取当前分钟的 RPM 计数
	GetRPM(ctx context.Context, accountID int64) (count int, err error)

	// GetRPMBatch 批量获取多个账号的 RPM 计数（使用 Pipeline）
	GetRPMBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error)
}

// UserRPMCache 用户侧 RPM 计数器缓存
//
// 设计要点：
//   - 覆盖两种限流粒度：
//     1) 用户在单个分组内：按 user_id + group_id 聚合，键 rpm:ug:{userID}:{groupID}:{minute}，
//        用于 Group.RPMLimit 限额（由管理员按分组配置）。
//     2) 用户跨所有分组：按 user_id 聚合，键 rpm:u:{userID}:{minute}，
//        用于 User.RPMLimit 限额（由管理员在用户上配置）。
//   - 两类计数彼此独立：任一超限即可触发 429；不会相互影响。
//   - 通过按 user 聚合（而不是按 api_key），杜绝“同一用户创建多个 Key 绕过 RPM 限制”的路径。
//   - 计数窗口：固定分钟窗口（基于 Redis 服务端时间，避免多实例时钟偏差）。
type UserRPMCache interface {
	// IncrementUserGroupRPM 原子递增指定（用户, 分组）组合在当前分钟的计数，并返回新的计数值。
	IncrementUserGroupRPM(ctx context.Context, userID, groupID int64) (count int, err error)

	// GetUserGroupRPM 获取（用户, 分组）当前分钟的计数；不存在时返回 0。
	GetUserGroupRPM(ctx context.Context, userID, groupID int64) (count int, err error)

	// IncrementUserRPM 原子递增用户跨所有分组在当前分钟的计数，并返回新的计数值。
	IncrementUserRPM(ctx context.Context, userID int64) (count int, err error)

	// GetUserRPM 获取用户当前分钟的跨分组计数；不存在时返回 0。
	GetUserRPM(ctx context.Context, userID int64) (count int, err error)
}
