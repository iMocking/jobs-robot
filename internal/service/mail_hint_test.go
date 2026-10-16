package service

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/mail"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// qqRawResponse 是服务器原始响应文本的形态：Go 的 textproto 会把响应消息体
// 按 %q 格式化，因此运维看到的是带引号的 `535 "Login fail. ..."`。
const qqRawResponse = `535 "Login fail. Account is abnormal, service is not open, password is incorrect, login frequency limited, or system is busy. More information at https://help.mail.qq.com/detail/108/1023"`

// qqAuthError 构造一个与真实链路完全同形的认证失败错误。
func qqAuthError() *mail.AuthError {
	return &mail.AuthError{Host: "smtp.qq.com", Code: 535, Raw: qqRawResponse}
}

// TestSMTPHostHint 锁定「主机名 → 服务商专属提示」的映射。
func TestSMTPHostHint(t *testing.T) {
	cases := []struct {
		name         string
		host         string
		wantContains []string
		wantEmpty    bool
	}{
		{
			name: "个人 QQ 邮箱",
			host: "smtp.qq.com",
			wantContains: []string{
				"QQ 邮箱",
				"设置 → 账户",
				"POP3/IMAP/SMTP",
				"授权码",
			},
		},
		{
			name: "腾讯企业邮（先于 qq.com 匹配）",
			host: "smtp.exmail.qq.com",
			wantContains: []string{
				"腾讯企业邮",
				"客户端专用密码",
				"互不通用",
			},
		},
		{
			name:         "主机名带大小写与空白也要能匹配",
			host:         "  SMTP.Exmail.QQ.COM  ",
			wantContains: []string{"腾讯企业邮"},
		},
		{
			name:         "163 邮箱",
			host:         "smtp.163.com",
			wantContains: []string{"网易邮箱", "POP3/SMTP/IMAP", "客户端授权密码"},
		},
		{
			name:         "126 邮箱",
			host:         "smtp.126.com",
			wantContains: []string{"网易邮箱", "客户端授权密码"},
		},
		{
			name:         "Gmail",
			host:         "smtp.gmail.com",
			wantContains: []string{"Gmail", "两步验证", "应用专用密码", "OAuth"},
		},
		{
			name:         "Outlook / Microsoft 365",
			host:         "smtp.office365.com",
			wantContains: []string{"Microsoft 365", "完整邮箱地址", "SMTP AUTH"},
		},
		{
			name:         "outlook.com 同样命中",
			host:         "smtp.outlook.com",
			wantContains: []string{"Outlook"},
		},
		{
			name:      "未知服务商只给通用四条",
			host:      "smtp.example.com",
			wantEmpty: true,
		},
		{
			name:      "主机名为空",
			host:      "",
			wantEmpty: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := smtpHostHint(tc.host)
			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("不应给出专属提示，实际: %s", got)
				}
				return
			}
			if got == "" {
				t.Fatal("应给出专属提示，实际为空")
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("提示应包含 %q，实际: %s", want, got)
				}
			}
		})
	}
}

// TestSMTPAuthFailureMessageKeepsRawResponseAndSteps 覆盖用户上报的第一种故障。
func TestSMTPAuthFailureMessageKeepsRawResponseAndSteps(t *testing.T) {
	msg := smtpAuthFailureMessage("smtp.qq.com", 535, qqRawResponse)

	for _, want := range []string{
		"SMTP 认证失败（服务器返回 535）",
		"授权码",
		"开启 SMTP/POP3 服务",
		"完整邮箱地址",
		"smtp.qq.com:465",
		"QQ 邮箱",
		"服务器原文：" + qqRawResponse,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message 应包含 %q，实际:\n%s", want, msg)
		}
	}

	// 关键回归点：不得再把「服务器内部错误」与内部错误码甩给运维。
	for _, banned := range []string{"服务器内部错误", "10007"} {
		if strings.Contains(msg, banned) {
			t.Errorf("message 不得包含 %q，实际:\n%s", banned, msg)
		}
	}
}

