// Package casbin 封装授权能力：模型、策略存储与角色管理。
//
// 授权事实来源（single source of truth）：
//   - casbin_rule 表中的 p 规则 = 「谁能在哪个接口上做什么」
//   - casbin_rule 表中的 g 规则 = 「哪个用户属于哪个角色」
//
// 数据库中的 roles 表只是角色的展示元数据（中文名、描述），不参与授权判定，
// 从而避免出现两套互相打架的权限数据。
package casbin

// RoleAdmin / RoleReviewer / RoleUser 是系统预设的三种角色。
const (
	RoleAdmin    = "admin"
	RoleReviewer = "reviewer"
	RoleUser     = "user"
)

// BuiltinRoles 是系统内置角色（顺序即权限从高到低）。
var BuiltinRoles = []string{RoleAdmin, RoleReviewer, RoleUser}

// IsBuiltinRole 判断是否内置角色。
func IsBuiltinRole(name string) bool {
	for _, r := range BuiltinRoles {
		if r == name {
			return true
		}
	}
	return false
}

// modelText 是 Casbin 模型定义。
//
// 匹配规则说明：
//   - g(r.sub, p.sub)  实现角色继承（admin ⊃ reviewer ⊃ user）
//   - keyMatch2        支持 /api/v1/jobs/:id 这类路径参数匹配
//   - p.act == "*"     允许策略里用通配动作
//
// 请求三元组为 (sub, obj, act) = (user:<id>, 请求路径, HTTP 方法)。
const modelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && (r.act == p.act || p.act == "*")
`

// defaultGroupingRules 预设的角色继承关系。
//
//	reviewer 继承 user 的全部权限，admin 继承 reviewer 的全部权限。
var defaultGroupingRules = [][]string{
	{"reviewer", RoleUser},
	{RoleAdmin, RoleReviewer},
}

// defaultPolicies 预设的权限策略。
//
// 约定：obj 使用接口路径，act 使用 HTTP 方法；管理端接口统一挂在 /api/v1/admin 下。
var defaultPolicies = [][]string{
	// —— 普通用户：浏览已发布岗位、维护自己的资料 ——
	{RoleUser, "/api/v1/jobs", "GET"},
	{RoleUser, "/api/v1/jobs/:id", "GET"},
	{RoleUser, "/api/v1/companies", "GET"},
	{RoleUser, "/api/v1/companies/:id", "GET"},
	{RoleUser, "/api/v1/meta/*", "GET"},
	{RoleUser, "/api/v1/me", "GET"},
	{RoleUser, "/api/v1/me/*", "*"},

	// —— 审核员：查看待审岗位并作出审核结论 ——
	{RoleReviewer, "/api/v1/admin/jobs", "GET"},
	{RoleReviewer, "/api/v1/admin/jobs/:id", "GET"},
	{RoleReviewer, "/api/v1/admin/jobs/:id/review", "POST"},
	{RoleReviewer, "/api/v1/admin/jobs/:id/revisions", "GET"},
	{RoleReviewer, "/api/v1/admin/companies", "GET"},
	{RoleReviewer, "/api/v1/admin/crawl-runs", "GET"},
	{RoleReviewer, "/api/v1/admin/crawl-runs/:id", "GET"},
	{RoleReviewer, "/api/v1/admin/stats", "GET"},

	// —— 管理员：拥有全部管理权限 ——
	{RoleAdmin, "/api/v1/*", "*"},
}
