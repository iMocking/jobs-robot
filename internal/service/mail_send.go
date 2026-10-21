package service

import (
	"context"
	"strings"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/mail"
)

// 系统触发邮件的审计 purpose 取值。
//
// 用稳定英文标识而不是中文标签：后台按 action/purpose 过滤时不应受文案改动影响。
const (
	mailPurposeVerifyRegister      = "verify_register"
	mailPurposeResetPassword       = "reset_password"
	mailPurposeVerifyChangeEmail   = "verify_change_email"
	mailPurposeVerifyLogin         = "verify_login"
	mailPurposeNotifyPasswordReset = "notify_password_changed"
	mailPurposeNotifyRoleChanged   = "notify_role_changed"
	mailPurposeVerifyUnknown       = "verify"
)

// mailPurposeForCode 把验证码用途映射为审计 purpose。
func mailPurposeForCode(p auth.CodePurpose) string {
	switch p {
	case auth.CodePurposeRegister:
		return mailPurposeVerifyRegister
	case auth.CodePurposeResetPassword:
		return mailPurposeResetPassword
	case auth.CodePurposeChangeEmail:
		return mailPurposeVerifyChangeEmail
	case auth.CodePurposeLogin:
		return mailPurposeVerifyLogin
	default:
		return mailPurposeVerifyUnknown
	}
}

// sendSystemMail 是「系统自动邮件」的统一出口：发送 + 记一条 mail.send 审计。
//
// 为什么集中在这里而不是每个调用点各写一遍：审计字段（actor、purpose、driver、
// 脱敏收件人）必须完全一致，否则后台无法用一条查询回答「机器人到底发了哪些信」。
//
// 注意：后台管理员点击的「测试发信」不走这里（那是人的动作，actor 必须是管理员），
// 见 MailService.SendTest / SetupService。
//
// 只记录可公开的元信息：绝不写正文、验证码、口令、SMTP 凭据或完整收件人地址。
func (svc *Services) sendSystemMail(ctx context.Context, purpose, to, resourceID string, msg mail.Message) error {
	err := svc.Mailer.Send(ctx, msg)

	detail := map[string]any{
		"purpose":   purpose,
		"driver":    svc.Mailer.Name(),
		"to_masked": maskEmail(to),
		"success":   err == nil,
	}
	svc.Audit.Record(ctx, SystemActorFor(SystemKeyMail), "mail.send", "user", resourceID, detail)
	return err
}

// maskEmail 把收件人脱敏成 a***@example.com 形式。
//
// 宁可少保留信息也不要写完整邮箱：审计日志会被更多角色看到，
// 而「谁在什么时候收到过哪类邮件」并不需要精确到完整地址。
func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		// 不是合法邮箱（理论上不会走到这里）：只留首字符。
		if len(email) <= 1 {
			return "***"
		}
		return email[:1] + "***"
	}
	local := email[:at]
	domain := email[at:]
	if local == "" {
		return "***" + domain
	}
	return local[:1] + "***" + domain
}
