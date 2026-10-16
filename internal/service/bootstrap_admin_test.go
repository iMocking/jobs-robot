package service

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 本文件只测纯函数：不依赖 PostgreSQL / Redis，因此任何环境都必须通过。
// 数据库相关的自举验证（真建账号、提升既有账号）由外部在新库上做首次启动验证。

// TestDecideBootstrapAdmin 锁定「是否还需要创建初始管理员」的判定分支。
//
// 这是自举幂等性的地基：只要还有一名可用管理员就必须跳过，
// 否则每次启动都会多出一个管理员账号。
func TestDecideBootstrapAdmin(t *testing.T) {
	cases := []struct {
		name       string
		admins     []string
		wantCreate bool
	}{
		{"没有任何管理员", nil, true},
		{"候选列表为空切片", []string{}, true},
		{"只有一名可用管理员", []string{"01JD7Q9K2M3N4P5R6S7T8V9W0X"}, false},
		{"多名可用管理员", []string{"01JD7Q9K2M3N4P5R6S7T8V9W0X", "01JD7Q9K2M3N4P5R6S7T8V9W0Y"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DecideBootstrapAdmin(tc.admins)
			if got.NeedBootstrap != tc.wantCreate {
				t.Fatalf("NeedBootstrap 期望 %v，实际 %v（reason=%s）", tc.wantCreate, got.NeedBootstrap, got.Reason)
			}
			if got.Reason == "" {
				t.Error("Reason 必须非空，便于运维从日志判断为何跳过/创建")
			}
		})
	}
}

// TestSelectAvailableAdminsKeepsOrderAndFilters 验证过滤语义与顺序稳定性。
func TestSelectAvailableAdminsKeepsOrderAndFilters(t *testing.T) {
	candidates := []string{"a", "b", "c", "d"}
	// b 是审核员（不是直接管理员），d 已被禁用。
	available := map[string]bool{"a": true, "c": true}

	got, err := selectAvailableAdmins(candidates, func(id string) (bool, error) {
		return available[id], nil
	})
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	want := []string{"a", "c"}
	if len(got) != len(want) {
		t.Fatalf("期望 %v，实际 %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("顺序必须保持原样：期望 %v，实际 %v", want, got)
		}
	}
}

// TestSelectAvailableAdminsAllUnavailable 全部不可用时必须返回空列表，
// 自举逻辑据此判定「需要创建」。
func TestSelectAvailableAdminsAllUnavailable(t *testing.T) {
	got, err := selectAvailableAdmins([]string{"a", "b"}, func(string) (bool, error) {
		return false, nil
	})
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("全部不可用时应返回空列表，实际 %v", got)
	}
	if decision := DecideBootstrapAdmin(got); !decision.NeedBootstrap {
		t.Fatal("没有可用管理员时必须决定创建")
	}
}

// TestSelectAvailableAdminsPropagatesError 关键安全断言：
// 查询出错必须向上传递，绝不能被当成「该用户不可用」。
//
// 若把数据库故障吞成「没有管理员」，系统会在每次启动时反复自举，
// 不断创建多余的管理员账号——这是比启动失败更糟的后果。
func TestSelectAvailableAdminsPropagatesError(t *testing.T) {
	sentinel := errors.New("数据库连接被拒绝")

	got, err := selectAvailableAdmins([]string{"a", "b"}, func(id string) (bool, error) {
		if id == "b" {
			return false, sentinel
		}
		return true, nil
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("应原样返回回调错误，实际: %v", err)
	}
	if got != nil {
		t.Fatalf("出错时不应返回结果，实际 %v", got)
	}
}

// TestWriteBootstrapCredentialFileCreatesDirsAndContent 验证凭据文件的落盘行为：
// 自动创建缺失目录、内容含邮箱与口令、并给出删除指引（否则运维不会记得清理明文口令）。
func TestWriteBootstrapCredentialFileCreatesDirsAndContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "bootstrap-admin.txt")
	at := time.Date(2025, 1, 8, 10, 12, 31, 0, time.Local)

	if err := writeBootstrapCredentialFile(path, "admin@jobs-robot.local", "Str0ngRandomPw", at); err != nil {
		t.Fatalf("写入凭据文件失败: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取凭据文件失败: %v", err)
	}
	content := string(raw)
	for _, want := range []string{
		"admin@jobs-robot.local",
		"Str0ngRandomPw",
		"2025-01-08 10:12:31",
		"删除本文件",
		path,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("凭据文件应包含 %q，实际内容：\n%s", want, content)
		}
	}
}

// TestWriteBootstrapCredentialFilePermissions 断言凭据文件权限为 0600。
//
// 这是可直接登录后台的明文口令，同机其它用户不应读到。
// Windows 上 os.FileMode 的权限位不生效（ACL 才是事实来源），因此跳过该断言。
func TestWriteBootstrapCredentialFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 不使用 Unix 权限位，无法断言 0600")
	}

	path := filepath.Join(t.TempDir(), "bootstrap-admin.txt")
	if err := writeBootstrapCredentialFile(path, "a@b.com", "Pw12345678", time.Now()); err != nil {
		t.Fatalf("写入凭据文件失败: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取文件信息失败: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("凭据文件权限应为 0600，实际 %o", perm)
	}
}

// TestWriteBootstrapCredentialFileOverwrites 重复自举（或人工删改后重跑）时
// 必须覆盖旧内容，不能追加出多份口令。
func TestWriteBootstrapCredentialFileOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap-admin.txt")
	if err := writeBootstrapCredentialFile(path, "old@example.com", "OldPassword1", time.Now()); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	if err := writeBootstrapCredentialFile(path, "new@example.com", "NewPassword2", time.Now()); err != nil {
		t.Fatalf("二次写入失败: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取凭据文件失败: %v", err)
	}
	if strings.Contains(string(raw), "OldPassword1") {
		t.Errorf("覆盖写入后不应残留旧口令：\n%s", raw)
	}
}
