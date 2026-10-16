package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// DecideSetupAvailable 是安装向导唯一的门控判定，被两条路径复用：
//   - HTTP 层：POST /system/setup 与 /system/setup/mail/test 的准入；
//   - GET /system/status 的 setup_available 字段（前端据此决定是否强制跳 /setup）。
//
// 它是纯函数正是为了能这样测：不碰数据库、不碰配置对象，三分支全覆盖。
// 这里锁死的是「什么情况下允许走安装向导」，一旦被改错，后果是
// 要么首次部署根本进不去向导，要么已初始化的系统对外敞开建号入口。
func TestDecideSetupAvailable(t *testing.T) {
	cases := []struct {
		name         string
		enabled      bool
		adminCount   int
		wantAvail    bool
		wantContains string
	}{
		{
			name:         "配置关闭时一律不可用",
			enabled:      false,
			adminCount:   0,
			wantAvail:    false,
			wantContains: "auth.setup.enabled=false",
		},
		{
			name:       "未初始化且开关打开时可用",
			enabled:    true,
			adminCount: 0,
			wantAvail:  true,
			// 可用时的 Reason 只用于日志，不返回给客户端
			wantContains: "尚未初始化",
		},
		{
			name:         "已有可用管理员时永久失效",
			enabled:      true,
			adminCount:   1,
			wantAvail:    false,
			wantContains: "已完成初始化",
		},
		{
			name:         "多个管理员同样失效",
			enabled:      true,
			adminCount:   7,
			wantAvail:    false,
			wantContains: "7 名可用管理员",
		},
		{
			// 关键分支：即使开关打开，只要已有管理员就必须拒绝。
			// 这条防的是「配置开着 + 系统已装好」时向导被重新打开。
			name:         "开关打开也拦不住已初始化",
			enabled:      true,
			adminCount:   2,
			wantAvail:    false,
			wantContains: "永久失效",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DecideSetupAvailable(tc.enabled, tc.adminCount)
			if got.Available != tc.wantAvail {
				t.Fatalf("Available = %v, 期望 %v", got.Available, tc.wantAvail)
			}
			if !strings.Contains(got.Reason, tc.wantContains) {
				t.Fatalf("Reason = %q, 期望包含 %q", got.Reason, tc.wantContains)
			}
			if strings.TrimSpace(got.Reason) == "" {
				t.Fatal("Reason 不能为空：不可用时它会原样作为 409 的提示文案返回给前端")
			}
		})
	}
}

// TestErrSetupClosedIsConflictWithReason 确认不可用时的原因会被包装成 409，
// 前端靠它把「系统已初始化」与「真的没权限」区分开。
// 这里确认包装后既是冲突语义、又能读到原始原因。
func TestErrSetupClosedIsConflictWithReason(t *testing.T) {
	err := ErrSetupClosed("系统已完成初始化（已存在 1 名可用管理员）")
	if err == nil {
		t.Fatal("ErrSetupClosed 不应返回 nil")
	}
	if !strings.Contains(err.Message, "安装向导") {
		t.Fatalf("错误文案应说明是安装向导的问题，实际 %q", err.Message)
	}
	if !strings.Contains(err.Message, "已完成初始化") {
		t.Fatalf("错误文案应带上原始原因，实际 %q", err.Message)
	}
	if err.HTTPStatus != http.StatusConflict {
		t.Fatalf("HTTPStatus = %d，期望 409（系统状态冲突，不是权限问题）", err.HTTPStatus)
	}
	if err.Code != apperr.CodeConflict {
		t.Fatalf("Code = %d，期望 %d", err.Code, apperr.CodeConflict)
	}
}

// TestSetupAvailableMatchesSystemStatusExpression 锁定 /system/status 的
// setup_available 字段的确切计算口径。
//
// 这是前端「启动即强制跳 /setup」的唯一依据，因此把 4 个组合全部试一遍：
//
//	setup_available = auth.setup.enabled && !initialized
//	                = DecideSetupAvailable(enabled, adminCount).Available
//
// 其中 initialized 与 adminCount > 0 同源（都来自 availableAdminIDs）。
// 配置关闭 + 无管理员这一格尤其关键：它必须是 false，否则会把一个
// 「明确不想开放向导」的部署强行推进安装流程，而接口那边只会回 409。
func TestSetupAvailableMatchesSystemStatusExpression(t *testing.T) {
	cases := []struct {
		name        string
		setupEnable bool
		adminCount  int
		want        bool
	}{
		{"开关打开 + 未初始化 → 强制进向导", true, 0, true},
		{"开关打开 + 已初始化 → 不进向导", true, 3, false},
		{"开关关闭 + 未初始化 → 不进向导（避免推进一个必然 409 的流程）", false, 0, false},
		{"开关关闭 + 已初始化 → 不进向导", false, 3, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 与 systemMailStatus/SystemStatus 中写入 out.SetupAvailable 的表达式完全一致。
			got := DecideSetupAvailable(tc.setupEnable, tc.adminCount).Available
			if got != tc.want {
				t.Fatalf("setup_available = %v，期望 %v", got, tc.want)
			}

			// initialized 与 admin_count > 0 必须同源，否则前端会同时看到
			// 「已初始化」与「setup_available=true」这种自相矛盾的状态。
			initialized := tc.adminCount > 0
			if got && initialized {
				t.Fatal("setup_available 为 true 时 initialized 必须为 false")
			}
		})
	}
}
