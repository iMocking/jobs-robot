package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jsong/jobs-robot/internal/config"
)

// fakeSMTP 是一个只实现投递必需命令的极简 SMTP 服务器。
//
// 目的：在不依赖任何外部邮件服务的前提下，真实验证 smtpMailer 的
// 连接、信封、DATA 提交与退出流程，以及最终报文的头部与正文编码是否合规。
// 默认刻意不宣告 STARTTLS 与 AUTH，覆盖的是「内网中继/无认证直连」这条常见路径；
// 设置 authReject 后则改为宣告 AUTH PLAIN 并拒绝认证（见 newFakeSMTPRejectingAuth），
// 用于覆盖「535 认证失败」这条最常被误判为服务端 bug 的路径。
type fakeSMTP struct {
	listener net.Listener

	// authReject 非空时：EHLO 宣告 AUTH PLAIN，且对 AUTH 命令返回这一行。
	authReject string
	// authEcho 为真时把收到的 AUTH 命令行原样拼进拒绝响应里，
	// 用于验证「即使服务器回显了凭据，错误文本里也不得出现口令」。
	authEcho bool

	mu       sync.Mutex
	messages []string // 每封邮件的原始 DATA 内容
	authSeen bool
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听本地端口失败: %v", err)
	}
	server := &fakeSMTP{listener: ln}
	go server.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return server
}

// newFakeSMTPRejectingAuth 返回一个「宣告 AUTH PLAIN 但拒绝认证」的假服务器。
//
// line 是 AUTH 的响应行（例如 QQ 邮箱真实的 535 原文）。
func newFakeSMTPRejectingAuth(t *testing.T, line string) *fakeSMTP {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听本地端口失败: %v", err)
	}
	server := &fakeSMTP{listener: ln, authReject: line}
	go server.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return server
}

// addr 返回服务器监听的地址与端口。
func (s *fakeSMTP) addr() (string, int) {
	host, portText, err := net.SplitHostPort(s.listener.Addr().String())
	if err != nil {
		return "127.0.0.1", 0
	}
	port := 0
	_, _ = fmt.Sscanf(portText, "%d", &port)
	return host, port
}

func (s *fakeSMTP) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeSMTP) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	reply := func(line string) {
		_, _ = writer.WriteString(line + "\r\n")
		_ = writer.Flush()
	}

	reply("220 fake.smtp ESMTP ready")

	var body strings.Builder
	inData := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		trimmed := strings.TrimRight(line, "\r\n")

		if inData {
			if trimmed == "." {
				inData = false
				s.mu.Lock()
				s.messages = append(s.messages, body.String())
				s.mu.Unlock()
				body.Reset()
				reply("250 OK: queued")
				continue
			}
			// 还原 dot-stuffing（以 "." 开头的正文行在传输时会多一个点）。
			body.WriteString(strings.TrimPrefix(trimmed, "."))
			body.WriteString("\r\n")
			continue
		}

		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			if s.authReject != "" {
				// 宣告 AUTH PLAIN，客户端才会真的发起认证。
				// 多行响应：首行是欢迎语，随后逐行列出扩展能力，最后一行以空格开头结束。
				_, _ = writer.WriteString("250-fake.smtp\r\n250-AUTH PLAIN\r\n250 OK\r\n")
				_ = writer.Flush()
				continue
			}
			reply("250 fake.smtp")
		case strings.HasPrefix(upper, "AUTH"):
			s.mu.Lock()
			s.authSeen = true
			s.mu.Unlock()
			if s.authReject == "" {
				reply("502 不支持认证")
				continue
			}
			// authEcho 模拟「网关把整行 AUTH 请求回显进错误文本」这种最坏情况。
			if s.authEcho {
				reply(s.authReject + " (echo: " + trimmed + ")")
				continue
			}
			reply(s.authReject)
		case strings.HasPrefix(upper, "MAIL FROM"), strings.HasPrefix(upper, "RCPT TO"):
			reply("250 OK")
		case strings.HasPrefix(upper, "DATA"):
			inData = true
			reply("354 End data with <CR><LF>.<CR><LF>")
		case strings.HasPrefix(upper, "QUIT"):
			reply("221 Bye")
			return
		default:
			reply("250 OK")
		}
	}
}

// last 返回最近一封邮件的原始内容。
func (s *fakeSMTP) last() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) == 0 {
		return ""
	}
	return s.messages[len(s.messages)-1]
}

