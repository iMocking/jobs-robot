package mail

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/jsong/jobs-robot/internal/config"
)

// closeOnAcceptListener 模拟「对端在 TLS 握手前直接关闭连接」。
//
// 这正是「465 端口选成 STARTTLS / 不加密」时 QQ 邮箱的行为：
// 服务器在等 TLS 客户端问候，等到的却是明文，于是直接关连接，
// 客户端只能读到 EOF。用它做测试比编造一个错误字符串更接近真实链路。
func closeOnAcceptListener(t *testing.T) net.Listener {
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
	return ln
}

// TestSMTPMailerClassifiesTLSMismatchAsEOF 覆盖用户上报的第二种真实故障：
//
//	{"code":10008,"message":"测试邮件发送失败：[10007] 服务器内部错误: 连接 SMTP 服务器失败: EOF"}
//
// 端口/加密方式错配必须被识别成 ErrTLSMismatch（而非笼统的连接失败），
// 并且错误里要带上本次提交的 host/port/tls，服务层才能生成
// 「当前提交：smtp.qq.com:465，加密方式=STARTTLS」这句定位提示。
func TestSMTPMailerClassifiesTLSMismatchAsEOF(t *testing.T) {
	const secret = "abcd1234efgh5678"

	ln := closeOnAcceptListener(t)
	host, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("解析监听地址失败: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("解析端口失败: %v", err)
	}

	mailer, err := New(config.Mail{
		Driver:      "smtp",
		FromAddress: "ops@qq.com",
		SMTP: config.SMTP{
			Host:     host,
			Port:     port,
			Username: "ops@qq.com",
			Password: secret,
			// 故意用非隐式 TLS：对应界面上选了「STARTTLS / 不加密」
			TLS: false,
		},
	})
	if err != nil {
		t.Fatalf("构造 SMTP 投递器失败: %v", err)
	}

	sendErr := sendOnce(t, mailer)
	if sendErr == nil {
		t.Fatal("对端关闭连接时必须返回错误")
	}

	if !errors.Is(sendErr, ErrConnectFailed) {
		t.Errorf("错误应满足 errors.Is(err, ErrConnectFailed)，实际: %v", sendErr)
	}
	if !errors.Is(sendErr, ErrTLSMismatch) {
		t.Errorf("EOF 应判定为端口/加密方式不匹配，实际: %v", sendErr)
	}

	var connErr *ConnectError
	if !errors.As(sendErr, &connErr) {
		t.Fatalf("错误应可用 errors.As 取出 *ConnectError，实际 %T: %v", sendErr, sendErr)
	}
	if connErr.Kind != ConnectTLSMismatch {
		t.Errorf("归因应为 %q，实际 %q", ConnectTLSMismatch, connErr.Kind)
	}
	if connErr.Raw != "EOF" {
		t.Errorf("服务器原文应为 EOF，实际 %q", connErr.Raw)
	}
	// 当前提交的三个参数必须被带上：文案里要回显它们。
	if connErr.Host != host || connErr.Port != port || connErr.TLS {
		t.Errorf("应带上本次提交的 host/port/tls，实际 host=%q port=%d tls=%v",
			connErr.Host, connErr.Port, connErr.TLS)
	}
	if strings.Contains(sendErr.Error(), secret) {
		t.Errorf("错误文本不得包含 SMTP 口令，实际: %s", sendErr)
	}
}

// TestSMTPMailerClassifiesPlaintextServerAsTLSMismatch 覆盖反方向错配：
// 选了「SSL/TLS 直连」却把端口填成明文端口（服务器回的是 220 欢迎语，
// 不是 TLS 记录），Go 的 TLS 客户端会报 first record does not look like a TLS handshake。
//
// 这里复用已存在的假 SMTP 服务器（它在 220 里明说 ESMTP），不另写一套。
func TestSMTPMailerClassifiesPlaintextServerAsTLSMismatch(t *testing.T) {
	server := newFakeSMTP(t)
	host, port := server.addr()

	mailer, err := New(config.Mail{
		Driver:      "smtp",
		FromAddress: "ops@example.com",
		SMTP: config.SMTP{
			Host: host,
			Port: port,
			// 隐式 TLS：整条连接按 TLS 处理
			TLS: true,
		},
	})
	if err != nil {
		t.Fatalf("构造 SMTP 投递器失败: %v", err)
	}

	sendErr := sendOnce(t, mailer)
	if sendErr == nil {
		t.Fatal("对端是明文服务器时必须返回错误")
	}
	if !errors.Is(sendErr, ErrTLSMismatch) {
		t.Errorf("明文服务器 + 隐式 TLS 应判定为端口/加密方式不匹配，实际: %v", sendErr)
	}

	var connErr *ConnectError
	if !errors.As(sendErr, &connErr) {
		t.Fatalf("错误应可用 errors.As 取出 *ConnectError，实际 %T", sendErr)
	}
	if !strings.Contains(connErr.Raw, "TLS handshake") {
		t.Errorf("服务器原文应包含 TLS handshake 相关说明，实际 %q", connErr.Raw)
	}
	if !connErr.TLS {
		t.Error("归因结果应带上 tls=true")
	}
}

