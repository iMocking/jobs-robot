// Package mail 提供可切换的邮件投递能力（开发期控制台输出，生产走 SMTP）。
package mail

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jsong/jobs-robot/internal/config"
)

// Message 是一封待发送的邮件。
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	// FromName 是可选的发件人显示名覆盖（例如「Jobs Mail Robot」）。
	//
	// 为空时回落到投递器自身的 from_name（config.mail.from_name），
	// 这样直接构造 mailer 的调用方与测试不需要感知系统身份。
	FromName string
}

// Mailer 是邮件投递接口。
type Mailer interface {
	// Send 投递邮件，失败返回错误。
	Send(ctx context.Context, msg Message) error
	// Name 返回驱动名（console/smtp），用于审计记录。
	Name() string
}

// New 按配置构造 Mailer。
func New(cfg config.Mail) (Mailer, error) {
	switch strings.ToLower(cfg.Driver) {
	case "", "console":
		return &consoleMailer{from: cfg.FromAddress}, nil
	case "smtp":
		return newSMTPMailer(cfg)
	default:
		return nil, fmt.Errorf("不支持的邮件驱动: %s", cfg.Driver)
	}
}

// consoleMailer 把邮件内容打印到日志，便于本地联调与自动化测试。
//
// 明确标注为「开发模式」，避免误以为已真实发信。
type consoleMailer struct{ from string }

// Name 返回驱动名。
func (m *consoleMailer) Name() string { return "console" }

// Send 打印邮件内容。
func (m *consoleMailer) Send(_ context.Context, msg Message) error {
	slog.Info("【开发模式邮件】未真实发送，仅打印内容",
		"from", m.from,
		"to", msg.To,
		"subject", msg.Subject,
		"text", msg.Text,
	)
	return nil
}

// SetFromName 为 Mailer 设置发件人显示名（可选能力）。
//
// 支持的实现：*Dynamic（跨配置热替换保持）与 *smtpMailer。
// 其他实现（例如测试替身）静默忽略，避免为了一个展示字段扩大 Mailer 接口。
func SetFromName(m Mailer, name string) {
	switch v := m.(type) {
	case *Dynamic:
		v.SetFromName(name)
	case *smtpMailer:
		v.fromName = name
	}
}

// VerificationCodeMessage 构造验证码邮件。
func VerificationCodeMessage(to, code, purposeLabel string, ttl time.Duration) Message {
	minutes := int(ttl.Minutes())
	subject := fmt.Sprintf("【直招好工作】%s验证码：%s", purposeLabel, code)

	text := fmt.Sprintf(`您好，

您正在进行「%s」操作，验证码为：

    %s

验证码 %d 分钟内有效，请勿转发给他人。
如果这不是您本人的操作，请忽略本邮件。

—— 直招好工作
`, purposeLabel, code, minutes)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<body style="margin:0;padding:24px;background:#f5f6f8;font-family:-apple-system,'Segoe UI','Microsoft YaHei',sans-serif;color:#1f2329;">
  <div style="max-width:520px;margin:0 auto;background:#ffffff;border-radius:12px;padding:32px;">
    <h2 style="margin:0 0 8px;font-size:18px;">直招好工作</h2>
    <p style="margin:0 0 24px;color:#646a73;font-size:14px;">企业官网直招岗位聚合平台</p>
    <p style="margin:0 0 8px;font-size:14px;">您正在进行「%s」操作，验证码为：</p>
    <p style="margin:16px 0;font-size:32px;font-weight:700;letter-spacing:6px;color:#3370ff;">%s</p>
    <p style="margin:0 0 8px;font-size:13px;color:#8f959e;">验证码 %d 分钟内有效，请勿转发给他人。</p>
    <p style="margin:0;font-size:13px;color:#8f959e;">如果这不是您本人的操作，请忽略本邮件。</p>
  </div>
</body>
</html>`, purposeLabel, code, minutes)

	return Message{To: to, Subject: subject, Text: text, HTML: html}
}

// PasswordResetDoneMessage 构造密码重置成功通知邮件。
func PasswordResetDoneMessage(to string, at time.Time) Message {
	subject := "【直招好工作】您的密码已重置"
	text := fmt.Sprintf(`您好，

您的账号密码已于 %s 重置成功。

如果这不是您本人的操作，请立即通过「忘记密码」重新设置，并检查账号安全。

—— 直招好工作
`, at.Format("2006-01-02 15:04:05"))

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<body style="margin:0;padding:24px;background:#f5f6f8;font-family:-apple-system,'Segoe UI','Microsoft YaHei',sans-serif;color:#1f2329;">
  <div style="max-width:520px;margin:0 auto;background:#ffffff;border-radius:12px;padding:32px;">
    <h2 style="margin:0 0 16px;font-size:18px;">密码已重置</h2>
    <p style="margin:0 0 8px;font-size:14px;">您的账号密码已于 <strong>%s</strong> 重置成功。</p>
    <p style="margin:0;font-size:13px;color:#8f959e;">如果这不是您本人的操作，请立即通过「忘记密码」重新设置，并检查账号安全。</p>
  </div>
</body>
</html>`, at.Format("2006-01-02 15:04:05"))

	return Message{To: to, Subject: subject, Text: text, HTML: html}
}
