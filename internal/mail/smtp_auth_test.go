package mail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/jsong/jobs-robot/internal/config"
)

// qqAuthRejectLine 是 QQ 邮箱对「授权码不对 / POP3-SMTP 服务未开启 / 账号只填了前缀」
// 的真实回应（用户工单里的原文）。测试用真句子而不是编造的假句子：
// 分类规则与最终文案都是围绕这段文本设计的。
//
// 注意这里是**线上传输的那一行**（不带引号）：Go 的 textproto 会把消息体按 %q
// 格式化，因此运维最终看到的是 `535 "Login fail. ..."`（带引号）——与工单原文一致。
const qqAuthRejectLine = `535 Login fail. Account is abnormal, service is not open, password is incorrect, login frequency limited, or system is busy. More information at https://help.mail.qq.com/detail/108/1023`

// newAuthRejectingMailer 用假 SMTP 服务器构造一个必然认证失败的投递器。
func newAuthRejectingMailer(t *testing.T, server *fakeSMTP, username, password string) Mailer {
	t.Helper()

	host, port := server.addr()
	mailer, err := New(config.Mail{
		Driver:      "smtp",
		FromName:    "JobHub 直招",
		FromAddress: "ops@example.com",
		SMTP: config.SMTP{
			Host:     host,
			Port:     port,
			Username: username,
			Password: password,
			TLS:      false,
		},
	})
	if err != nil {
		t.Fatalf("构造 SMTP 投递器失败: %v", err)
	}
	return mailer
}

func sendOnce(t *testing.T, mailer Mailer) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return mailer.Send(ctx, Message{
		To:      "ops@example.com",
		Subject: "【JobHub 直招】邮件配置测试",
		Text:    "测试邮件",
	})
}

// TestSMTPMailerClassifiesAuthRejection 是本文件的核心用例：
// 服务器拒绝 AUTH 并返回 535 时，错误必须
//  1. 能被 errors.Is(err, ErrAuthFailed) 判定（上层据此换成可执行的排查提示）；
//  2. 保留服务器原文（535 与那段 Login fail. 文本都不能被吞掉）；
//  3. 不泄漏口令。
func TestSMTPMailerClassifiesAuthRejection(t *testing.T) {
	const secret = "abcd1234efgh5678" // 形如 QQ 授权码的 16 位口令

	server := newFakeSMTPRejectingAuth(t, qqAuthRejectLine)
	mailer := newAuthRejectingMailer(t, server, "ops@qq.com", secret)

	err := sendOnce(t, mailer)
	if err == nil {
		t.Fatal("认证被服务器拒绝时必须返回错误")
	}

	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("错误应满足 errors.Is(err, ErrAuthFailed)，实际 %T: %v", err, err)
	}

	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("错误应可用 errors.As 取出 *AuthError，实际 %T: %v", err, err)
	}
	if authErr.Code != 535 {
		t.Errorf("响应码应为 535，实际 %d", authErr.Code)
	}

	// 服务器原文必须原样保留（上层要展示给运维）。
	text := err.Error()
	if !strings.Contains(text, "535") {
		t.Errorf("错误文本应包含服务器响应码 535，实际: %s", text)
	}
	if !strings.Contains(text, "Login fail. Account is abnormal") {
		t.Errorf("错误文本应包含服务器原文，实际: %s", text)
	}
	if !strings.Contains(text, "help.mail.qq.com/detail/108/1023") {
		t.Errorf("错误文本不应截断服务器原文，实际: %s", text)
	}

	// 任何日志/审计/响应都不得出现口令明文。
	if strings.Contains(text, secret) {
		t.Errorf("错误文本不得包含 SMTP 口令，实际: %s", text)
	}
}