// TestClassifyConnectKind 锁定「错误特征 → 归因」的映射（服务层文案完全依赖它）。
func TestClassifyConnectKind(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantKind ConnectKind
		wantSent error
	}{
		{name: "EOF", raw: "EOF", wantKind: ConnectTLSMismatch, wantSent: ErrTLSMismatch},
		{name: "unexpected EOF", raw: "unexpected EOF", wantKind: ConnectTLSMismatch, wantSent: ErrTLSMismatch},
		{
			name:     "连接被重置",
			raw:      "read tcp 10.0.0.1:51000->1.2.3.4:465: read: connection reset by peer",
			wantKind: ConnectTLSMismatch,
			wantSent: ErrTLSMismatch,
		},
		{
			name:     "broken pipe",
			raw:      "write tcp 10.0.0.1:51000->1.2.3.4:465: write: broken pipe",
			wantKind: ConnectTLSMismatch,
			wantSent: ErrTLSMismatch,
		},
		{
			name:     "TLS 协议错配",
			raw:      "tls: first record does not look like a TLS handshake",
			wantKind: ConnectTLSMismatch,
			wantSent: ErrTLSMismatch,
		},
		{
			name:     "DNS 解析失败",
			raw:      "dial tcp: lookup smtp.qq.con: no such host",
			wantKind: ConnectHostUnresolved,
			wantSent: ErrHostUnresolved,
		},
		{
			name:     "端口拒绝",
			raw:      "dial tcp 1.2.3.4:2525: connect: connection refused",
			wantKind: ConnectRefused,
			wantSent: ErrConnectRefused,
		},
		{
			name:     "连接超时",
			raw:      "dial tcp 1.2.3.4:465: i/o timeout",
			wantKind: ConnectTimeout,
			wantSent: ErrConnectTimeout,
		},
		{
			name:     "整体超时",
			raw:      "context deadline exceeded",
			wantKind: ConnectTimeout,
			wantSent: ErrConnectTimeout,
		},
		{
			name:     "无法归因时保持未知",
			raw:      "smtp: bad greeting",
			wantKind: ConnectUnknown,
			wantSent: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connErr := newConnectError(errors.New(tc.raw), "", "smtp.qq.com", 465, false)
			if connErr.Kind != tc.wantKind {
				t.Errorf("归因应为 %q，实际 %q", tc.wantKind, connErr.Kind)
			}
			if !errors.Is(connErr, ErrConnectFailed) {
				t.Error("任何连接失败都应满足 errors.Is(err, ErrConnectFailed)")
			}
			if tc.wantSent == nil {
				if errors.Is(connErr, ErrTLSMismatch) || errors.Is(connErr, ErrHostUnresolved) ||
					errors.Is(connErr, ErrConnectRefused) || errors.Is(connErr, ErrConnectTimeout) {
					t.Error("未知归因不应匹配任何细分哨兵")
				}
				return
			}
			if !errors.Is(connErr, tc.wantSent) {
				t.Errorf("应满足 errors.Is(err, %v)", tc.wantSent)
			}
		})
	}
}

// TestNewConnectErrorKeepsDeadlineContext 顺带锁定：无法归因的错误也必须保留原文。
func TestNewConnectErrorKeepsDeadlineContext(t *testing.T) {
	connErr := newConnectError(errors.New("smtp: 服务器问候语超时"), "", "smtp.example.com", 587, true)
	if connErr.Raw != "smtp: 服务器问候语超时" {
		t.Errorf("原文不应被改写，实际 %q", connErr.Raw)
	}
	if !strings.Contains(connErr.Error(), "smtp.example.com:587") {
		t.Errorf("日志里应能看到目标地址，实际 %q", connErr.Error())
	}
}
