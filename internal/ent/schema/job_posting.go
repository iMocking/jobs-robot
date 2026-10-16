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

// JobPosting 是企业官网直招岗位，是系统的核心数据实体。
//
// 字段分组：
//   - 来源标识：source_url / source_site / is_direct_hire —— 保证「直招」可追溯
//   - 岗位内容：title / description / responsibilities / requirements ...
//   - 联系方式：contact_* / apply_url / apply_method
//   - 时间信息：published_at（发布时间）/ last_updated_at（最后更新时间）/ last_seen_at（最近仍在架）
//   - 抓取元数据：content_hash / revision / quality_score / extractor
//   - 审核状态：status / reviewed_at / reviewer
type JobPosting struct {
	ent.Schema
}

// Mixin 提供 ULID 主键与 created_at / updated_at。
func (JobPosting) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.ULID{}, mixin.Time{}}
}

// Fields 定义岗位字段。
func (JobPosting) Fields() []ent.Field {
	return []ent.Field{
		// —— 来源标识 ——
		field.String("source_url").
			MaxLen(1024).
			NotEmpty().
			Unique().
			Comment("岗位详情页 URL，与公司构成唯一来源标识"),
		field.String("source_site").
			MaxLen(255).
			Default("").
			Comment("来源域名"),
		field.String("source_company_name").
			MaxLen(255).
			Default("").
			Comment("来源页面上标注的公司名，可能与库中公司名不一致"),
		field.String("source_job_id").
			MaxLen(128).
			Default("").
			Comment("企业侧岗位编号（若页面提供）"),
		field.Bool("is_direct_hire").
			Default(true).
			Comment("是否判定为官网直招（来源为官网加入我们页面）"),
		field.String("apply_url").
			MaxLen(1024).
			Default(""),
		field.String("company_id").
			MaxLen(ulidid.Length).
			NotEmpty().
			Comment("所属公司"),

		// —— 岗位内容 ——
		field.String("title").
			MaxLen(255).
			NotEmpty().
			Comment("岗位名称"),
		field.String("job_code").
			MaxLen(64).
			Default(""),
		field.String("category").
			MaxLen(64).
			Default("").
			Comment("职位类别（工作类型），需命中收集范围白名单"),
		field.String("industry").
			MaxLen(64).
			Default("").
			Comment("所属行业，继承自公司或来源页面"),
		field.String("employment_type").
			MaxLen(32).
			Default("").
			Comment("用工性质：全职/兼职/实习/校招/社招"),
		field.String("department").
			MaxLen(128).
			Default(""),
		field.Int("headcount").
			Optional().
			Nillable().
			Comment("招聘人数"),
		field.String("experience").
			MaxLen(64).
			Default("").
			Comment("经验要求，如 3-5 年"),
		field.String("education").
			MaxLen(64).
			Default("").
			Comment("学历要求"),
		field.String("work_mode").
			MaxLen(32).
			Default("").
			Comment("坐班/远程/混合"),
		field.String("salary_text").
			MaxLen(128).
			Default("").
			Comment("薪资原始文本，如 25-40K·14薪"),
		field.Int("salary_min").
			Optional().
			Nillable().
			Comment("月薪下限（元）"),
		field.Int("salary_max").
			Optional().
			Nillable().
			Comment("月薪上限（元）"),
		field.Int("salary_months").
			Optional().
			Nillable().
			Comment("薪资月数，如 14 薪"),

		// —— 工作地址 ——
		field.String("country").
			MaxLen(64).
			Default("中国"),
		field.String("province").
			MaxLen(64).
			Default(""),
		field.String("city").
			MaxLen(64).
			Default("").
			Comment("城市，用于前台筛选"),
		field.String("district").
			MaxLen(64).
			Default(""),
		field.String("address").
			MaxLen(512).
			Default("").
			Comment("工作地址（详细）"),
		field.String("location_text").
			MaxLen(255).
			Default("").
			Comment("来源页面原始地点文本"),

		// —— 介绍与要求 ——
		field.String("summary").
			MaxLen(512).
			Default("").
			Comment("岗位一句话简介"),
		field.Text("description").
			Optional().
			Default("").
			Comment("岗位介绍"),
		field.Text("responsibilities").
			Optional().
			Default("").
			Comment("岗位职责"),
		field.Text("requirements").
			Optional().
			Default("").
			Comment("任职要求（原文汇总）"),
		field.Strings("requirements_list").
			Optional().
			Comment("任职要求结构化条目"),
		field.Text("benefits").
			Optional().
			Default("").
			Comment("福利待遇"),
		field.Strings("tags").
			Optional().
			Comment("标签，如 五险一金/弹性工作"),

		// —— 联系方式 ——
		field.String("contact_person").
			MaxLen(64).
			Default(""),
		field.String("contact_email").
			MaxLen(255).
			Default(""),
		field.String("contact_phone").
			MaxLen(64).
			Default(""),
		field.String("contact_wechat").
			MaxLen(64).
			Default(""),
		field.String("apply_method").
			MaxLen(64).
			Default("").
			Comment("投递方式：官网投递/邮箱投递/链接投递"),

		// —— 时间信息 ——
		field.Time("published_at").
			Optional().
			Nillable().
			Comment("发布时间"),
		field.Time("deadline_at").
			Optional().
			Nillable().
			Comment("投递截止时间"),
		field.Time("source_updated_at").
			Optional().
			Nillable().
			Comment("来源页面标注的最后更新时间"),
		field.Time("last_updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("本系统感知到的最后更新时间：内容发生变化时刷新"),
		field.Time("last_seen_at").
			Default(time.Now).
			Comment("最近一次抓取时该岗位仍在架"),
		field.Time("crawled_at").
			Default(time.Now).
			Comment("最近一次抓取时间"),

		// —— 抓取元数据 ——
		field.String("content_hash").
			MaxLen(64).
			Default("").
			Comment("岗位内容指纹，用于判断是否发生变更"),
		field.Int("revision").
			Default(1).
			Comment("内容修订号，每次变更 +1"),
		field.Int("quality_score").
			Default(0).
			Comment("信息完整度评分 0~100"),
		field.Float("parse_confidence").
			Default(0).
			Comment("解析置信度 0~1"),
		field.String("extractor").
			MaxLen(32).
			Default("").
			Comment("抽取器：jsonld/dom/rules/headless"),
		field.JSON("raw_json", map[string]any{}).
			Optional().
			Comment("抽取出的原始结构化数据，便于回溯"),
		field.String("snapshot_path").
			MaxLen(512).
			Default("").
			Comment("原始 HTML 快照路径（开启快照时）"),
		field.Int("view_count").
			Default(0),

		// —— 审核状态 ——
		field.Enum("status").
			Values("pending_review", "approved", "rejected", "offline", "expired").
			Default("pending_review").
			Comment("pending_review=待审核 approved=已发布 rejected=已驳回 offline=已下线 expired=已过期"),
		field.String("reviewer_id").
			MaxLen(ulidid.Length).
			Optional().
			Nillable().
			Comment("审核人"),
		field.Time("reviewed_at").
			Optional().
			Nillable(),
		field.String("reject_reason").
			MaxLen(512).
			Default(""),
		field.Time("expired_at").
			Optional().
			Nillable().
			Comment("判定下线的检测时间"),
	}
}

// Edges 定义岗位关联。
func (JobPosting) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("company", Company.Type).
			Ref("jobs").
			Field("company_id").
			Unique().
			Required().
			Comment("所属公司"),
		edge.From("reviewer", User.Type).
			Ref("reviewed_jobs").
			Field("reviewer_id").
			Unique().
			Comment("审核人"),
		edge.To("revisions", JobRevision.Type).
			Comment("内容变更历史，用于追溯最后更新时间的成因"),
	}
}

// Indexes 定义岗位索引。
func (JobPosting) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "published_at"),
		index.Fields("company_id", "status"),
		index.Fields("content_hash"),
		index.Fields("category"),
		index.Fields("industry"),
		index.Fields("city"),
		index.Fields("last_seen_at"),
	}
}
