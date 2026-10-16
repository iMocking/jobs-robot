package middleware

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// TokenParser 抽象访问令牌的解析能力，由 *auth.JWTManager 实现。
//
// 之所以用接口而不是直接依赖 *auth.JWTManager：Auth 同时需要「解析令牌」（JWTManager）
// 与「校验会话」（SessionManager）两种能力，接口让两者解耦；
// 若后续 auth.SessionManager 自己也补上 Parse 方法，`Auth(sessions)` 的写法同样成立。
type TokenParser interface {
	Parse(token string) (*auth.Claims, error)
}

// compile-time 断言：确保 *auth.JWTManager 满足 TokenParser（即 Auth(sessions, jwtManager) 可用）。
var _ TokenParser = (*auth.JWTManager)(nil)

// Auth 解析 Bearer 访问令牌并校验会话有效性。
//
// 核心语义是「无令牌不拦截、令牌无效也不拦截」：
//
//   - 没有 Authorization 头 → 直接 c.Next()，请求以匿名身份继续；
//   - 令牌过期/伪造/解析失败，或会话已登出、密码版本已变更 → 清空上下文后 c.Next()，
//     只在 debug 日志里留痕；
//   - 只有解析且校验都通过时，才把 claims、userID、roles 写入 gin.Context。
//
// 这样同一套中间件可以同时挂在公开接口与受保护接口上：
// 公开接口即使带着过期令牌也能正常访问（只是以匿名身份），
// 「必须登录」的判定统一交给后置的 RequireAuth / Authorize，避免在这里写两套中间件。
//
// tokens 传入 *auth.JWTManager；若省略，则尝试用 sessions 自身充当解析器。
func Auth(sessions *auth.SessionManager, tokens ...TokenParser) gin.HandlerFunc {
	parser := resolveTokenParser(sessions, tokens)

	return func(c *gin.Context) {
		raw := bearerToken(c.GetHeader("Authorization"))
		if raw == "" {
			// 未携带令牌：可能是公开接口，放行由后续中间件决定是否需要身份。
			c.Next()
			return
		}

		claims, err := parser.Parse(raw)
		if err != nil {
			// 公开接口带着过期令牌访问是正常现象（前端还没清理本地缓存），
			// 因此这里不能直接 401，只按匿名继续。
			slog.Debug("访问令牌解析失败，按匿名继续",
				"request_id", requestIDOf(c),
				"path", c.Request.URL.Path,
				"client_ip", ClientIP(c),
				"error", err,
			)
			clearAuthContext(c)
			c.Next()
			return
		}

		if err := sessions.Validate(c.Request.Context(), claims); err != nil {
			// 会话失效（登出、改密）属预期路径，debug 即可；
			// 若是 Redis 故障导致的内部错误，则是运维必须感知的问题，升为 warn。
			if apperr.Is(err, apperr.CodeInternal) {
				slog.Warn("会话校验依赖异常，本次按匿名继续",
					"request_id", requestIDOf(c),
					"user_id", claims.Subject,
					"path", c.Request.URL.Path,
					"error", err,
				)
			} else {
				slog.Debug("会话校验未通过，按匿名继续",
					"request_id", requestIDOf(c),
					"path", c.Request.URL.Path,
					"error", err,
				)
			}
			clearAuthContext(c)
			c.Next()
			return
		}

		userID, err := claims.UserID()
		if err != nil {
			// sub 不是合法用户 ID（只可能来自被篡改或历史遗留令牌），按匿名处理。
			slog.Debug("令牌主体非法，按匿名继续",
				"request_id", requestIDOf(c),
				"path", c.Request.URL.Path,
				"error", err,
			)
			clearAuthContext(c)
			c.Next()
			return
		}

		roles := claims.Roles
		if roles == nil {
			roles = []string{}
		}
		c.Set(ContextKeyClaims, claims)
		c.Set(ContextKeyUserID, userID)
		c.Set(ContextKeyRoles, roles)
		c.Next()
	}
}

// RequireAuth 要求请求必须已登录（即 Auth 成功写入上下文），否则返回 401。
//
// 必须挂在 Auth 之后：本中间件只做判定，不负责解析令牌。
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := Claims(c); !ok {
			// response.Fail 内部已 AbortWithStatusJSON，无需再调用 c.Abort()。
			response.Fail(c, apperr.ErrUnauthorized)
			return
		}
		if _, ok := UserID(c); !ok {
			response.Fail(c, apperr.ErrUnauthorized)
			return
		}
		c.Next()
	}
}

// resolveTokenParser 解析出令牌解析器；配置缺失时立即 panic。
//
// 这里选择「启动即失败」而不是静默放行：漏传解析器会让所有受保护接口永远返回 401
// （或更糟，被误当成匿名而放开），在路由装配阶段就暴露出来成本最低。
func resolveTokenParser(sessions *auth.SessionManager, tokens []TokenParser) TokenParser {
	for _, t := range tokens {
		if t != nil {
			return t
		}
	}
	if p, ok := any(sessions).(TokenParser); ok {
		return p
	}
	panic("middleware.Auth: 缺少访问令牌解析器，请传入 *auth.JWTManager（Auth(sessions, jwtManager)），" +
		"或在 auth.SessionManager 上补充 Parse 方法")
}

// bearerToken 从 Authorization 头提取 Bearer 令牌。
//
// 大小写不敏感地识别 "bearer" 前缀（RFC 7235 中认证 scheme 不区分大小写），
// 并容忍多余空白；格式不符时返回空串（等同于未携带令牌）。
func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}
