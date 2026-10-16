// Package middleware 汇集 HTTP 中间件：链路 ID、访问日志、异常恢复、跨域、认证、鉴权与限流。
//
// 建议注册顺序（自外向内）：
//
//	RequestID → Recovery → AccessLog → CORS → Auth → RequireAuth / Authorize → RateLimit
//
// 其中 Recovery 必须在业务 handler 外侧；Auth 必须早于 RequireAuth 与 Authorize，
// 否则后两者读不到已解析的登录态。
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/auth"
)

// gin.Context 中存放认证信息的键。
//
// 统一用 "auth." 前缀，避免与其它中间件或 handler 的键冲突。
const (
	ContextKeyClaims = "auth.claims"  // *auth.Claims
	ContextKeyUserID = "auth.user_id" // string（ULID）
	ContextKeyRoles  = "auth.roles"   // []string
)

// Claims 返回当前请求的访问令牌载荷；未登录（或校验未通过）时第二个返回值为 false。
func Claims(c *gin.Context) (*auth.Claims, bool) {
	v, ok := c.Get(ContextKeyClaims)
	if !ok {
		return nil, false
	}
	claims, ok := v.(*auth.Claims)
	if !ok || claims == nil {
		return nil, false
	}
	return claims, true
}

// UserID 返回当前登录用户 ID（ULID 字符串）；未登录时返回 ("", false)。
func UserID(c *gin.Context) (string, bool) {
	v, ok := c.Get(ContextKeyUserID)
	if !ok {
		return "", false
	}
	id, ok := v.(string)
	if !ok || id == "" {
		return "", false
	}
	return id, true
}

// MustUserID 返回当前登录用户 ID，未登录时返回空串。
//
// 供只在受保护路由里使用的代码（如访问日志）省去判空样板；这些位置不校验身份，
// 因此不返回 (id, ok) 二元组。
func MustUserID(c *gin.Context) string {
	id, _ := UserID(c)
	return id
}

// Roles 返回当前登录用户的角色列表；未登录时返回空切片（非 nil，便于调用方直接遍历）。
func Roles(c *gin.Context) []string {
	v, ok := c.Get(ContextKeyRoles)
	if !ok {
		return []string{}
	}
	roles, ok := v.([]string)
	if !ok || roles == nil {
		return []string{}
	}
	return roles
}

// ClientIP 返回客户端 IP。
//
// 取值优先级：X-Forwarded-For 首个地址 → X-Real-IP → gin 的 c.ClientIP()。
//
// 取舍：XFF 由代理逐跳追加，首个地址才是最初的客户端；但它可被客户端伪造，
// 因此该函数只用于限流分桶、登录日志这类「尽力而为」的场景。
// 若需要严格可信的 IP，应配置 server.trusted_proxies 并只用 c.ClientIP()。
func ClientIP(c *gin.Context) string {
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// XFF 形如 "client, proxy1, proxy2"，取第一个非空段。
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			xff = xff[:idx]
		}
		if ip := strings.TrimSpace(xff); ip != "" {
			return ip
		}
	}
	if ip := strings.TrimSpace(c.GetHeader("X-Real-IP")); ip != "" {
		return ip
	}
	return c.ClientIP()
}

// clearAuthContext 清空上下文中的认证信息（Auth 解析或校验失败时调用）。
//
// gin.Context 没有删除键的方法，这里统一写入零值；读取侧（Claims/UserID/Roles）
// 对零值一律按未登录处理，因此效果等同于删除。
func clearAuthContext(c *gin.Context) {
	c.Set(ContextKeyClaims, (*auth.Claims)(nil))
	c.Set(ContextKeyUserID, "")
	c.Set(ContextKeyRoles, []string(nil))
}