// TestSMTPConnectFailureMessage 覆盖「端口与加密方式不匹配」等四类连接失败。
func TestSMTPConnectFailureMessage(t *testing.T) {
	cases := []struct {
		name         string
		connErr      *mail.ConnectError
		wantContains []string
	}{
		{
			name: "465 选了 STARTTLS/不加密（EOF）",
			connErr: &mail.ConnectError{
				Kind: mail.ConnectTLSMismatch, Host: "smtp.qq.com", Port: 465, TLS: false, Raw: "EOF",
			},
			wantContains: []string{
				"无法建立 SMTP 连接（EOF）：端口与加密方式很可能不匹配",
				"465 端口必须选「SSL/TLS 直连」（隐式 TLS）",
				"587 端口必须选「STARTTLS」",
				"25 端口通常被云厂商与运营商封禁",
				"当前提交：smtp.qq.com:465，加密方式=STARTTLS 或不加密（tls=false）",
				"该组合下服务器会因收到明文而立即断开",
				"服务器原文：EOF",
			},
		},
		{
			name: "587 选成了 SSL/TLS 直连",
			connErr: &mail.ConnectError{
				Kind: mail.ConnectTLSMismatch, Host: "smtp.office365.com", Port: 587, TLS: true,
				Raw: "tls: first record does not look like a TLS handshake",
			},
			wantContains: []string{
				"端口与加密方式很可能不匹配",
				"当前提交：smtp.office365.com:587，加密方式=SSL/TLS 直连（tls=true）",
				"587 端口只接受先明文再升级 TLS",
				"服务器原文：tls: first record does not look like a TLS handshake",
			},
		},
		{
			name: "DNS 解析失败",
			connErr: &mail.ConnectError{
				Kind: mail.ConnectHostUnresolved, Host: "smtp.qq.con", Port: 465, TLS: true,
				Raw: "dial tcp: lookup smtp.qq.con: no such host",
			},
			wantContains: []string{
				"无法解析 SMTP 主机名 smtp.qq.con：请检查主机名是否拼写正确",
				"当前提交：smtp.qq.con:465，加密方式=SSL/TLS 直连（tls=true）",
				"服务器原文：dial tcp: lookup smtp.qq.con: no such host",
			},
		},
		{
			name: "端口拒绝",
			connErr: &mail.ConnectError{
				Kind: mail.ConnectRefused, Host: "smtp.qq.com", Port: 2525, TLS: true,
				Raw: "dial tcp 1.2.3.4:2525: connect: connection refused",
			},
			wantContains: []string{
				"SMTP 服务器拒绝连接：请检查端口（465/587）",
				"当前提交：smtp.qq.com:2525，加密方式=SSL/TLS 直连（tls=true）",
			},
		},
		{
			name: "连接超时",
			connErr: &mail.ConnectError{
				Kind: mail.ConnectTimeout, Host: "smtp.qq.com", Port: 25, TLS: false,
				Raw: "dial tcp 1.2.3.4:25: i/o timeout",
			},
			wantContains: []string{
				"连接 SMTP 服务器超时：请检查网络/安全组是否放行出站 465/587",
				"部分云主机默认封禁 25 端口",
				"当前提交：smtp.qq.com:25，加密方式=不加密（tls=false）",
			},
		},
		{
			name: "无法归因也直说连不上",
			connErr: &mail.ConnectError{
				Kind: mail.ConnectUnknown, Host: "smtp.example.com", Port: 465, TLS: true,
				Raw: "smtp: bad greeting",
			},
			wantContains: []string{
				"无法连接 SMTP 服务器：smtp: bad greeting",
				"当前提交：smtp.example.com:465，加密方式=SSL/TLS 直连（tls=true）",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := smtpConnectFailureMessage(tc.connErr)
			for _, want := range tc.wantContains {
				if !strings.Contains(msg, want) {
					t.Errorf("message 应包含 %q，实际:\n%s", want, msg)
				}
			}
			for _, banned := range []string{"服务器内部错误", "10007"} {
				if strings.Contains(msg, banned) {
					t.Errorf("message 不得包含 %q，实际:\n%s", banned, msg)
				}
			}
		})
	}
}

// TestDescribeMailFailureCodes 锁定对外错误码与 HTTP 状态码。
func TestDescribeMailFailureCodes(t *testing.T) {
	authBiz := describeMailFailure("测试邮件发送失败", qqAuthError())
	if authBiz.Code != apperr.CodeMailAuthFailed {
		t.Errorf("认证失败应返回 %d，实际 %d", apperr.CodeMailAuthFailed, authBiz.Code)
	}
	if authBiz.HTTPStatus != 503 {
		t.Errorf("认证失败应保持 HTTP 503（前端按 503 展示 message），实际 %d", authBiz.HTTPStatus)
	}

	connBiz := describeMailFailure("测试邮件发送失败", &mail.ConnectError{
		Kind: mail.ConnectTLSMismatch, Host: "smtp.qq.com", Port: 465, Raw: "EOF",
	})
	if connBiz.Code != apperr.CodeUnavailable {
		t.Errorf("连接失败应沿用 %d，实际 %d", apperr.CodeUnavailable, connBiz.Code)
	}
	if connBiz.HTTPStatus != 503 {
		t.Errorf("连接失败应保持 HTTP 503，实际 %d", connBiz.HTTPStatus)
	}
}

