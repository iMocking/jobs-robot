package casbin

import (
	"context"
	"testing"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
)

// 系统角色是「身份标记」而不是权限角色，这一组断言锁死它的两条边界：
// 不继承任何角色（否则 admin 的兜底策略会顺着继承链落下来）、
// 不拥有任何路径策略（否则机器人可以通过 HTTP 访问后台）。
func TestSystemRoleIsIsolated(t *testing.T) {
	if !IsBuiltinRole(RoleSystem) {
		t.Fatal("system 必须是内置角色，否则 SetRoles 会拒绝它")
	}

	for _, rule := range defaultGroupingRules {
		if rule[0] == RoleSystem || rule[1] == RoleSystem {
			t.Errorf("system 不应出现在角色继承中: %v", rule)
		}
	}
	for _, rule := range defaultPolicies {
		if len(rule) > 0 && rule[0] == RoleSystem {
			t.Errorf("system 不应拥有任何路径策略: %v", rule)
		}
	}

	// admin 有 /api/v1/* 兜底；确认它没有通过继承链与 system 建立关系。
	for _, rule := range defaultGroupingRules {
		if rule[0] == RoleAdmin && rule[1] == RoleSystem {
			t.Fatal("admin 不应继承 system（否则 system 反而会获得权限）")
		}
	}
}

// TestSeedDefaultsAcceptsRoleWithoutPolicies 验证「角色没有任何策略」不会让播种失败。
//
// system 就是这种角色。SeedDefaults 若对空策略做了非空校验，服务启动会在
// 播种阶段直接报错——而不是等到第一次抓取才发现。
func TestSeedDefaultsAcceptsRoleWithoutPolicies(t *testing.T) {
	m, err := casbinmodel.NewModelFromString(modelText)
	if err != nil {
		t.Fatalf("解析模型失败: %v", err)
	}
	enforcer, err := casbin.NewEnforcer(m)
	if err != nil {
		t.Fatalf("构造内存 enforcer 失败: %v", err)
	}

	authz := &Authorizer{enforcer: enforcer}
	if err := authz.SeedDefaults(context.Background()); err != nil {
		t.Fatalf("SeedDefaults 不应因为 system 没有策略而失败: %v", err)
	}

	// 播种完成后，即使把某个用户显式授予 system，它仍然不能访问任何后台路径。
	const systemUserID = "01TESTSYSTEM00000000000000"
	if err := authz.SetRoles(systemUserID, []string{RoleSystem}); err != nil {
		t.Fatalf("授予 system 角色失败: %v", err)
	}
	allowed, err := authz.EnforceUser(systemUserID, "/api/v1/admin/stats", "GET")
	if err != nil {
		t.Fatalf("权限判定失败: %v", err)
	}
	if allowed {
		t.Fatal("播种后 system 仍不应获得任何 HTTP 权限")
	}
}
