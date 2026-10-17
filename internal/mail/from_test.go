package mail

import (
	"context"
	"strings"
	"testing"

	"github.com/jsong/jobs-robot/internal/config"
)

// 发件人显示名有两条要求：
//  1. 系统邮件统一使用品牌化系统通知名（config.mail.system_from_name）；
//  2. 该值为空时回落到 config.mail.from_name，两者都为空再回落 no-reply。
//
// 两者都必须按 RFC 2047 正确编码：ASCII 名可以原样输出，含中点/中文的名必须编码成 encoded-word。
func TestFormatAddressEncodesDisplayName(t *testing.T) {
	// 「·」是 U+00B7 中点，不是 ASCII 句点；它属于非 ASCII 字符，必须进入 RFC 2047 编码分支。
	name := "直招好工作 · 系统通知"
	got := formatAddress(name, "no-reply@example.com")
	t.Logf("构造出的 From 头：From: %s", got)
	if !strings.HasPrefix(got, "=?UTF-8?b?") {
		t.Fatalf("含中点与中文的显示名必须按 RFC 2047 编码，实际 %q", got)
	}
	if !strings.HasSuffix(got, "?= <no-reply@example.com>") {
		t.Fatalf("编码后的地址格式不对，实际 %q", got)
	}

	ascii := formatAddress("Jobs Mail Robot", "no-reply@example.com")
	if ascii != "Jobs Mail Robot <no-reply@example.com>" {
		t.Fatalf("纯 ASCII 显示名不应被多余编码，实际 %q", ascii)
	}
}

func TestEffectiveSystemFromNameFallbacks(t *testing.T) {
	cases := []struct {
		name string
		mail config.Mail
		want string
	}{
		{"system_from_name 优先", config.Mail{SystemFromName: "直招好工作 · 系统通知", FromName: "旧品牌"}, "直招好工作 · 系统通知"},
		{"system_from_name 为空回落 from_name", config.Mail{FromName: "直招好工作"}, "直招好工作"},
		{"两者都为空回落 no-reply", config.Mail{}, "no-reply"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.mail.EffectiveSystemFromName(); got != tc.want {
				t.Fatalf("EffectiveSystemFromName = %q，期望 %q", got, tc.want)
			}
		})
	}
}

func TestSMTPBuildUsesMessageFromNameOverride(t *testing.T) {
	m := &smtpMailer{from: "no-reply@example.com", fromName: "直招好工作"}

	raw := string(m.build(Message{
		To:       "user@example.com",
		Subject:  "subject",
		Text:     "text",
		FromName: "直招好工作 · 系统通知",
	}))
	if !strings.Contains(raw, "From: =?UTF-8?b?") {
		t.Fatalf("消息级 FromName 未被使用或未编码，报文片段：%q", firstHeaderLines(raw, 8))
	}

	// 未设置覆盖值时回落投递器自身的 from_name。
	fallback := string(m.build(Message{To: "user@example.com", Subject: "subject", Text: "text"}))
	if !strings.Contains(fallback, "From: =?UTF-8?b?") {
		t.Fatalf("未设置覆盖值时应回落到配置的中文 from_name，报文片段：%q", firstHeaderLines(fallback, 8))
	}
}

func TestDynamicInjectsSystemFromName(t *testing.T) {
	fake := &fakeMailer{}
	d := NewDynamic(fake)
	d.SetFromName("直招好工作 · 系统通知")

	if err := d.Send(context.Background(), Message{To: "user@example.com"}); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if fake.last.FromName != "直招好工作 · 系统通知" {
		t.Fatalf("Dynamic 未注入系统发件人显示名，实际 %q", fake.last.FromName)
	}

	// 消息自带的显式值优先于包装器设置（例如将来某个特殊场景需要不同显示名）。
	if err := d.Send(context.Background(), Message{To: "user@example.com", FromName: "Custom"}); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if fake.last.FromName != "Custom" {
		t.Fatalf("消息级 FromName 应优先，实际 %q", fake.last.FromName)
	}
}

type fakeMailer struct {
	last Message
}

func (f *fakeMailer) Send(_ context.Context, msg Message) error {
	f.last = msg
	return nil
}

func (f *fakeMailer) Name() string { return "fake" }

func firstHeaderLines(raw string, n int) string {
	lines := strings.Split(raw, "\r\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\r\n")
}
