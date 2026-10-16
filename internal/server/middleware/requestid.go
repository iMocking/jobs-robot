package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// maxRequestIDLen 是允许复用的入站链路 ID 最大长度。
const maxRequestIDLen = 64

// RequestID 为每个请求分配链路 ID。
//
// 行为：
//   - 优先复用入站 X-Request-Id，但必须先过长度与字符白名单校验：
//     该值来自外部，直接写日志会造成日志注入（换行/控制字符）、超长值会撑爆日志；
//   - 校验不通过或未携带时生成 UUID v4 风格随机串；
//   - 结果写入 gin.Context（键为 response.RequestIDKey）并回写响应头，
//     便于前端与网关用同一个 ID 串联日志。
//
// 本中间件应最先注册，保证后续中间件与 handler 都能取到链路 ID。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := sanitizeRequestID(c.GetHeader(response.RequestIDHeader))
		if rid == "" {
			rid = newRequestID()
		}
		c.Set(response.RequestIDKey, rid)
		c.Writer.Header().Set(response.RequestIDHeader, rid)
		c.Next()
	}
}

// sanitizeRequestID 校验并规整入站链路 ID，非法时返回空串（交由调用方生成新 ID）。
//
// 字符白名单只允许字母、数字、'-'、'_'：足以覆盖各语言/网关生成的 UUID、trace id，
// 又能挡掉换行、空格、引号等会污染日志或响应头的字符。
func sanitizeRequestID(raw string) string {
	if raw == "" || len(raw) > maxRequestIDLen {
		return ""
	}
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case ch >= 'a' && ch <= 'z',
			ch >= 'A' && ch <= 'Z',
			ch >= '0' && ch <= '9',
			ch == '-', ch == '_':
		default:
			return ""
		}
	}
	return raw
}

// newRequestID 生成 UUID v4 风格的随机链路 ID。
func newRequestID() string {
	id, err := uuid.NewRandom()
	if err == nil {
		return id.String()
	}
	// 熵源不可用属于极端情况，退化为「时间戳 + 随机十六进制」，
	// 宁可 ID 形态不标准，也不能让链路 ID 为空导致日志无法串联。
	slog.Warn("生成 UUID 失败，降级为时间戳链路 ID", "error", err)
	return fallbackRequestID()
}

func fallbackRequestID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + hex.EncodeToString(buf[:])
}
