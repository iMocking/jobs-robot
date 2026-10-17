package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/casbin"
	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/auditlog"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// 本文件是依赖真实 PostgreSQL/Redis 的集成测试（与 user_test.go 同一套基建）。
//
// 系统身份是「与真人同表、无邮箱无密码、角色 system」的特殊账号，
// 这里锁定的是它最容易退化的几条性质：幂等、不可登录、不可被后台修改、不计入用户数。
// 测试创建的行会在 t.Cleanup 中删除（仅当本次测试确实是它创建的时候）。

// ensureSystemIdentitiesForTest 调用被测方法，并注册在测试结束后清理。
//
// 若库里本来就存在系统身份（例如真实启动过 API），则保留不动：
// 测试只应回收自己造出来的数据，不能删掉别人的运行态。
func ensureSystemIdentitiesForTest(t *testing.T, svc *Services) map[string]string {
	t.Helper()
	ctx := context.Background()

	pre := map[string]string{}
	rows, err := svc.Store.Ent.User.Query().
		Where(entuser.KindEQ(entuser.KindSystem)).
		All(ctx)
	if err != nil {
		t.Fatalf("查询既有系统身份失败: %v", err)
	}
	for _, row := range rows {
		pre[row.SystemKey] = row.ID
	}

	ids, err := svc.EnsureSystemIdentities(ctx)
	if err != nil {
		t.Fatalf("EnsureSystemIdentities 失败: %v", err)
	}

	t.Cleanup(func() {
		for key, id := range ids {
			if _, existed := pre[key]; existed {
				continue
			}
			_ = svc.Authz.SetRoles(id, nil)
			// 审计是外键 SET NULL 的旁路表；测试自己产生的那几条一并回收。
			_, _ = svc.Store.Ent.AuditLog.Delete().
				Where(auditlog.ActionEQ("system.identity_ensure"), auditlog.ResourceIDEQ(id)).
				Exec(ctx)
			_ = svc.Store.Ent.User.DeleteOneID(id).Exec(ctx)
		}
	})
	return ids
}

func TestEnsureSystemIdentitiesIsIdempotent(t *testing.T) {
	svc := testServices(t)
	ctx := context.Background()

	ids1 := ensureSystemIdentitiesForTest(t, svc)
	totalAfterFirst, err := svc.Store.Ent.User.Query().Count(ctx)
	if err != nil {
		t.Fatalf("统计用户数失败: %v", err)
	}

	ids2, err := svc.EnsureSystemIdentities(ctx)
	if err != nil {
		t.Fatalf("第二次 EnsureSystemIdentities 失败: %v", err)
	}
	totalAfterSecond, err := svc.Store.Ent.User.Query().Count(ctx)
	if err != nil {
		t.Fatalf("统计用户数失败: %v", err)
	}

	if totalAfterFirst != totalAfterSecond {
		t.Fatalf("幂等性被破坏：第二次调用后用户总数从 %d 变成 %d", totalAfterFirst, totalAfterSecond)
	}
	if len(ids1) != len(SystemIdentities) || len(ids2) != len(SystemIdentities) {
		t.Fatalf("系统身份数量不符：期望 %d，第一次 %v，第二次 %v", len(SystemIdentities), ids1, ids2)
	}
	for _, identity := range SystemIdentities {
		if ids1[identity.Key] == "" || ids1[identity.Key] != ids2[identity.Key] {
			t.Fatalf("%s 的 ULID 不稳定：第一次 %q，第二次 %q", identity.Key, ids1[identity.Key], ids2[identity.Key])
		}

		user, err := svc.Store.Ent.User.Get(ctx, ids1[identity.Key])
		if err != nil {
			t.Fatalf("读取系统身份 %s 失败: %v", identity.Key, err)
		}
		if user.Kind != entuser.KindSystem {
			t.Errorf("%s kind = %s，期望 system", identity.Key, user.Kind)
		}
		if user.SystemKey != identity.Key {
			t.Errorf("%s system_key = %q，期望 %q", identity.Key, user.SystemKey, identity.Key)
		}
		if user.Nickname != identity.Name {
			t.Errorf("%s nickname = %q，期望 %q", identity.Key, user.Nickname, identity.Name)
		}
		if user.Email != "" || user.PasswordHash != "" {
			t.Errorf("%s 不应有邮箱或密码哈希：email=%q password_hash=%q", identity.Key, user.Email, user.PasswordHash)
		}
		if user.Status != entuser.StatusActive || !user.EmailVerified {
			t.Errorf("%s 状态应为 active + email_verified：status=%s verified=%v", identity.Key, user.Status, user.EmailVerified)
		}
		if user.RegisterSource != "system" {
			t.Errorf("%s register_source = %q，期望 system", identity.Key, user.RegisterSource)
		}

		roles, err := svc.Authz.RolesForUser(user.ID)
		if err != nil {
			t.Fatalf("读取 %s 角色失败: %v", identity.Key, err)
		}
		if len(roles) != 1 || roles[0] != casbin.RoleSystem {
			t.Errorf("%s 的角色应为 [system]，实际 %v", identity.Key, roles)
		}
	}
}