// TestDescribeMailFailureStripsInternalPrefix 保证「其它失败」也不再带内部前缀。
func TestDescribeMailFailureStripsInternalPrefix(t *testing.T) {
	// 模拟底层包用 10007 包过一层的历史写法。
	wrapped := apperr.ErrInternal.WithCause(errors.New("SMTP RCPT TO 失败: 550 relay access denied"))
	biz := describeMailFailure("测试邮件发送失败", wrapped)

	if biz.Code != apperr.CodeUnavailable {
		t.Errorf("应返回 %d，实际 %d", apperr.CodeUnavailable, biz.Code)
	}
	if strings.Contains(biz.Message, "服务器内部错误") || strings.Contains(biz.Message, "10007") {
		t.Errorf("message 不得包含内部错误前缀，实际: %s", biz.Message)
	}
	if !strings.Contains(biz.Message, "550 relay access denied") {
		t.Errorf("message 应保留真实原因，实际: %s", biz.Message)
	}
}

/* -------------------------------------------------------------------------- */
/* 端到端：真实 SMTP 会话 → SendTest → 对外 message                            */
/* -------------------------------------------------------------------------- */

// startClosingServer 模拟「对端在 TLS 握手前直接关闭连接」。
func startClosingServer(t *testing.T) (string, int) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听本地端口失败: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return splitHostPort(t, ln.Addr().String())
}

// startAuthRejectServer 是「宣告 AUTH PLAIN 但回 535」的最小 SMTP 服务器。
//
// 为什么 service 包里另有一个：internal/mail 的假服务器写在 _test.go 中，
// Go 不允许跨包复用测试文件里的辅助类型。这里只实现认证失败必需的两条命令
// （EHLO / AUTH），完整投递流程的覆盖仍在 internal/mail。
func startAuthRejectServer(t *testing.T, rejectLine string) (string, int) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听本地端口失败: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveAuthReject(conn, rejectLine)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return splitHostPort(t, ln.Addr().String())
}

func serveAuthReject(conn net.Conn, rejectLine string) {
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	reply := func(line string) {
		_, _ = writer.WriteString(line + "\r\n")
		_ = writer.Flush()
	}

	reply("220 fake.smtp ESMTP ready")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		upper := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			_, _ = writer.WriteString("250-fake.smtp\r\n250-AUTH PLAIN\r\n250 OK\r\n")
			_ = writer.Flush()
		case strings.HasPrefix(upper, "AUTH"):
			reply(rejectLine)
		case strings.HasPrefix(upper, "QUIT"):
			reply("221 Bye")
			return
		default:
			reply("250 OK")
		}
	}
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()

	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("解析监听地址失败: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("解析端口失败: %v", err)
	}
	return host, port
}

// newTestMailService 构造一个只依赖投递器的 MailService。
//
// Store 为 nil 时 Audit.Record 会直接返回（审计是旁路能力），
// 因此测试不需要真实 PostgreSQL，也不会污染任何数据。
func newTestMailService(t *testing.T, host string, port int, tls bool, username, password string) *MailService {
	t.Helper()

	mailer, err := mail.New(config.Mail{
		Driver:      "smtp",
		FromName:    "JobHub 直招",
		FromAddress: "ops@example.com",
		SMTP: config.SMTP{
			Host: host, Port: port, Username: username, Password: password, TLS: tls,
		},
	})
	if err != nil {
		t.Fatalf("构造 SMTP 投递器失败: %v", err)
	}

	svc := New(Params{Cfg: &config.Config{}, Mailer: mail.NewDynamic(mailer)})
	return svc.Mail
}