func TestSMTPMailerDeliversCompliantMessage(t *testing.T) {
	server := newFakeSMTP(t)
	host, port := server.addr()

	mailer, err := New(config.Mail{
		Driver:      "smtp",
		FromName:    "JobHub 直招",
		FromAddress: "no-reply@example.com",
		SMTP:        config.SMTP{Host: host, Port: port, TLS: false},
	})
	if err != nil {
		t.Fatalf("构造 SMTP 投递器失败: %v", err)
	}
	if mailer.Name() != "smtp" {
		t.Fatalf("驱动名应为 smtp，实际 %q", mailer.Name())
	}

	text := "您的验证码是 135790，10 分钟内有效。"
	html := "<p>您的验证码是 <strong>135790</strong></p>"
	msg := Message{
		To:      "user@example.com",
		Subject: "【JobHub 直招】注册验证码",
		Text:    text,
		HTML:    html,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := mailer.Send(ctx, msg); err != nil {
		t.Fatalf("发送邮件失败: %v", err)
	}

	raw := server.last()
	if raw == "" {
		t.Fatal("假 SMTP 服务器没有收到任何报文")
	}

	// —— 头部合规性 ——
	for _, want := range []string{
		"To: user@example.com",
		"no-reply@example.com",
		"multipart/alternative",
		"MIME-Version: 1.0",
		"Content-Transfer-Encoding: base64",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("报文中应包含 %q\n%s", want, raw)
		}
	}

	// 中文主题必须按 RFC 2047 编码，否则很多邮件网关会显示乱码。
	if !strings.Contains(raw, "Subject: =?UTF-8?") {
		t.Errorf("中文主题应使用 RFC 2047 编码，实际报文:\n%s", raw)
	}
	if strings.Contains(raw, "Subject: 【JobHub 直招】") {
		t.Error("主题不应以原始中文直接出现在头部")
	}

	// —— 正文编码：base64 解码后应能还原原文 ——
	decodedText := decodeBase64Blocks(t, raw)
	if !strings.Contains(decodedText, text) {
		t.Errorf("纯文本正文解码后应包含原文，实际:\n%s", decodedText)
	}
	if !strings.Contains(decodedText, "135790") {
		t.Errorf("HTML 正文解码后应包含验证码，实际:\n%s", decodedText)
	}

	// —— 行长限制：RFC 5322 规定每行不超过 998 字节 ——
	for i, line := range strings.Split(raw, "\r\n") {
		if len(line) > 998 {
			t.Fatalf("第 %d 行超过 998 字节（%d），邮件网关可能拒绝", i+1, len(line))
		}
	}
}

// decodeBase64Blocks 按块解码所有 base64 正文，并拼接为一串便于断言。
//
// 注意必须逐块解码：multipart 的每个部分各自独立 base64，
// 直接把两块的编码文本首尾相接会破坏 4 字节对齐而解码失败。
func decodeBase64Blocks(t *testing.T, raw string) string {
	t.Helper()

	var out strings.Builder
	var current strings.Builder
	inBody := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(current.String())
		if err != nil {
			t.Fatalf("base64 解码失败: %v", err)
		}
		out.Write(decoded)
		out.WriteString("\n")
		current.Reset()
	}

	for _, line := range strings.Split(raw, "\r\n") {
		switch {
		case strings.HasPrefix(line, "Content-Transfer-Encoding: base64"):
			inBody = true
		case strings.HasPrefix(line, "--"):
			// multipart 边界：结束当前块
			flush()
			inBody = false
		case inBody && line != "":
			current.WriteString(line)
		}
	}
	flush()

	return out.String()
}

func TestNewRejectsIncompleteSMTPConfig(t *testing.T) {
	cases := []config.Mail{
		{Driver: "smtp"},
		{Driver: "smtp", SMTP: config.SMTP{Host: "smtp.example.com"}},
		{Driver: "smtp", SMTP: config.SMTP{Host: "smtp.example.com", Port: 465}},
		{Driver: "unknown"},
	}
	for i, cfg := range cases {
		if _, err := New(cfg); err == nil {
			t.Errorf("第 %d 组不完整配置应返回错误: %+v", i+1, cfg)
		}
	}
}

func TestConsoleMailerDoesNotSend(t *testing.T) {
	mailer, err := New(config.Mail{Driver: "console"})
	if err != nil {
		t.Fatalf("构造 console 投递器失败: %v", err)
	}
	if mailer.Name() != "console" {
		t.Fatalf("驱动名应为 console，实际 %q", mailer.Name())
	}
	if err := mailer.Send(context.Background(), Message{To: "a@example.com", Subject: "s", Text: "t"}); err != nil {
		t.Errorf("console 驱动打印日志即可，不应报错: %v", err)
	}
}

func TestDynamicSwapsMailerAtRuntime(t *testing.T) {
	// 未配置任何投递器时应给出可执行的提示，而不是崩溃。
	empty := NewDynamic(nil)
	if empty.Name() != "none" {
		t.Errorf("未装载时的驱动名应为 none，实际 %q", empty.Name())
	}
	if err := empty.Send(context.Background(), Message{To: "a@example.com"}); err == nil {
		t.Error("未装载投递器时应返回错误")
	}

	console, err := New(config.Mail{Driver: "console"})
	if err != nil {
		t.Fatalf("构造 console 失败: %v", err)
	}
	empty.Swap(console)

	if empty.Name() != "console" {
		t.Errorf("替换后驱动名应为 console，实际 %q", empty.Name())
	}
	if err := empty.Send(context.Background(), Message{To: "a@example.com", Subject: "s", Text: "t"}); err != nil {
		t.Errorf("替换后应可发送: %v", err)
	}
}

func TestTestMessageCarriesContext(t *testing.T) {
	at := time.Date(2025, 1, 8, 10, 30, 0, 0, time.Local)
	msg := TestMessage("admin@example.com", "smtp", at)

	if msg.To != "admin@example.com" {
		t.Errorf("收件人不正确: %s", msg.To)
	}
	for _, want := range []string{"2025-01-08 10:30:00", "smtp"} {
		if !strings.Contains(msg.Text, want) {
			t.Errorf("测试邮件正文应包含 %q，实际:\n%s", want, msg.Text)
		}
	}
	if !strings.Contains(msg.Subject, "测试") {
		t.Errorf("测试邮件主题应可辨识，实际 %q", msg.Subject)
	}
}
