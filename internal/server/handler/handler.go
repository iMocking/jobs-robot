// Package handler 是 HTTP 传输层：只做参数绑定、调用 service、封装响应。
//
// 约定：
//   - 任何业务判断都不写在这里，全部下沉到 service；
//   - 错误一律通过 response.Fail 输出，由 service 返回带错误码的 *apperr.Error。
package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/ulidid"
	"github.com/jsong/jobs-robot/internal/server/middleware"
	"github.com/jsong/jobs-robot/internal/service"
)

// Handler 持有服务集合，为所有接口提供依赖。
type Handler struct {
	svc *service.Services
}

// New 构造 Handler。
func New(svc *service.Services) *Handler { return &Handler{svc: svc} }

// Services 暴露服务集合，供路由装配时读取配置等。
func (h *Handler) Services() *service.Services { return h.svc }

// actor 从鉴权上下文构造审计操作者。
func (h *Handler) actor(c *gin.Context) service.Actor {
	userID, _ := middleware.UserID(c)
	claims, _ := middleware.Claims(c)

	email := ""
	if claims != nil {
		email = claims.Email
	}
	return service.Actor{
		UserID:    userID,
		Email:     email,
		IP:        middleware.ClientIP(c),
		UserAgent: c.Request.UserAgent(),
		Method:    c.Request.Method,
		Path:      c.Request.URL.Path,
	}
}

// pathID 解析路径中的 ULID 标识。
//
// 所有实体主键都是 26 位 ULID 字符串，因此这里先做格式校验再交给 service，
// 避免把任意字符串带进数据库查询（非法值统一返回参数错误 10001）。
func (h *Handler) pathID(c *gin.Context, name string) (string, error) {
	raw := strings.TrimSpace(c.Param(name))
	if !ulidid.Valid(raw) {
		return "", apperr.ErrInvalidParams.WithMessage("路径参数 " + name + " 必须是合法的 ULID")
	}
	return raw, nil
}

// queryString 读取查询参数中的字符串标识（去除首尾空白，缺省返回空串）。
//
// 标识类查询参数（company_id / actor_id / source_url 等）都是字符串，
// 空串表示「不过滤」，与旧的 queryInt(...,0) 语义一致。
func queryString(c *gin.Context, name string) string {
	return strings.TrimSpace(c.Query(name))
}

// queryInt 读取查询参数中的整数（缺省或非法时返回默认值）。
//
// 仅用于页码、每页条数这类真正的数字参数；标识一律用 queryString。
func queryInt(c *gin.Context, name string, fallback int) int {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

// queryBool 读取查询参数中的布尔值（缺省时返回 nil，表示不过滤）。
func queryBool(c *gin.Context, name string) *bool {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return nil
	}
	return &v
}

// pagination 读取分页参数。
func pagination(c *gin.Context) (page, size int) {
	return queryInt(c, "page", 1), queryInt(c, "page_size", 20)
}

// bindJSON 绑定并校验 JSON 请求体。
func bindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return apperr.ErrInvalidParams.WithDetails(map[string]any{
			"reason": err.Error(),
		})
	}
	return nil
}

// clientIP 返回客户端地址（限流与审计使用）。
func clientIP(c *gin.Context) string { return middleware.ClientIP(c) }
