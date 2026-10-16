package middleware

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// 配置留空时的兜底：预检请求缺少 Allow-Methods / Allow-Headers 会被浏览器直接拦下，
// 且排查成本高，因此这里给出常见默认值，而不是让跨域在「配置漏填」时静默失效。
var (
	defaultCORSMethods = []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
	}
	defaultCORSHeaders = []string{
		"Origin", "Content-Type", "Accept", "Authorization", response.RequestIDHeader,
	}
)

// CORS 按配置处理跨域请求。
//
// 自行实现而不用第三方库，是为了让「通配来源 + 携带凭证」这种不安全组合的处置方式显式可见：
//   - AllowOrigins 精确匹配（忽略首尾空白与末尾斜杠），命中即回显具体 Origin；
//   - 配置中出现 "*" 时视为通配；但按 CORS 规范，携带凭证时不允许返回 "*"，
//     此时必须回显具体 Origin（因此 Vary: Origin 是必须的，否则 CDN/代理会把
//     某个来源的响应缓存给其它来源）；
//   - 无论是否放行都写 Vary: Origin；预检请求直接 204 返回，不进业务 handler。
func CORS(cfg config.CORS) gin.HandlerFunc {
	if !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	// 启动时预处理配置：可见性检查与切片拼接都只做一次，请求路径上零分配。
	allowAll := false
	origins := make(map[string]struct{}, len(cfg.AllowOrigins))
	for _, o := range cfg.AllowOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" {
			allowAll = true
			continue
		}
		origins[strings.TrimSuffix(o, "/")] = struct{}{}
	}

	methods := cfg.AllowMethods
	if len(methods) == 0 {
		methods = defaultCORSMethods
	}
	headers := cfg.AllowHeaders
	if len(headers) == 0 {
		headers = defaultCORSHeaders
	}
	allowMethods := strings.Join(methods, ", ")
	allowHeaders := strings.Join(headers, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")

	if allowAll && cfg.AllowCred {
		slog.Warn("CORS 配置为通配来源且允许携带凭证：将回显具体 Origin 而非 *，" +
			"这等同于允许任意站点携带用户凭证调用本服务，请改为显式来源白名单")
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		// 响应内容随 Origin 变化，必须声明 Vary；否则缓存会把带 CORS 头的响应复用给其它来源。
		c.Writer.Header().Add("Vary", "Origin")

		if origin == "" {
			// 非跨域请求（同源页面、服务端调用、curl）：无需任何 CORS 头。
			c.Next()
			return
		}

		_, exact := origins[strings.TrimSuffix(origin, "/")]
		if !exact && !allowAll {
			// 不在白名单：不写 Access-Control-Allow-Origin，浏览器自会拦截。
			// 预检请求没有业务语义，直接 204 结束，避免落到路由或 404 处理器上。
			if isPreflight(c) {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
			c.Next()
			return
		}

		if allowAll && !cfg.AllowCred {
			// 公开只读接口的常见形态：不携带凭证，可以安全地返回通配符。
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			// 精确白名单，或「通配 + 携带凭证」：规范要求回显具体 Origin。
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}
		if cfg.AllowCred {
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if exposeHeaders != "" {
			c.Writer.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
		}

		if isPreflight(c) {
			// 预检结果还取决于请求的方法与头，一并声明 Vary。
			c.Writer.Header().Add("Vary", "Access-Control-Request-Method")
			c.Writer.Header().Add("Vary", "Access-Control-Request-Headers")
			c.Writer.Header().Set("Access-Control-Allow-Methods", allowMethods)
			c.Writer.Header().Set("Access-Control-Allow-Headers", allowHeaders)
			if cfg.MaxAge > 0 {
				c.Writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
			}
			// 204 + 结束：预检不应触发鉴权、限流与业务逻辑，否则会白白消耗配额。
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// isPreflight 判断是否为 CORS 预检请求。
//
// 只有「OPTIONS + 携带 Access-Control-Request-Method」才是预检；
// 普通 OPTIONS 请求（用于探测服务支持的方法）应正常走路由。
func isPreflight(c *gin.Context) bool {
	return c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != ""
}
