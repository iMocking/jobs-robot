package config

import (
	"strings"
	"testing"
	"time"
)

// 本文件锁定「首次安装」相关的默认值与互斥约束。
//
// 为什么值得单测：这几个默认值直接决定全新部署的行为——
//   - auth.bootstrap.enabled 默认 true 会让启动自举抢在安装向导之前建号，
//     系统立刻变成「已初始化」，前端 /setup 就永远用不上了；
//   - auth.setup.enabled 默认 false 则相反：向导页永远进不去（409）。
//
// 两者都只能有一个默认开启，且不能同时开启，否则必然有一个功能形同虚设。

// TestDefaultDisablesBootstrapAndEnablesSetup 锁定两个开关的默认值。
func TestDefaultDisablesBootstrapAndEnablesSetup(t *testing.T) {
	cfg := Default()

	if cfg.Auth.Bootstrap.Enabled {
		t.Error("auth.bootstrap.enabled 默认必须为 false：默认路径是安装向导，" +
			"启动自举会先建号导致向导永久失效")
	}
	if !cfg.Auth.Setup.Enabled {
		t.Error("auth.setup.enabled 默认必须为 true：全新部署第一次打开前端就该能进向导")
	}
	if cfg.Auth.RateLimit.SetupLimit != 10 {
		t.Errorf("auth.rate_limit.setup_limit 默认应为 10，实际 %d", cfg.Auth.RateLimit.SetupLimit)
	}
	if cfg.Auth.RateLimit.SetupWindow != time.Hour {
		t.Errorf("auth.rate_limit.setup_window 默认应为 1h，实际 %s", cfg.Auth.RateLimit.SetupWindow)
	}
}

// TestValidateRejectsBootstrapAndSetupBothEnabled 确认互斥约束在启动时就被拦下。
//
// 若只靠运行期表现，运维会看到「向导页一打开就 409」，而真正的原因
// （启动自举已经把管理员建好了）藏在启动日志里，很难联想到配置冲突。
func TestValidateRejectsBootstrapAndSetupBothEnabled(t *testing.T) {
	cfg := Default()
	cfg.JWT.Secret = strings.Repeat("a", 40)
	cfg.Auth.Bootstrap.Enabled = true
	cfg.Auth.Setup.Enabled = true

	err := cfg.Validate()
	if err == nil {
		t.Fatal("setup 与 bootstrap 同时启用时应报配置错误")
	}
	if !strings.Contains(err.Error(), "auth.setup.enabled") {
		t.Errorf("错误信息应点名 auth.setup.enabled，实际: %v", err)
	}
}

// TestValidateAcceptsExclusiveSwitches 确认两种合理的部署形态都能通过校验：
// 有界面（向导开、自举关）与无界面（向导关、自举开）。
func TestValidateAcceptsExclusiveSwitches(t *testing.T) {
	cases := []struct {
		name      string
		setup     bool
		bootstrap bool
	}{
		{"有界面部署：向导开、自举关", true, false},
		{"无界面部署：向导关、自举开", false, true},
		{"两者都关：账号由外部系统预置", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.JWT.Secret = strings.Repeat("a", 40)
			cfg.Auth.Setup.Enabled = tc.setup
			cfg.Auth.Bootstrap.Enabled = tc.bootstrap

			if err := cfg.Validate(); err != nil {
				t.Fatalf("该组合应当通过校验，实际: %v", err)
			}
		})
	}
}

// TestValidateRejectsMalformedSetupTestMailTo 确认可选的默认收件人一旦填写就必须合法。
func TestValidateRejectsMalformedSetupTestMailTo(t *testing.T) {
	cfg := Default()
	cfg.JWT.Secret = strings.Repeat("a", 40)

	cfg.Auth.Setup.TestMailTo = "not-an-email"
	if err := cfg.Validate(); err == nil {
		t.Fatal("auth.setup.test_mail_to 非邮箱格式时应报配置错误")
	}

	// 留空是合法值：表示由请求体指定收件人。
	cfg.Auth.Setup.TestMailTo = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("留空应当合法，实际: %v", err)
	}
}
