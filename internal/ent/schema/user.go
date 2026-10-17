package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"

	"github.com/jsong/jobs-robot/internal/ent/schema/mixins"
)

// User 是系统用户（注册求职者、审核员、管理员共用一张表，权限差异由 Casbin 角色决定）。
type User struct {
	ent.Schema
}

// Mixin 提供 ULID 主键与 created_at / updated_at。
func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.ULID{}, mixin.Time{}}
}

// Fields 定义用户字段。
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			MaxLen(255).
			Default("").
			Comment("登录邮箱，统一小写存储；系统身份为空。非空值仍要求全局唯一（见部分唯一索引）"),
		field.String("password_hash").
			MaxLen(255).
			Default("").
			Sensitive().
			Comment("bcrypt 哈希，永不返回给客户端；系统身份无密码"),
		field.Enum("kind").
			Values("human", "system").
			Default("human").
			Comment("身份类型：human=真人账号 system=系统内置机器身份"),
		field.String("system_key").
			MaxLen(32).
			Default("").
			Comment("系统身份的稳定标识（crawler/mail），普通用户为空"),
		field.String("nickname").
			MaxLen(64).
			Default("").
			Comment("昵称/姓名"),
		field.String("avatar_url").
			MaxLen(512).
			Default(""),
		field.String("phone").
			MaxLen(32).
			Optional().
			Nillable(),
		field.Enum("status").
			Values("pending", "active", "disabled", "locked").
			Default("active").
			Comment("pending=待邮箱验证 active=正常 disabled=禁用 locked=连续登录失败锁定"),
		field.Bool("email_verified").
			Default(false),
		field.Time("email_verified_at").
			Optional().
			Nillable(),
		field.Time("last_login_at").
			Optional().
			Nillable(),
		field.String("last_login_ip").
			MaxLen(64).
			Default(""),
		field.String("register_ip").
			MaxLen(64).
			Default(""),
		field.String("register_source").
			MaxLen(32).
			Default("email"),
		field.Int("failed_login_count").
			Default(0).
			Comment("连续登录失败次数，成功登录后清零"),
		field.Time("locked_until").
			Optional().
			Nillable(),
		field.Time("password_changed_at").
			Optional().
			Nillable().
			Comment("改密后用于使旧令牌失效"),
		field.Bool("must_change_password").
			Default(false).
			Comment("是否需要强制修改密码：首次启动自举的管理员为 true"),
		field.String("remark").
			MaxLen(512).
			Default(""),
	}
}

// Edges 定义用户关联。
//
// 注意：用户的「角色」不在此处建模——授权信息以 Casbin 为唯一事实来源
// （casbin_rule 表中的 g 规则），roles 表仅作为角色的展示元数据。
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("reviewed_jobs", JobPosting.Type).
			Comment("该用户审核过的岗位"),
		edge.To("audit_logs", AuditLog.Type).
			Comment("该用户的管理操作审计记录"),
		edge.To("verification_codes", VerificationCode.Type).
			Comment("该用户邮箱验证码发放记录"),
	}
}

// Indexes 定义用户索引。
//
// 说明：created_at 来自 mixin.Time，ent 不支持对其建索引；
// 后台用户列表按 status 过滤即可满足性能需求。
//
// email / system_key 使用 PostgreSQL 部分唯一索引：系统身份的 email 为空、
// 普通用户的 system_key 为空，全表唯一约束会让第二个空值插入失败，
// 因此唯一性只在「非空值」上成立（where email <> ”）。
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("email").
			Unique().
			Annotations(entsql.IndexWhere("email <> ''")),
		index.Fields("system_key").
			Unique().
			Annotations(entsql.IndexWhere("system_key <> ''")),
	}
}
