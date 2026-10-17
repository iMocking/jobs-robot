package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"

	"github.com/jsong/jobs-robot/internal/ent/schema/mixins"
	"github.com/jsong/jobs-robot/internal/pkg/ulidid"
)

// CrawlRun 是一次抓取任务的运行记录，用于可观测性与问题排查。
type CrawlRun struct {
	ent.Schema
}

// Mixin 提供 ULID 主键与 created_at / updated_at。
func (CrawlRun) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.ULID{}, mixin.Time{}}
}

// Fields 定义抓取运行字段。
func (CrawlRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("company_id").
			MaxLen(ulidid.Length).
			NotEmpty(),
		field.Enum("trigger").
			Values("schedule", "manual", "retry").
			Default("schedule"),
		field.String("triggered_by").
			MaxLen(ulidid.Length).
			Optional().
			Nillable().
			Comment("触发者用户 ID：手动=管理员，定时/自动=爬虫系统身份"),
		field.Enum("status").
			Values("running", "success", "partial", "failed", "skipped").
			Default("running").
			Comment("partial=部分成功 skipped=被 robots 或去重跳过"),
		field.String("skip_reason").
			MaxLen(255).
			Default(""),
		field.Time("started_at").
			Default(time.Now),
		field.Time("finished_at").
			Optional().
			Nillable(),
		field.Int64("duration_ms").
			Default(0),
		field.Int("pages_fetched").
			Default(0),
		field.Int("jobs_found").
			Default(0).
			Comment("页面上发现的岗位数"),
		field.Int("jobs_created").
			Default(0),
		field.Int("jobs_updated").
			Default(0),
		field.Int("jobs_unchanged").
			Default(0),
		field.Int("jobs_filtered").
			Default(0).
			Comment("因类型/行业/关键词白名单被过滤掉的岗位数"),
		field.Int("jobs_expired").
			Default(0).
			Comment("本轮判定为已下线的岗位数"),
		field.String("careers_url").
			MaxLen(512).
			Default("").
			Comment("本轮实际使用的招聘入口"),
		field.Bool("used_headless").
			Default(false).
			Comment("是否触发了无头浏览器兜底"),
		field.String("error_message").
			MaxLen(1024).
			Default(""),
		field.JSON("detail", map[string]any{}).
			Optional(),
	}
}

// Edges 定义抓取运行关联。
func (CrawlRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("company", Company.Type).
			Ref("crawl_runs").
			Field("company_id").
			Unique().
			Required(),
		edge.To("logs", CrawlLog.Type),
	}
}

// Indexes 定义抓取运行索引。
func (CrawlRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("company_id", "started_at"),
		index.Fields("status"),
	}
}
