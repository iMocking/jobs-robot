package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"

	"github.com/jsong/jobs-robot/internal/ent/schema/mixins"
)

// Role 是角色的展示元数据。
//
// 设计说明：本表只承载角色的名称、描述、内置标记等信息，用于后台角色列表展示。
// 真正的「谁能做什么」（p 规则）与「谁是什么角色」（g 规则）全部存放在 Casbin 的
// casbin_rule 表中，由 internal/casbin 包统一读写，避免出现两套授权事实来源。
type Role struct {
	ent.Schema
}

// Mixin 提供 ULID 主键与 created_at / updated_at。
func (Role) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.ULID{}, mixin.Time{}}
}

// Fields 定义角色字段。
func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(64).
			NotEmpty().
			Unique().
			Comment("角色标识，与 Casbin 中的角色名一致：admin/reviewer/user"),
		field.String("display_name").
			MaxLen(64).
			NotEmpty().
			Comment("中文展示名，如「管理员」"),
		field.String("description").
			MaxLen(512).
			Default(""),
		field.Bool("is_system").
			Default(false).
			Comment("系统内置角色，不允许删除"),
		field.Int("level").
			Default(0).
			Comment("权限层级，数值越大权限越高，仅用于展示排序"),
	}
}

// Indexes 定义角色索引。
func (Role) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("level"),
	}
}