func TestSendTestReportsAuthFailureWithSteps(t *testing.T) {
	const secret = "abcd1234efgh5678"

	host, port := startAuthRejectServer(t, "535 Login fail. Account is abnormal, service is not open, password is incorrect, login frequency limited, or system is busy.")
	svc := newTestMailService(t, host, port, false, "ops@qq.com", secret)

	err := svc.SendTest(context.Background(), "ops@qq.com", Actor{Email: "ops@qq.com"})
	if err == nil {
		t.Fatal("认证失败时 SendTest 必须返回错误")
	}

	biz, ok := apperr.As(err)
	if !ok {
		t.Fatalf("应返回业务错误，实际 %T: %v", err, err)
	}
	if biz.Code != apperr.CodeMailAuthFailed {
		t.Errorf("错误码应为 %d，实际 %d", apperr.CodeMailAuthFailed, biz.Code)
	}
	if biz.HTTPStatus != 503 {
		t.Errorf("HTTP 状态应为 503，实际 %d", biz.HTTPStatus)
	}
	for _, want := range []string{"SMTP 认证失败（服务器返回 535）", "授权码", "服务器原文："} {
		if !strings.Contains(biz.Message, want) {
			t.Errorf("message 应包含 %q，实际:\n%s", want, biz.Message)
		}
	}
	for _, banned := range []string{"服务器内部错误", "10007", secret} {
		if strings.Contains(biz.Message, banned) {
			t.Errorf("message 不得包含 %q，实际:\n%s", banned, biz.Message)
		}
	}
}

func TestSendTestReportsTLSMismatchWithCurrentTarget(t *testing.T) {
	const secret = "zzzz9999yyyy8888"

	// 真实模拟「465 端口选了 STARTTLS」：对端在收到明文后直接断开 → EOF。
	host, port := startClosingServer(t)
	svc := newTestMailService(t, host, port, false, "ops@qq.com", secret)

	err := svc.SendTest(context.Background(), "ops@qq.com", Actor{Email: "ops@qq.com"})
	if err == nil {
		t.Fatal("连接失败时 SendTest 必须返回错误")
	}

	biz, ok := apperr.As(err)
	if !ok {
		t.Fatalf("应返回业务错误，实际 %T: %v", err, err)
	}
	if biz.Code != apperr.CodeUnavailable || biz.HTTPStatus != 503 {
		t.Errorf("错误码/状态应为 10008/503，实际 %d/%d", biz.Code, biz.HTTPStatus)
	}
	for _, want := range []string{
		"端口与加密方式很可能不匹配",
		"当前提交：",
		"服务器原文：EOF",
		"SSL/TLS 直连",
	} {
		if !strings.Contains(biz.Message, want) {
			t.Errorf("message 应包含 %q，实际:\n%s", want, biz.Message)
		}
	}
	for _, banned := range []string{"服务器内部错误", "10007", secret} {
		if strings.Contains(biz.Message, banned) {
			t.Errorf("message 不得包含 %q，实际:\n%s", banned, biz.Message)
		}
	}
}

// TestTrimMailSecret 锁定「首尾空白自动去除」这一行为（而不是拒绝保存）。
//
// 复制授权码时最常见的产物就是尾随空格/换行；自动去除让两条路径
// （后台邮件设置、安装向导）行为一致，也让运维不必为此重填一次。
func TestTrimMailSecret(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "尾随空格", in: "abcd1234efgh5678 ", want: "abcd1234efgh5678"},
		{name: "前置空格", in: " abcd1234efgh5678", want: "abcd1234efgh5678"},
		{name: "尾随换行与制表符", in: "abcd1234efgh5678\r\n\t", want: "abcd1234efgh5678"},
		{name: "不间断空格（网页复制常见）", in: "\u00a0abcd1234efgh5678\u00a0", want: "abcd1234efgh5678"},
		{name: "本身干净则原样返回", in: "abcd1234efgh5678", want: "abcd1234efgh5678"},
		{name: "全空白收敛为空", in: "   ", want: ""},
		{name: "空串", in: "", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trimMailSecret(tc.in, "smtp.qq.com", "ops@qq.com"); got != tc.want {
				t.Errorf("应返回 %q，实际 %q", tc.want, got)
			}
		})
	}
}

// TestDocumentedMessages 把最终会展示给运维的两段文案打印出来。
//
// 断言之外还刻意 t.Logf：这两段文本是对外契约（README 与 docs/api.md 都引用它们），
// 让它出现在 `go test -v` 的输出里，改文案时能一眼看到 diff。
func TestDocumentedMessages(t *testing.T) {
	auth := smtpAuthFailureMessage("smtp.qq.com", 535, qqRawResponse)
	conn := smtpConnectFailureMessage(&mail.ConnectError{
		Kind: mail.ConnectTLSMismatch, Host: "smtp.qq.com", Port: 465, TLS: false, Raw: "EOF",
	})

	t.Logf("\n【认证失败 535 的完整 message】\n%s\n", auth)
	t.Logf("\n【端口与加密方式错配的完整 message】\n%s\n", conn)

	if auth == "" || conn == "" {
		t.Fatal("文案不能为空")
	}
}
