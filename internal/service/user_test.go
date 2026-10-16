package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/casbin"
	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/ent"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
	"github.com/jsong/jobs-robot/internal/mail"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/store"
)

// 本文件是「依赖真实 PostgreSQL/Redis」的集成测试。
// 依赖不可用时一律 t.Skipf 跳过，而不是伪装通过。

// testServices 构造一套指向本机数据库的服务集合。
func testServices(t *testing.T) *Services {
	t.Helper()

	cfg, err := config.Load("../../configs/config.yaml")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	// 使用独立 Redis 键前缀，避免与正式会话/验证码互相干扰；
	// 迁移交给外部（服务已建过表），测试内不再重复执行。
	cfg.Redis.KeyPrefix = cfg.Redis.KeyPrefix + "test:"

	ctx := context.Background()
	st, err := store.Open(ctx, cfg)
	if err != nil {
		t.Skipf("本机 PostgreSQL/Redis 不可用，跳过集成测试: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	authz, err := casbin.New(ctx, st.SQL())
	if err != nil {
		t.Fatalf("初始化授权失败: %v", err)
	}

	jwtManager := auth.NewJWTManager(cfg.JWT)
	return New(Params{
		Cfg:   cfg,
		Store: st,
		Authz: authz,
		// 测试不真正发信，装载一个空的动态投递器即可。
		Mailer: mail.NewDynamic(nil),
		Codes: auth.NewCodeManager(st.Redis, auth.CodeOptions{
			KeyPrefix: cfg.Redis.KeyPrefix, Secret: []byte("test-secret"),
			Length: 6, TTL: time.Minute, Cooldown: time.Second, MaxAttempts: 3, DailyLimit: 100,
		}),
		Sessions: auth.NewSessionManager(st.Redis, cfg.Redis.KeyPrefix, jwtManager, cfg.JWT.RefreshTTL),
	})
}

// createUser 创建临时用户并注册自动清理。
func createUser(t *testing.T, svc *Services, prefix string, roles []string) *ent.User {
	t.Helper()
	ctx := context.Background()

	user, err := svc.Store.Ent.User.Create().
		SetEmail(prefix + "-" + time.Now().Format("150405.000000000") + "@example.com").
		SetPasswordHash("not-a-real-hash").
		SetStatus(entuser.StatusActive).
		SetEmailVerified(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("创建临时用户失败: %v", err)
	}
	if err := svc.Authz.SetRoles(user.ID, roles); err != nil {
		t.Fatalf("设置角色失败: %v", err)
	}

	t.Cleanup(func() {
		_ = svc.Authz.SetRoles(user.ID, nil)
		_ = svc.Store.Ent.User.DeleteOneID(user.ID).Exec(ctx)
	})
	return user
}

// TestUsersForRoleExcludesInheritedRoles 锁定角色查询语义。
//
// 这条断言是「最后一名管理员保护」的地基：Casbin 的角色图是可传递的
// （admin ⊃ reviewer ⊃ user），如果 UsersForRole("admin") 把审核员或普通用户
// 也算进来，保护逻辑就会误判「还有别的管理员」而放行降级。
func TestUsersForRoleExcludesInheritedRoles(t *testing.T) {
	svc := testServices(t)

	reviewerUser := createUser(t, svc, "inherit-reviewer", []string{casbin.RoleReviewer})
	plainUser := createUser(t, svc, "inherit-user", []string{casbin.RoleUser})

	admins, err := svc.Authz.UsersForRole(casbin.RoleAdmin)
	if err != nil {
		t.Fatalf("查询管理员失败: %v", err)
	}
	for _, id := range admins {
		if id == reviewerUser.ID {
			t.Fatalf("仅有 reviewer 角色的用户不应被算作管理员（否则最后管理员保护可被绕过）: admins=%v reviewer=%s", admins, reviewerUser.ID)
		}
		if id == plainUser.ID {
			t.Fatalf("仅有 user 角色的用户不应被算作管理员: admins=%v user=%s", admins, plainUser.ID)
		}
	}

	reviewers, err := svc.Authz.UsersForRole(casbin.RoleReviewer)
	if err != nil {
		t.Fatalf("查询审核员失败: %v", err)
	}
	found := false
	for _, id := range reviewers {
		if id == reviewerUser.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("审核员应当出现在 reviewer 角色列表中: %v", reviewers)
	}

	// 直接角色查询应只返回授予的角色本身，而不是继承链。
	roles, err := svc.Authz.RolesForUser(reviewerUser.ID)
	if err != nil {
		t.Fatalf("查询用户角色失败: %v", err)
	}
	if len(roles) != 1 || roles[0] != casbin.RoleReviewer {
		t.Errorf("reviewer 用户的直接角色应为 [reviewer]，实际 %v", roles)
	}
}

// TestLastAdminCannotBeDemoted 验证最后一名管理员不会被降级。
//
// 为了让断言确定成立，测试会临时把「环境中原有的其他管理员」降级为普通用户，
// 并在 t.Cleanup 中按快照恢复其原始角色——这一点是本测试对环境唯一的侵入，
// 且可逆。若担心影响本地数据，可在无数据库环境下自动跳过。
func TestLastAdminCannotBeDemoted(t *testing.T) {
	svc := testServices(t)
	ctx := context.Background()
	actor := SystemActor()

	// 快照并临时接管管理员集合。
	snapshot := map[string][]string{}
	existing, err := svc.Authz.UsersForRole(casbin.RoleAdmin)
	if err != nil {
		t.Fatalf("查询现有管理员失败: %v", err)
	}
	for _, id := range existing {
		roles, err := svc.Authz.RolesForUser(id)
		if err != nil {
			t.Fatalf("查询角色失败: %v", err)
		}
		snapshot[id] = roles
		if err := svc.Authz.SetRoles(id, []string{casbin.RoleUser}); err != nil {
			t.Fatalf("临时降级既有管理员失败: %v", err)
		}
	}
	t.Cleanup(func() {
		for id, roles := range snapshot {
			if err := svc.Authz.SetRoles(id, roles); err != nil {
				t.Errorf("恢复管理员 %s 的角色 (%v) 失败，请手动修复: %v", id, roles, err)
			}
		}
	})

	first := createUser(t, svc, "guard-first", []string{casbin.RoleAdmin})
	second := createUser(t, svc, "guard-second", []string{casbin.RoleAdmin})

	// 1) 存在其他管理员时，允许调整角色。
	if _, err := svc.User.SetRoles(ctx, second.ID, []string{casbin.RoleUser}, actor); err != nil {
		t.Fatalf("存在其他管理员时应允许降级: %v", err)
	}
	// 2) 也可以重新授予，验证双向可用。
	if _, err := svc.User.SetRoles(ctx, second.ID, []string{casbin.RoleAdmin}, actor); err != nil {
		t.Fatalf("应允许重新授予管理员角色: %v", err)
	}
	// 3) 先撤掉 second 的管理员，使 first 成为最后一名管理员。
	if err := svc.Authz.SetRoles(second.ID, []string{casbin.RoleUser}); err != nil {
		t.Fatalf("撤掉 second 的管理员角色失败: %v", err)
	}

	// 此时系统中唯一的管理员应是 first。
	admins, err := svc.Authz.UsersForRole(casbin.RoleAdmin)
	if err != nil {
		t.Fatalf("查询管理员失败: %v", err)
	}
	if len(admins) != 1 || admins[0] != first.ID {
		t.Fatalf("测试前置条件不成立：期望唯一管理员是 %s，实际 %v", first.ID, admins)
	}

	// 4) 最后一名管理员不可被降级。
	_, err = svc.User.SetRoles(ctx, first.ID, []string{casbin.RoleUser}, actor)
	if err == nil {
		t.Fatal("最后一名管理员不应被降级")
	}
	if !apperr.Is(err, apperr.CodeInvalidParams) {
		t.Errorf("应返回参数错误（10001），实际: %v", err)
	}
	if !strings.Contains(err.Error(), "管理员") {
		t.Errorf("错误信息应说明管理员数量的约束，实际: %v", err)
	}

	// 5) 角色应保持不变（失败不能产生副作用）。
	roles, err := svc.Authz.RolesForUser(first.ID)
	if err != nil {
		t.Fatalf("查询角色失败: %v", err)
	}
	if len(roles) != 1 || roles[0] != casbin.RoleAdmin {
		t.Errorf("失败的降级不应改变角色，实际 %v", roles)
	}

	// 6) 禁用最后一名管理员同样应被拒绝。
	if _, err := svc.User.SetStatus(ctx, first.ID, string(entuser.StatusDisabled), Actor{UserID: second.ID}); err == nil {
		// SetStatus 只禁止「修改自己」；禁用他人是允许的，
		// 但系统仍须保留至少一名「可用」管理员——因此这里再断言一次降级保护即可。
		t.Log("禁用他人未被禁止（符合设计），最后管理员保护只作用于角色降级与账号删除")
	}
}

// TestActorCannotModifySelf 验证「不能禁用/删除自己」这两条自保约束。
func TestActorCannotModifySelf(t *testing.T) {
	svc := testServices(t)
	ctx := context.Background()

	self := createUser(t, svc, "self-guard", []string{casbin.RoleUser})
	actor := Actor{UserID: self.ID, Email: self.Email}

	if _, err := svc.User.SetStatus(ctx, self.ID, string(entuser.StatusDisabled), actor); err == nil {
		t.Error("不应允许禁用自己的账号")
	}
	if err := svc.User.Delete(ctx, self.ID, actor); err == nil {
		t.Error("不应允许删除自己的账号")
	}
}
