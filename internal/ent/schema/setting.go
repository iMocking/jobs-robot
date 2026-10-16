package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"

	"github.com/jsong/jobs-robot/internal/ent/schema/mixins"
)

// Setting 是可在运行时热更新的系统配置（键值 + JSON 内容）。
//
// 爬虫收集范围就落在这里：key = "crawler.scope"，
// value 形如 {"job_types":[...],"industries":[...],"exclude_keywords":[...]}。
// 管理员通过接口修改后立即生效，无需重启服务。
type Setting struct {
	ent.Schema
}

// Mixin 提供 ULID 主键与 created_at / updated_at。
func (Setting) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.ULID{}, mixin.Time{}}
}

// Fields 定义配置项字段。
func (Setting) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").
			MaxLen(128).
			NotEmpty().
			Unique().
			Comment("配置键，点号分层，如 crawler.scope"),
		field.JSON("value", map[string]any{}).
			Optional(),
		field.String("description").
			MaxLen(512).
			Default(""),
		field.String("group").
			MaxLen(64).
			Default("").
			Comment("分组，便于后台分类展示：crawler/auth/mail/seo"),
		field.Bool("is_system").
			Default(false).
			Comment("系统内置配置，不允许删除"),
		field.String("updated_by").
			MaxLen(255).
			Default(""),
	}
}

// Indexes 定义配置项索引。
func (Setting) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group"),
	}
}