func TestSystemIdentitiesCannotLoginOrAccessHTTP(t *testing.T) {
	svc := testServices(t)
	ctx := context.Background()
	ids := ensureSystemIdentitiesForTest(t, svc)

	// 空邮箱直接走「凭据错误」，不泄露任何存在性信息。
	if _, err := svc.Auth.Login(ctx, LoginInput{Email: "", Password: "whatever"}); !apperr.Is(err, apperr.CodeInvalidCredentials) {
		t.Fatalf("空邮箱登录应返回凭据错误，实际: %v", err)
	}
	// 即使把系统身份的空邮箱原样送进来，也必须失败（登录入口的 looksLikeEmail 会先拦下）。
	mailID := ids[SystemKeyMail]
	user, err := svc.Store.Ent.User.Get(ctx, mailID)
	if err != nil {
		t.Fatalf("读取邮件机器人失败: %v", err)
	}
	if _, err := svc.Auth.Login(ctx, LoginInput{Email: user.Email, Password: "whatever"}); !apperr.Is(err, apperr.CodeInvalidCredentials) {
		t.Fatalf("系统身份不应能登录，实际: %v", err)
	}

	for _, identity := range SystemIdentities {
		id := ids[identity.Key]
		// system 不继承任何角色：admin 的 /api/v1/* 兜底策略不会落到它身上。
		for _, probe := range []struct{ path, method string }{
			{"/api/v1/admin/stats", "GET"},
			{"/api/v1/admin/users", "GET"},
			{"/api/v1/me", "GET"},
			{"/api/v1/admin/crawl/run-all", "POST"},
		} {
			allowed, err := svc.Authz.EnforceUser(id, probe.path, probe.method)
			if err != nil {
				t.Fatalf("权限判定失败: %v", err)
			}
			if allowed {
				t.Errorf("系统身份 %s 不应能访问 %s %s", identity.Key, probe.method, probe.path)
			}
		}
	}
}

func TestSystemIdentitiesCannotBeModifiedByAdmin(t *testing.T) {
	svc := testServices(t)
	ctx := context.Background()
	ids := ensureSystemIdentitiesForTest(t, svc)

	// 用无 UserID 的系统 actor：本测试只验证「目标是不是系统身份」，
	// 不涉及「不能改自己」那条独立的保护。
	actor := SystemActor()

	assertRejected := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s 应被拒绝", label)
		}
		if !apperr.Is(err, apperr.CodeInvalidParams) {
			t.Errorf("%s 应返回参数错误（10001），实际: %v", label, err)
		}
		if !strings.Contains(err.Error(), "系统身份不可修改") {
			t.Errorf("%s 的错误文案应说明系统身份不可修改，实际: %v", label, err)
		}
	}

	for _, identity := range SystemIdentities {
		id := ids[identity.Key]

		_, err := svc.User.SetStatus(ctx, id, string(entuser.StatusDisabled), actor)
		assertRejected("SetStatus("+identity.Key+")", err)

		err = svc.User.Delete(ctx, id, actor)
		assertRejected("Delete("+identity.Key+")", err)

		_, err = svc.User.SetRoles(ctx, id, []string{casbin.RoleUser}, actor)
		assertRejected("SetRoles("+identity.Key+")", err)

		err = svc.User.ResetPassword(ctx, id, "AnotherPassw0rd!", actor)
		assertRejected("ResetPassword("+identity.Key+")", err)

		// 失败不能产生副作用：身份仍然是 active、角色仍然是 system。
		user, err := svc.Store.Ent.User.Get(ctx, id)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", identity.Key, err)
		}
		if user.Status != entuser.StatusActive {
			t.Errorf("%s 被失败的 SetStatus 改成了 %s", identity.Key, user.Status)
		}
		roles, err := svc.Authz.RolesForUser(id)
		if err != nil {
			t.Fatalf("读取 %s 角色失败: %v", identity.Key, err)
		}
		if len(roles) != 1 || roles[0] != casbin.RoleSystem {
			t.Errorf("%s 的角色被失败的 SetRoles 改成了 %v", identity.Key, roles)
		}
	}
}

