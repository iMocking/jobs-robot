package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// Recovery 捕获 handler 中的 panic，记录堆栈日志并返回统一的 500 响应。
//
// 取舍：
//   - 堆栈只落日志，响应体固定为 apperr.ErrInternal 的通用文案，
//     避免把内部实现路径、变量值等细节泄漏给客户端；
//   - 若 panic 发生在响应已经开始写出之后，状态码与响应体都已无法回滚，
//     此时只记日志、不再写响应，防止拼出半截 JSON；
//   - http.ErrAbortHandler 是标准库约定的「主动放弃连接」信号（反向代理等会用到），
//     按惯例继续向上抛出，不当作业务异常吞掉。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			if r == http.ErrAbortHandler {
				panic(r)
			}

			// 客户端主动断开不属于服务端故障，降为 warn 且不写响应。
			if isBrokenPipe(r) {
				slog.Warn("客户端连接中断",
					"request_id", requestIDOf(c),
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"client_ip", ClientIP(c),
					"panic", fmt.Sprint(r),
				)
				c.Abort()
				return
			}

			slog.Error("请求处理发生 panic",
				"request_id", requestIDOf(c),
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"client_ip", ClientIP(c),
				"user_id", MustUserID(c),
				"panic", fmt.Sprint(r),
				"stack", string(debug.Stack()),
			)

			if c.Writer.Written() {
				c.Abort()
				return
			}
			response.Fail(c, apperr.ErrInternal)
			c.Abort()
		}()

		c.Next()
	}
}

// isBrokenPipe 判断 panic 是否来自客户端断连（写响应失败）。
//
// 只做错误文本匹配：net.OpError 的错误串里必然包含这两种描述，
// 比逐层 errors.As 更简洁，且不会漏判被包装过的错误。
func isBrokenPipe(r any) bool {
	err, ok := r.(error)
	if !ok {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset by peer")
}

// requestIDOf 读取链路 ID，取不到时返回空串（不依赖 RequestID 中间件一定已注册）。
func requestIDOf(c *gin.Context) string {
	if v, ok := c.Get(response.RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return c.GetHeader(response.RequestIDHeader)
}
