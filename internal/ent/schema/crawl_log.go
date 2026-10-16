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

// CrawlLog 是抓取过程中单个步骤的明细日志。
type CrawlLog struct {
	ent.Schema
}

// Mixin 提供 ULID 主键与 created_at / updated_at。
func (CrawlLog) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.ULID{}, mixin.Time{}}
}

// Fields 定义抓取日志字段。
func (CrawlLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("run_id").
			MaxLen(ulidid.Length).
			NotEmpty(),
		field.Enum("level").
			Values("debug", "info", "warn", "error").
			Default("info"),
		field.String("stage").
			MaxLen(32).
			Default("").
			Comment("阶段：discover/fetch/parse/filter/persist/robots/headless"),
		field.String("url").
			MaxLen(1024).
			Default(""),
		field.String("message").
			MaxLen(1024).
			Default(""),
		field.JSON("context", map[string]any{}).
			Optional(),
	}
}

// Edges 定义抓取日志关联。
func (CrawlLog) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("run", CrawlRun.Type).
			Ref("logs").
			Field("run_id").
			Unique().
			Required(),
	}
}

// Indexes 定义抓取日志索引。
func (CrawlLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("run_id", "level"),
	}
}