// TestSMTPAuthErrorRedactsEchoedCredential 覆盖最坏情况：服务器（或中间网关）
// 把整行 AUTH 请求回显进错误文本。此时明文与 base64 凭据都不允许出现在错误里。
func TestSMTPAuthErrorRedactsEchoedCredential(t *testing.T) {
	const (
		username = "ops@qq.com"
		secret   = "zzzz9999yyyy8888"
	)

	server := newFakeSMTPRejectingAuth(t, qqAuthRejectLine)
	server.authEcho = true
	mailer := newAuthRejectingMailer(t, server, username, secret)

	err := sendOnce(t, mailer)
	if err == nil {
		t.Fatal("认证被服务器拒绝时必须返回错误")
	}

	text := err.Error()
	// base64("\x00用户\x00口令") 正是 AUTH PLAIN 发出去的凭据。
	blob := base64.StdEncoding.EncodeToString([]byte("\x00" + username + "\x00" + secret))
	if !strings.Contains(text, "echo:") {
		t.Fatalf("本用例要求服务器确实回显了 AUTH 行，实际: %s", text)
	}
	if strings.Contains(text, secret) {
		t.Errorf("回显场景下错误文本仍不得包含口令明文，实际: %s", text)
	}
	if strings.Contains(text, blob) {
		t.Errorf("回显场景下错误文本仍不得包含 base64 凭据，实际: %s", text)
	}
	if !strings.Contains(text, redactMask) {
		t.Errorf("应当能看到脱敏掩码 %s，实际: %s", redactMask, text)
	}
}

// TestClassifyAuthFailure 锁定判定规则本身（响应码 + 文本特征两条路径）。
func TestClassifyAuthFailure(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantAuth bool
		wantCode int
	}{
		{
			name:     "535 响应码",
			err:      &textproto.Error{Code: 535, Msg: "Login fail."},
			wantAuth: true,
			wantCode: 535,
		},
		{name: "534 响应码", err: &textproto.Error{Code: 534, Msg: "need app password"}, wantAuth: true, wantCode: 534},
		{name: "530 响应码", err: &textproto.Error{Code: 530, Msg: "auth required"}, wantAuth: true, wantCode: 530},
		{name: "454 响应码", err: &textproto.Error{Code: 454, Msg: "temporary auth failure"}, wantAuth: true, wantCode: 454},
		{
			name:     "响应码被包装成普通错误但文本含 535",
			err:      fmt.Errorf("smtp: 服务器返回 535，请检查授权码"),
			wantAuth: true,
			wantCode: 535,
		},
		{
			name:     "文本含 login fail（大小写不敏感）",
			err:      errors.New("554 LOGIN FAIL: invalid credentials"),
			wantAuth: true,
			wantCode: 0,
		},
		{
			name:     "文本含 authentication",
			err:      errors.New("Authentication credentials invalid"),
			wantAuth: true,
			wantCode: 0,
		},
		{
			name:     "服务器不支持 AUTH 也算配置问题",
			err:      errors.New("smtp: server doesn't support AUTH"),
			wantAuth: true,
			wantCode: 0,
		},
		{
			name:     "连接被拒不属于认证失败",
			err:      errors.New("dial tcp 1.2.3.4:465: connect: connection refused"),
			wantAuth: false,
		},
		{
			name:     "较长数字串里的 535 不算响应码",
			err:      errors.New("message id 1535 rejected by relay"),
			wantAuth: false,
		},
		{
			name:     "收件人被拒不属于认证失败",
			err:      &textproto.Error{Code: 550, Msg: "relay access denied"},
			wantAuth: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyAuthFailure("smtp.qq.com", "ops@qq.com", "secret-code", tc.err)
			if tc.wantAuth {
				if got == nil {
					t.Fatalf("应判定为认证失败，实际为 nil（错误: %v）", tc.err)
				}
				if got.Code != tc.wantCode {
					t.Errorf("响应码应为 %d，实际 %d", tc.wantCode, got.Code)
				}
				if !errors.Is(got, ErrAuthFailed) {
					t.Error("返回值应满足 errors.Is(err, ErrAuthFailed)")
				}
				return
			}
			if got != nil {
				t.Errorf("不应判定为认证失败，实际 %+v", got)
			}
		})
	}
}
