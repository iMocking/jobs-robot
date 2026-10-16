package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// probePaths 是探针与监控抓取路径：这些请求由编排系统高频发起，没有业务含义。
var probePaths = map[string]struct{}{
	"/healthz": {},
	"/readyz":  {},
	"/livez":   {},
	"/metrics": {},
}

// AccessLog 记录访问日志（方法、路径、状态码、耗时、客户端 IP、用户 ID、链路 ID）。
//
// 取舍：
//   - enabled 为 false 时直接返回空中间件，连计时都不做，避免无谓开销；
//   - 探针路径降为 debug：k8s 每几秒一次的探测会在 info 级别淹没有效日志；
//   - 状态码决定级别：5xx → error，4xx → warn，其余 → info，
//     便于用日志级别直接做告警与筛选，而不必写日志查询条件；
//   - 日志在 c.Next() 之后写，此时状态码与响应体大小才确定。
func AccessLog(enabled bool) gin.HandlerFunc {
	if !enabled {
		// 显式返回空操作中间件而不是 nil，避免调用方在 AppendHandlers 时崩溃。
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			"request_id", requestIDOf(c),
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency", time.Since(start).Round(time.Microsecond).String(),
			"client_ip", ClientIP(c),
			"user_id", MustUserID(c),
		}
		if rawQuery != "" {
			attrs = append(attrs, "query", rawQuery)
		}
		// gin 未写出任何内容时 Size() 为 -1，归一成 0 更符合直觉。
		if size := c.Writer.Size(); size > 0 {
			attrs = append(attrs, "bytes", size)
		}
		// 只上报 handler 主动写入的私有错误（公共错误已由 response.Fail 输出响应体）。
		if msg := strings.TrimSpace(c.Errors.ByType(gin.ErrorTypePrivate).String()); msg != "" {
			attrs = append(attrs, "errors", msg)
		}

		switch {
		case isProbePath(path):
			slog.Debug("http 探针请求", attrs...)
		case status >= 500:
			slog.Error("http 请求处理失败", attrs...)
		case status >= 400:
			slog.Warn("http 请求被拒绝", attrs...)
		default:
			slog.Info("http 请求完成", attrs...)
		}
	}
}

// isProbePath 判断是否为探针路径。
func isProbePath(path string) bool {
	_, ok := probePaths[path]
	return ok
}
