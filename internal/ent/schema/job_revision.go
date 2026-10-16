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

// JobRevision 记录岗位内容的每一次变更。
//
// 有了它，「最后更新时间」不再是一个孤立的字段：可以回答
// 「这个岗位在什么时间、由哪次抓取、改动了哪些字段」。
type JobRevision struct {
	ent.Schema
}

// Mixin 提供 ULID 主键与 created_at / updated_at。
func (JobRevision) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.ULID{}, mixin.Time{}}
}

// Fields 定义变更记录字段。
func (JobRevision) Fields() []ent.Field {
	return []ent.Field{
		field.String("job_id").
			MaxLen(ulidid.Length).
			NotEmpty(),
		field.Int("revision").
			Comment("第几次修订"),
		field.String("content_hash").
			MaxLen(64).
			Default("").
			Comment("本次修订后的内容指纹"),
		field.Strings("changed_fields").
			Optional().
			Comment("发生变化的字段名列表"),
		field.JSON("snapshot", map[string]any{}).
			Optional().
			Comment("本次修订后的关键字段快照"),
		field.String("source_url").
			MaxLen(1024).
			Default(""),
		field.String("trigger").
			MaxLen(32).
			Default("crawl").
			Comment("变更来源：crawl=抓取 manual=人工编辑"),
		field.String("operator_id").
			MaxLen(ulidid.Length).
			Optional().
			Nillable().
			Comment("人工编辑时的操作人"),
	}
}

// Edges 定义变更记录关联。
func (JobRevision) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("job", JobPosting.Type).
			Ref("revisions").
			Field("job_id").
			Unique().
			Required(),
	}
}

// Indexes 定义变更记录索引。
func (JobRevision) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("job_id", "revision").Unique(),
	}
}