func TestUserCountsExcludeSystemIdentities(t *testing.T) {
	svc := testServices(t)
	ctx := context.Background()
	ensureSystemIdentitiesForTest(t, svc)

	humanCount, err := svc.Store.Ent.User.Query().
		Where(entuser.KindNEQ(entuser.KindSystem)).
		Count(ctx)
	if err != nil {
		t.Fatalf("统计真人用户失败: %v", err)
	}
	systemCount, err := svc.Store.Ent.User.Query().
		Where(entuser.KindEQ(entuser.KindSystem)).
		Count(ctx)
	if err != nil {
		t.Fatalf("统计系统身份失败: %v", err)
	}
	if systemCount < len(SystemIdentities) {
		t.Fatalf("前置条件不成立：系统身份只有 %d 个", systemCount)
	}

	status, err := svc.Meta.SystemStatus(ctx)
	if err != nil {
		t.Fatalf("SystemStatus 失败: %v", err)
	}
	if status.UserCount != humanCount {
		t.Errorf("SystemStatus.UserCount = %d，期望真人用户数 %d（系统身份 %d 不应计入）",
			status.UserCount, humanCount, systemCount)
	}

	stats, err := svc.Meta.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats 失败: %v", err)
	}
	if stats.Users.Total != humanCount {
		t.Errorf("Stats.Users.Total = %d，期望 %d", stats.Users.Total, humanCount)
	}
	if stats.Users.Active > humanCount {
		t.Errorf("Stats.Users.Active = %d 超过了真人用户总数 %d", stats.Users.Active, humanCount)
	}
}

func TestMailSendAuditMasksRecipient(t *testing.T) {
	svc := testServices(t)
	ctx := context.Background()
	ids := ensureSystemIdentitiesForTest(t, svc)

	// 让 mail.send 的 actor 指向真实的邮件机器人 ULID。
	RegisterSystemActor(SystemKeyMail, Actor{UserID: ids[SystemKeyMail]})
	t.Cleanup(func() { RegisterSystemActor(SystemKeyMail, Actor{}) })

	email := "masked-" + time.Now().Format("150405.000000000") + "@example.com"
	// 测试环境 Mailer 为 nil：发信必然失败，但 mail.send 审计仍应落库。
	if _, err := svc.Auth.SendCode(ctx, email, auth.CodePurposeRegister, "127.0.0.1", "integration-test"); err == nil {
		t.Fatal("测试环境没有可用 Mailer，SendCode 应当失败")
	}

	row, err := svc.Store.Ent.AuditLog.Query().
		Where(auditlog.ActionEQ("mail.send")).
		Order(ent.Desc(auditlog.FieldID)).
		First(ctx)
	if err != nil {
		t.Fatalf("未找到 mail.send 审计: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.Store.Ent.AuditLog.DeleteOneID(row.ID).Exec(ctx)
	})

	if row.ActorID == nil || *row.ActorID != ids[SystemKeyMail] {
		t.Errorf("mail.send 的 actor_id = %v，期望邮件机器人 %s", row.ActorID, ids[SystemKeyMail])
	}
	if row.ActorEmail != "" {
		t.Errorf("系统身份不应写 actor_email，实际 %q", row.ActorEmail)
	}

	detail := row.Detail
	if detail["purpose"] != mailPurposeVerifyRegister {
		t.Errorf("purpose = %v，期望 %s", detail["purpose"], mailPurposeVerifyRegister)
	}
	if detail["driver"] != "none" {
		t.Errorf("driver = %v，期望 none（未配置投递器）", detail["driver"])
	}
	if detail["to_masked"] != "m***@example.com" {
		t.Errorf("to_masked = %v，期望 m***@example.com", detail["to_masked"])
	}

	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("序列化 detail 失败: %v", err)
	}
	if strings.Contains(string(raw), email) {
		t.Errorf("mail.send 审计泄露了完整收件人：%s", raw)
	}
	t.Logf("mail.send detail 示例: %s", raw)
}
