package mail

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// Dynamic 是可热替换的邮件投递器。
//
// 为什么需要它：管理员在后台修改 SMTP 配置后必须立即生效，
// 但 Services 在启动时就已经持有了 Mailer。用一个带读写锁的包装器承载
// 「当前生效的实现」，就能在不重启进程、不动依赖注入图的前提下完成替换。
type Dynamic struct {
	mu      sync.RWMutex
	current Mailer
	// fromName 是系统邮件的发件人显示名（由 config.mail.system_from_name 解析，
	// 为空时回退 from_name）。
	//
	// 存在包装器而不是只放在 SMTP 实现里：配置热替换（Swap）会换掉底层
	// mailer，如果显示名跟着底层对象走，管理员改一次 SMTP 就会退回旧配置。
	// 放在这里可以跨 Swap 保持，并与 mail.from_name 形成「系统通知名优先、
	// 通用名兜底」的层次。
	fromName string
}

// NewDynamic 包装一个初始投递器；initial 可为 nil（表示暂不可用）。
func NewDynamic(initial Mailer) *Dynamic {
	return &Dynamic{current: initial}
}

// SetFromName 设置系统发件人显示名；空值表示回落到配置的 from_name。
func (d *Dynamic) SetFromName(name string) {
	d.mu.Lock()
	d.fromName = strings.TrimSpace(name)
	d.mu.Unlock()
}

// Send 使用当前生效的投递器发送邮件。
func (d *Dynamic) Send(ctx context.Context, msg Message) error {
	d.mu.RLock()
	m := d.current
	fromName := d.fromName
	d.mu.RUnlock()

	if m == nil {
		return apperr.ErrUnavailable.WithMessage("邮件服务尚未配置，请在后台「邮件设置」中完成配置")
	}
	if strings.TrimSpace(msg.FromName) == "" {
		msg.FromName = fromName
	}
	return m.Send(ctx, msg)
}

// Name 返回当前驱动名；未配置时返回 "none"。
func (d *Dynamic) Name() string {
	if m := d.Current(); m != nil {
		return m.Name()
	}
	return "none"
}

// Current 返回当前生效的投递器（可能为 nil）。
func (d *Dynamic) Current() Mailer {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.current
}

// Swap 原子替换当前投递器。
func (d *Dynamic) Swap(next Mailer) {
	d.mu.Lock()
	d.current = next
	d.mu.Unlock()
}

// TestMessage 构造配置测试邮件。
//
// 后台点「发送测试邮件」时会用到：内容刻意包含发信时间与驱动信息，
// 便于管理员确认「收到的确实是刚刚这次配置」。
func TestMessage(to, driver string, at time.Time) Message {
	subject := "【JobHub 直招】邮件配置测试"
	text := fmt.Sprintf(`您好，

这是一封来自 JobHub 直招后台的测试邮件。

发送时间：%s
投递通道：%s

如果您收到了这封邮件，说明邮件配置已经生效，注册验证码与密码重置邮件可以正常送达。
若非本人操作，请检查后台「系统设置 - 邮件设置」是否有他人修改。

—— JobHub 直招
`, at.Format("2006-01-02 15:04:05"), driver)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<body style="margin:0;padding:24px;background:#F5F7FA;font-family:-apple-system,'Segoe UI','Microsoft YaHei',sans-serif;color:#111827;">
  <div style="max-width:520px;margin:0 auto;background:#ffffff;border-radius:16px;padding:32px;border:1px solid #E5E7EB;">
    <h2 style="margin:0 0 8px;font-size:18px;">邮件配置测试</h2>
    <p style="margin:0 0 20px;color:#6B7280;font-size:13px;">JobHub 直招 · 后台系统设置</p>
    <p style="margin:0 0 12px;font-size:14px;">测试邮件发送成功，邮件通道工作正常。</p>
    <table style="font-size:13px;color:#374151;border-collapse:collapse;">
      <tr><td style="padding:4px 12px 4px 0;color:#6B7280;">发送时间</td><td>%s</td></tr>
      <tr><td style="padding:4px 12px 4px 0;color:#6B7280;">投递通道</td><td>%s</td></tr>
    </table>
    <p style="margin:20px 0 0;font-size:12px;color:#9CA3AF;">若非本人操作，请检查后台「系统设置 - 邮件设置」是否有他人修改。</p>
  </div>
</body>
</html>`, at.Format("2006-01-02 15:04:05"), driver)

	return Message{To: to, Subject: subject, Text: text, HTML: html}
}
