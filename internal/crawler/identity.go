package crawler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/jsong/jobs-robot/internal/config"
)

// UserAgent 构造符合行业惯例的 AI 机器人 User-Agent。
//
// 形如：JobsRobotBot/1.0 (+https://example.com/bot; AI-crawler; contact: bot@example.com)
// 这是让站长能够识别、联系并单独放行/封禁我们的标准做法。
func UserAgent(id config.Identity) string {
	name := strings.TrimSpace(id.BotName)
	if name == "" {
		name = "JobsRobotBot"
	}
	version := strings.TrimSpace(id.BotVersion)
	if version == "" {
		version = "1.0"
	}

	var comments []string
	if home := strings.TrimSpace(id.Homepage); home != "" {
		comments = append(comments, home)
	}
	if id.IsAI {
		comments = append(comments, "AI-crawler")
	}
	if email := strings.TrimSpace(id.ContactEmail); email != "" {
		comments = append(comments, "contact: "+email)
	}

	if len(comments) == 0 {
		return name + "/" + version
	}
	return name + "/" + version + " (+" + strings.Join(comments, "; ") + ")"
}

// BuildHeaders 构造单次请求的完整请求头。
//
// 除标准浏览器兼容头之外，额外携带一组 X-Robot-* 头，把抓取方身份与用途
// 显式声明给目标站点；站长在日志中一眼即可辨认，无需猜测流量来源。
func BuildHeaders(id config.Identity, target *url.URL) http.Header {
	h := http.Header{}

	h.Set("User-Agent", UserAgent(id))
	// From 是 RFC 9110 定义的联络方式头，给站长提供可回信的地址。
	if email := strings.TrimSpace(id.ContactEmail); email != "" {
		h.Set("From", email)
	}

	// 显式声明机器人身份与用途。
	h.Set("X-Robot-Name", headerValue(id.BotName))
	h.Set("X-Robot-Version", headerValue(id.BotVersion))
	h.Set("X-Robot-Contact", headerValue(id.ContactEmail))
	if home := strings.TrimSpace(id.Homepage); home != "" {
		h.Set("X-Robot-Homepage", headerValue(home))
	}
	if purpose := strings.TrimSpace(id.Purpose); purpose != "" {
		h.Set("X-Robot-Purpose", headerValue(purpose))
	}
	if id.IsAI {
		// 明确标注为 AI 机器人，便于站点按 AI 策略处置。
		h.Set("X-Robot-AI", "true")
		h.Set("X-Robot-Type", "ai-crawler")
	}
	h.Set("X-Robot-Respect-Robots", "true")

	// 兼容常规站点的内容协商头，保证拿到与浏览器一致的页面。
	lang := strings.TrimSpace(id.Language)
	if lang == "" {
		lang = "zh-CN,zh;q=0.9,en;q=0.8"
	}
	h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/json;q=0.8,*/*;q=0.7")
	h.Set("Accept-Language", lang)
	h.Set("Accept-Encoding", "gzip, deflate, br")
	h.Set("Cache-Control", "no-cache")
	h.Set("Pragma", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("Upgrade-Insecure-Requests", "1")

	if target != nil {
		h.Set("Host", target.Host)
	}
	return h
}

// headerValue 把非 ASCII 字符按 RFC 3986 百分号编码。
//
// HTTP 头字段值只允许可见 ASCII；用途说明等中文文案必须先编码，
// 否则部分服务器会直接返回 400。
func headerValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(s))
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 0x20 && b < 0x7f {
			sb.WriteByte(b)
			continue
		}
		const hexDigits = "0123456789ABCDEF"
		sb.WriteByte('%')
		sb.WriteByte(hexDigits[b>>4])
		sb.WriteByte(hexDigits[b&0x0f])
	}
	return sb.String()
}

// estimateTextLength 去掉标签后估算正文长度。
func estimateTextLength(html string) int {
	if html == "" {
		return 0
	}
	var sb strings.Builder
	inTag := false
	for i := 0; i < len(html); i++ {
		switch c := html[i]; c {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag && c > ' ' {
				sb.WriteByte(c)
			}
		}
	}
	return sb.Len()
}
