package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"

	"github.com/jsong/jobs-robot/internal/ent/schema/mixins"
	"github.com/jsong/jobs-robot/internal/pkg/ulidid"
)

// AuditLog 记录所有会改变系统状态的管理操作，满足可追责要求。
type AuditLog struct {
	ent.Schema
}

// Mixin 提供 ULID 主键与 created_at / updated_at。
func (AuditLog) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.ULID{}, mixin.Time{}}
}

// Fields 定义审计字段。
func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("actor_id").
			MaxLen(ulidid.Length).
			Optional().
			Nillable().
			Comment("操作人，系统自动任务为空"),
		field.String("actor_email").
			MaxLen(255).
			Default("").
			Comment("操作人邮箱冗余，避免用户删除后无法追溯"),
		field.String("action").
			MaxLen(64).
			NotEmpty().
			Comment("动作标识，如 job.approve / company.create / user.assign_role"),
		field.String("resource").
			MaxLen(64).
			Default(""),
		field.String("resource_id").
			MaxLen(64).
			Default(""),
		field.String("method").
			MaxLen(10).
			Default(""),
		field.String("path").
			MaxLen(512).
			Default(""),
		field.Int("status_code").
			Default(0),
		field.Bool("success").
			Default(true),
		field.String("ip").
			MaxLen(64).
			Default(""),
		field.String("user_agent").
			MaxLen(512).
			Default(""),
		field.JSON("detail", map[string]any{}).
			Optional().
			Comment("变更明细，如 旧值/新值"),
	}
}

// Edges 定义审计关联。
func (AuditLog) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("actor", User.Type).
			Ref("audit_logs").
			Field("actor_id").
			Unique(),
	}
}

// Indexes 定义审计索引。
func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("actor_id"),
		index.Fields("action"),
	}
}
