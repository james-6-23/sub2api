package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

// userGroupRPMOverrideResolver 解析 (用户, 分组) 上的 RPM 覆盖值。
// 与 userGroupRateResolver 类似：进程内缓存 + singleflight，避免高频请求穿透到 DB。
// 缓存的 sentinel：nil 表示已查过 DB 但无覆盖；*int 表示有覆盖（含 0=不限制）。
type userGroupRPMOverrideResolver struct {
	repo     UserGroupRateRepository
	cache    *gocache.Cache
	cacheTTL time.Duration
	sf       *singleflight.Group
}

// rpmOverrideCacheValue 包一层避免 gocache.Get 把 nil 当成"未命中"。
type rpmOverrideCacheValue struct {
	value *int
}

func newUserGroupRPMOverrideResolver(repo UserGroupRateRepository, cacheTTL time.Duration) *userGroupRPMOverrideResolver {
	if cacheTTL <= 0 {
		cacheTTL = defaultUserGroupRateCacheTTL
	}
	return &userGroupRPMOverrideResolver{
		repo:     repo,
		cache:    gocache.New(cacheTTL, time.Minute),
		cacheTTL: cacheTTL,
		sf:       &singleflight.Group{},
	}
}

// NewUserGroupRPMOverrideResolverForBilling 用于 wire 注入到 BillingCacheService。
// 返回 groupRPMOverrideResolver 接口（由 BillingCacheService 定义），内部实现复用 userGroupRateResolver 的缓存常量。
func NewUserGroupRPMOverrideResolverForBilling(repo UserGroupRateRepository, cfg *config.Config) groupRPMOverrideResolver {
	return newUserGroupRPMOverrideResolver(repo, resolveUserGroupRateCacheTTL(cfg))
}

// Resolve 返回 (user, group) 上的 RPM 覆盖：
//   - nil  → 无覆盖（使用 group.RPMLimit）
//   - *int → 有覆盖（含 0 表示该用户在此分组不限流）
func (r *userGroupRPMOverrideResolver) Resolve(ctx context.Context, userID, groupID int64) *int {
	if r == nil || r.repo == nil || userID <= 0 || groupID <= 0 {
		return nil
	}

	key := fmt.Sprintf("%d:%d", userID, groupID)
	if cached, ok := r.cache.Get(key); ok {
		if wrap, castOK := cached.(*rpmOverrideCacheValue); castOK {
			return wrap.value
		}
	}

	value, err, _ := r.sf.Do(key, func() (any, error) {
		if cached, ok := r.cache.Get(key); ok {
			if wrap, castOK := cached.(*rpmOverrideCacheValue); castOK {
				return wrap.value, nil
			}
		}
		override, repoErr := r.repo.GetRPMOverrideByUserAndGroup(ctx, userID, groupID)
		if repoErr != nil {
			return nil, repoErr
		}
		r.cache.Set(key, &rpmOverrideCacheValue{value: override}, r.cacheTTL)
		return override, nil
	})
	if err != nil {
		logger.LegacyPrintf(
			"service.billing_cache",
			"get user group rpm override failed, fallback to group default: user=%d group=%d err=%v",
			userID, groupID, err,
		)
		return nil
	}
	if v, ok := value.(*int); ok {
		return v
	}
	return nil
}
