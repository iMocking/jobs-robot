package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/jsong/jobs-robot/internal/config"
)

// smtpMailer 通过 SMTP 投递邮件，支持隐式 TLS（465）与 STARTTLS（587）。
type smtpMailer struct {
	host     string
	port     int
	username string
	password string
	from     string
	fromName string
	implicit bool
}

func newSMTPMailer(cfg config.Mail) (*smtpMailer, error) {
	if cfg.SMTP.Host == "" || cfg.SMTP.Port <= 0 {
		return nil, fmt.Errorf("SMTP 配置不完整：host/port 必填")
	}
	if cfg.FromAddress == "" {
		return nil, fmt.Errorf("SMTP 配置不完整：from_address 必填")
	}
	return &smtpMailer{
		host:     cfg.SMTP.Host,
		port:     cfg.SMTP.Port,
		username: cfg.SMTP.Username,
		password: cfg.SMTP.Password,
		from:     cfg.FromAddress,
		fromName: cfg.FromName,
		implicit: cfg.SMTP.TLS,
	}, nil
}

// Name 返回驱动名。
func (m *smtpMailer) Name() string { return "smtp" }

// Send 投递邮件。
func (m *smtpMailer) Send(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	client, err := m.dial()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if m.username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
			// 认证失败单独成类（ErrAuthFailed）：535 这类响应说明的是
			// 「凭据或服务开关不对」，属于使用者配置问题，必须能被上层区分出来
			// 并给出可照做的排查步骤，而不是笼统地报「服务器内部错误」。
			if authErr := classifyAuthFailure(m.host, m.username, m.password, err); authErr != nil {
				return authErr
			}
			// 其余（例如认证过程中连接被重置）：保留原文，但同样先做口令脱敏。
			return newRedactedError("SMTP 认证失败", m.username, m.password, err)
		}
	}
	if err := client.Mail(m.from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM 失败: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("SMTP RCPT TO 失败: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA 失败: %w", err)
	}
	if _, err := w.Write(m.build(msg)); err != nil {
		_ = w.Close()
		return fmt.Errorf("写入邮件正文失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("提交邮件失败: %w", err)
	}
	return client.Quit()
}

func (m *smtpMailer) dial() (*smtp.Client, error) {
	addr := net.JoinHostPort(m.host, strconv.Itoa(m.port))
	tlsConfig := &tls.Config{ServerName: m.host, MinVersion: tls.VersionTLS12}

	if m.implicit {
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return nil, m.connectError(err, "")
		}
		client, err := smtp.NewClient(conn, m.host)
		if err != nil {
			_ = conn.Close()
			return nil, m.connectError(err, "SMTP 问候")
		}
		return client, nil
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, m.connectError(err, "")
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(tlsConfig); err != nil {
			_ = client.Close()
			return nil, m.connectError(err, "STARTTLS 握手")
		}
	}
	return client, nil
}

// connectError 归类连接/握手失败，并把本次提交的 host/port/加密方式一起带上。
//
// 带上这三个参数是刻意的：465 端口选成 STARTTLS 时对端只回一个 EOF，
// 单看错误文本根本推不出「端口与加密方式不匹配」，而把当前提交回显出来，
// 运维一眼就能看出自己选错了哪一项（见 internal/service/mail_hint.go）。
func (m *smtpMailer) connectError(err error, stage string) error {
	return newConnectError(err, stage, m.host, m.port, m.implicit)
}

// build 组装 MIME 报文；正文使用 base64 编码，避免中文与长行被网关截断。
func (m *smtpMailer) build(msg Message) []byte {
	boundary := "jobsrobot-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	var sb strings.Builder
	// 显示名优先用消息级的覆盖值（由 service 按 mail.system_from_name 注入），
	// 为空才回落配置里的 mail.from_name。两者都按 RFC 2047 编码。
	fromName := strings.TrimSpace(msg.FromName)
	if fromName == "" {
		fromName = m.fromName
	}
	writeHeader(&sb, "From", formatAddress(fromName, m.from))
	writeHeader(&sb, "To", msg.To)
	writeHeader(&sb, "Subject", mime.BEncoding.Encode("UTF-8", msg.Subject))
	writeHeader(&sb, "Date", time.Now().Format(time.RFC1123Z))
	writeHeader(&sb, "MIME-Version", "1.0")
	writeHeader(&sb, "Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", boundary))
	sb.WriteString("\r\n")

	// 纯文本部分
	sb.WriteString("--" + boundary + "\r\n")
	writeHeader(&sb, "Content-Type", "text/plain; charset=UTF-8")
	writeHeader(&sb, "Content-Transfer-Encoding", "base64")
	sb.WriteString("\r\n")
	sb.WriteString(base64Lines(msg.Text))
	sb.WriteString("\r\n")

	// HTML 部分
	html := msg.HTML
	if strings.TrimSpace(html) == "" {
		html = "<pre>" + msg.Text + "</pre>"
	}
	sb.WriteString("--" + boundary + "\r\n")
	writeHeader(&sb, "Content-Type", "text/html; charset=UTF-8")
	writeHeader(&sb, "Content-Transfer-Encoding", "base64")
	sb.WriteString("\r\n")
	sb.WriteString(base64Lines(html))
	sb.WriteString("\r\n")

	sb.WriteString("--" + boundary + "--\r\n")
	return []byte(sb.String())
}

func writeHeader(sb *strings.Builder, key, value string) {
	sb.WriteString(key)
	sb.WriteString(": ")
	sb.WriteString(value)
	sb.WriteString("\r\n")
}

// formatAddress 生成 "显示名 <地址>" 形式，显示名按 RFC 2047 编码。
func formatAddress(name, addr string) string {
	if strings.TrimSpace(name) == "" {
		return addr
	}
	return mime.BEncoding.Encode("UTF-8", name) + " <" + addr + ">"
}

// base64Lines 输出每行 76 字符的 base64，符合 RFC 2045 对行长度的要求。
func base64Lines(s string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(s))
	var sb strings.Builder
	for len(encoded) > 76 {
		sb.WriteString(encoded[:76])
		sb.WriteString("\r\n")
		encoded = encoded[76:]
	}
	sb.WriteString(encoded)
	return sb.String()
}
