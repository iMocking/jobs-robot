package middleware

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// RateLimit 是基于 Redis 的固定窗口限流：同一 key 在 window 内最多 limit 次。
//
// 键形如 <keyPrefix>ratelimit:<keyParts>:<ip>，keyParts 用于区分不同接口
// （例如 "auth:login"），标识维度为客户端 IP。
//
// 取舍：
//   - 用固定窗口而不是滑动窗口/令牌桶：一条 INCR 就能完成计数，无额外内存与定时任务，
//     代价是窗口边界处最多出现 2 倍突发；对登录、注册、发验证码这类接口足够，
//     真要严格削峰应在网关层做。
//   - **Redis 出错时放行（fail open）**：限流是保护性组件，不能因为它故障就让登录注册整体不可用；
//     故障只记 warn 日志，由监控告警兜底。
//   - limit<=0 视为不限流，直接放行，连 Redis 都不访问。
func RateLimit(rdb redis.UniversalClient, keyPrefix string, keyParts string, limit int, window time.Duration) gin.HandlerFunc {
	if limit <= 0 || window <= 0 || rdb == nil {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		ctx := c.Request.Context()
		key := keyPrefix + "ratelimit:" + keyParts + ":" + ClientIP(c)

		count, ttl, err := incrWindow(ctx, rdb, key, window)
		if err != nil {
			// fail open：放行并告警，绝不因为限流组件故障阻断业务。
			slog.Warn("限流组件异常，本次请求放行",
				"request_id", requestIDOf(c),
				"key", key,
				"path", c.Request.URL.Path,
				"error", err,
			)
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remainingOf(count, limit)))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(ttl).Unix(), 10))

		if count > int64(limit) {
			c.Header("Retry-After", strconv.Itoa(retryAfterSeconds(ttl)))
			slog.Warn("触发限流",
				"request_id", requestIDOf(c),
				"key", key,
				"limit", limit,
				"window", window.String(),
			)
			response.Fail(c, apperr.ErrTooManyReqs)
			c.Abort()
			return
		}

		c.Next()
	}
}

// incrWindow 在固定窗口内自增计数，返回当前计数与窗口剩余时长。
//
// 实现要点：
//   - INCR 与 TTL 放进同一个 pipeline，一次往返拿到全部信息；
//   - 仅在「窗口首次请求」或「键存在但已丢失 TTL」（被外部写入、持久化恢复等）时设置过期，
//     避免每个请求都续期，把固定窗口退化成「最后一次请求起算」的滑动窗口；
//   - 不使用 EXPIRE ... NX：它要求 Redis 7.0+，低版本会直接报语法错误，
//     在 fail open 语义下会变成「限流完全静默失效」，代价太大。
func incrWindow(ctx context.Context, rdb redis.UniversalClient, key string, window time.Duration) (int64, time.Duration, error) {
	pipe := rdb.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	ttlCmd := pipe.TTL(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return 0, 0, err
	}
	if err := incrCmd.Err(); err != nil {
		return 0, 0, err
	}

	count := incrCmd.Val()
	ttl := ttlCmd.Val()

	if count <= 1 || ttl <= 0 {
		// TTL 为 -1（键无过期）或 -2（键不存在）时都需要补设过期时间。
		if err := rdb.Expire(ctx, key, window).Err(); err != nil {
			return 0, 0, err
		}
		ttl = window
	}
	return count, ttl, nil
}

// remainingOf 计算剩余可用次数，超限时归零（不返回负数）。
func remainingOf(count int64, limit int) int {
	remaining := limit - int(count)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// retryAfterSeconds 把剩余窗口时长换算为 Retry-After 需要的整数秒，至少 1 秒。
func retryAfterSeconds(ttl time.Duration) int {
	if ttl <= 0 {
		return 1
	}
	secs := int((ttl + time.Second - 1) / time.Second)
	if secs < 1 {
		return 1
	}
	return secs
}
