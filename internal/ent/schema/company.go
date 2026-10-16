package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"

	"github.com/jsong/jobs-robot/internal/ent/schema/mixins"
)

// Company 是招聘来源公司。
//
// careers_url 即「《加入我们》超链接」指向的招聘页地址：既可由管理员手工录入，
// 也可由爬虫从官网首页自动发现后回填（此时 discovery_url 记录发现来源页面）。
type Company struct {
	ent.Schema
}

// Mixin 提供 ULID 主键与 created_at / updated_at。
func (Company) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.ULID{}, mixin.Time{}}
}

// Fields 定义公司字段。
func (Company) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(255).
			NotEmpty().
			Unique().
			Comment("公司名称"),
		field.String("short_name").
			MaxLen(128).
			Default(""),
		field.String("website").
			MaxLen(512).
			Default("").
			Comment("官网首页地址，爬虫发现入口的起点"),
		field.String("careers_url").
			MaxLen(512).
			Default("").
			Comment("《加入我们》链接指向的招聘页，直招岗位的采集入口"),
		field.String("discovery_url").
			MaxLen(512).
			Default("").
			Comment("发现 careers_url 的来源页面"),
		field.String("logo_url").
			MaxLen(512).
			Default(""),
		field.String("industry").
			MaxLen(64).
			Default("").
			Comment("所属行业，需命中爬虫收集范围白名单才采集"),
		field.String("sub_industry").
			MaxLen(64).
			Default(""),
		field.String("scale").
			MaxLen(32).
			Default("").
			Comment("人员规模，如 1000-5000 人"),
		field.String("nature").
			MaxLen(32).
			Default("").
			Comment("企业性质，如 民营/国企/外企"),
		field.String("location").
			MaxLen(255).
			Default("").
			Comment("总部/主要办公地"),
		field.Text("description").
			Optional().
			Default("").
			Comment("公司简介"),
		field.String("contact_email").
			MaxLen(255).
			Default(""),
		field.String("contact_phone").
			MaxLen(64).
			Default(""),
		field.Bool("enabled").
			Default(true).
			Comment("是否启用；关闭后不参与抓取且其岗位前台不可见"),
		field.Bool("verified").
			Default(false).
			Comment("是否已人工核实为官网直招来源"),
		field.Int("priority").
			Default(0).
			Comment("抓取优先级，数值越大越先被抓取"),

		// —— 抓取配置 ——
		field.Bool("crawl_enabled").
			Default(true),
		field.Int("crawl_interval_seconds").
			Default(21600).
			Comment("该公司独立抓取间隔（秒），默认 6 小时"),
		field.JSON("crawl_rules", map[string]any{}).
			Optional().
			Comment("站点自定义抓取规则：入口选择器、字段选择器等"),
		field.Bool("force_headless").
			Default(false).
			Comment("强制使用无头浏览器渲染（SPA 站点）"),
		field.Bool("respect_robots").
			Default(true).
			Comment("是否遵循该站点 robots.txt"),
		field.Time("last_crawled_at").
			Optional().
			Nillable(),
		field.String("last_crawl_status").
			MaxLen(32).
			Default(""),
		field.String("last_crawl_error").
			MaxLen(1024).
			Default(""),
		field.Int("job_count").
			Default(0).
			Comment("当前在架已发布岗位数（冗余统计）"),
		field.Time("robots_checked_at").
			Optional().
			Nillable(),
		field.Bool("robots_allowed").
			Default(true),
		field.String("source").
			MaxLen(32).
			Default("manual").
			Comment("来源：manual=后台录入 discovered=爬虫发现"),
		field.String("remark").
			MaxLen(512).
			Default(""),
	}
}

// Edges 定义公司关联。
func (Company) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("jobs", JobPosting.Type).
			Comment("该公司采集到的岗位"),
		edge.To("crawl_runs", CrawlRun.Type).
			Comment("该公司的抓取运行记录"),
	}
}

// Indexes 定义公司索引。
func (Company) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("industry"),
		index.Fields("enabled", "crawl_enabled"),
		index.Fields("last_crawled_at"),
	}
}
