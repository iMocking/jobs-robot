package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/casbin"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// Authorize 基于 Casbin 做接口级鉴权，请求三元组为 (user:<ulid>, 请求路径, HTTP 方法)。
//
// 设计要点：
//   - 路径取 c.Request.URL.Path 原值，不做 /api/v1 前缀裁剪：
//     本项目接口统一挂在该前缀下，策略表里也以完整路径为准，
//     保持一致可避免「路由前缀调整后策略静默失配」；
//   - 未登录（前面没有 RequireAuth 时）返回 401，已登录但策略未放行返回 403，
//     两者语义不同，前端需要据此区分「去登录」与「无权限」；
//   - 策略引擎自身出错按 500 处理，不能降级成 403：
//     否则数据库/适配器故障会被误报成用户没有权限，掩盖真实故障。
func Authorize(authz *casbin.Authorizer) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := UserID(c)
		if !ok {
			response.Fail(c, apperr.ErrUnauthorized)
			c.Abort()
			return
		}

		path := c.Request.URL.Path
		method := c.Request.Method

		allowed, err := authz.Enforce(casbin.SubjectForUser(userID), path, method)
		if err != nil {
			slog.Error("鉴权判定失败",
				"request_id", requestIDOf(c),
				"user_id", userID,
				"path", path,
				"method", method,
				"error", err,
			)
			response.Fail(c, apperr.ErrInternal.WithCause(err))
			c.Abort()
			return
		}
		if !allowed {
			slog.Debug("鉴权未通过",
				"request_id", requestIDOf(c),
				"user_id", userID,
				"path", path,
				"method", method,
			)
			response.Fail(c, apperr.ErrForbidden)
			c.Abort()
			return
		}

		c.Next()
	}
}
