package crawler

import (
	"strings"
)

// Scope 规定只收集哪些工作类型与行业。
//
// 它同时来自静态配置（config.Crawler.Scope）与数据库 Setting(crawler.scope)，
// 由业务层在构造爬虫前合并，因此这里保持为纯粹的值对象。
//
// 带 json 标签是为了让管理接口的请求体与响应体字段名完全一致
// （GET /admin/crawl-scope 会直接序列化本结构，缺标签会输出 Go 字段名）。
type Scope struct {
	JobTypes        []string `json:"job_types"`
	Industries      []string `json:"industries"`
	EmploymentTypes []string `json:"employment_types"`
	Locations       []string `json:"locations"`
	Keywords        []string `json:"keywords"`
	ExcludeKeywords []string `json:"exclude_keywords"`
	MinQualityScore int      `json:"min_quality_score"`
}

// Match 判定岗位是否落在收集范围内。
//
// 返回 false 时同时给出可读原因，便于在抓取日志中解释「为什么这个岗位被丢弃」。
//
// 判定顺序（先排除、后白名单）：
//  1. 命中排除关键词 → 丢弃
//  2. 配置了关键词但一个都没命中 → 丢弃
//  3. 工作类型（职位类别）不在白名单 → 丢弃
//  4. 行业不在白名单 → 丢弃（行业完全未知时放行，交由人工审核判断）
//  5. 用工性质不在白名单 → 丢弃
//  6. 地点不在白名单 → 丢弃
func (s Scope) Match(job *ExtractedJob, companyIndustry string) (bool, string) {
	if job == nil {
		return false, "岗位为空"
	}

	haystack := strings.ToLower(strings.Join([]string{
		job.Title, job.Category, job.Summary, job.Description,
		job.Responsibilities, job.Requirements, job.Department,
	}, "\n"))

	// 1) 排除关键词
	if reason, hit := matchAny(haystack, s.ExcludeKeywords, false); hit {
		return false, "命中排除关键词：" + reason
	}

	// 2) 必含关键词
	if len(s.Keywords) > 0 {
		if _, hit := matchAny(haystack, s.Keywords, false); !hit {
			return false, "未命中任何目标关键词"
		}
	}

	// 3) 工作类型
	if len(s.JobTypes) > 0 {
		category := job.Category
		if category == "" {
			category = InferCategory(job.Title, job.Description+job.Responsibilities+job.Requirements)
			job.Category = category
		}
		if category == "" {
			return false, "无法判定职位类别"
		}
		if !containsFold(s.JobTypes, category) {
			return false, "职位类别不在收集范围：" + category
		}
	}

	// 4) 行业
	if len(s.Industries) > 0 {
		industry := job.Industry
		if industry == "" {
			industry = companyIndustry
		}
		// 行业确实无法判断时不拦，避免把大量合规岗位误杀。
		if industry != "" && !containsFold(s.Industries, industry) {
			return false, "行业不在收集范围：" + industry
		}
		if industry != "" && job.Industry == "" {
			job.Industry = industry
		}
	}

	// 5) 用工性质
	if len(s.EmploymentTypes) > 0 && job.EmploymentType != "" {
		if !containsFold(s.EmploymentTypes, job.EmploymentType) {
			return false, "用工性质不在收集范围：" + job.EmploymentType
		}
	}

	// 6) 地点
	if len(s.Locations) > 0 {
		loc := strings.Join([]string{job.City, job.Province, job.Address, job.LocationText}, " ")
		if _, hit := matchAny(loc, s.Locations, true); !hit {
			return false, "工作地点不在收集范围：" + strings.TrimSpace(loc)
		}
	}

	return true, ""
}

// ScoreJob 计算岗位信息完整度评分（0~100）。
//
// 分数用于两处：低于 MinQualityScore 的直接丢弃；其余作为审核优先级参考——
// 信息越完整的岗位越值得优先审核上线。
func ScoreJob(job *ExtractedJob) int {
	if job == nil {
		return 0
	}
	score := 0

	// 岗位名称（必备）
	if strings.TrimSpace(job.Title) != "" {
		score += 10
	}
	// 岗位介绍
	if len([]rune(job.Description)) >= 20 {
		score += 10
	}
	// 任职要求——本系统最核心的字段之一
	switch n := len([]rune(job.Requirements)); {
	case n >= 50:
		score += 15
	case n >= 10:
		score += 8
	}
	if len(job.RequirementsList) >= 3 {
		score += 5
	}
	// 岗位职责
	if len([]rune(job.Responsibilities)) >= 20 {
		score += 10
	}
	// 工作地址
	if strings.TrimSpace(job.Address) != "" {
		score += 6
	}
	if strings.TrimSpace(job.City) != "" {
		score += 6
	}
	// 发布时间
	if job.PublishedAt != nil && !job.PublishedAt.IsZero() {
		score += 10
	}
	// 薪资
	if job.SalaryMin != nil || job.SalaryMax != nil {
		score += 5
	} else if strings.TrimSpace(job.SalaryText) != "" {
		score += 3
	}
	// 联系方式 / 投递入口
	if strings.TrimSpace(job.ContactEmail) != "" {
		score += 5
	}
	if strings.TrimSpace(job.ContactPhone) != "" {
		score += 3
	}
	if strings.TrimSpace(job.ApplyURL) != "" {
		score += 4
	}
	// 分类信息
	if strings.TrimSpace(job.EmploymentType) != "" {
		score += 3
	}
	if strings.TrimSpace(job.Category) != "" {
		score += 3
	}
	if strings.TrimSpace(job.Experience) != "" {
		score += 3
	}
	if strings.TrimSpace(job.Education) != "" {
		score += 2
	}
	// 福利标签
	if len(job.Tags) > 0 || strings.TrimSpace(job.Benefits) != "" {
		score += 2
	}

	if score > 100 {
		score = 100
	}
	return score
}

// categoryKeywords 是职位类别推断词表。
//
// 站点往往只在标题里写岗位名，不提供类别字段，因此需要按关键词归类；
// 归类的意义在于让「按工作类型收集」的配置真正生效。
var categoryKeywords = []struct {
	Category string
	Keywords []string
}{
	{"技术", []string{
		"工程师", "开发", "研发", "架构师", "算法", "前端", "后端", "全栈", "测试", "运维", "devops",
		"数据", "大数据", "ai", "人工智能", "机器学习", "深度学习", "安全", "sre", "技术", "程序",
		"android", "ios", "java", "golang", "python", "c++", "php", "嵌入式", "硬件",
	}},
	{"产品", []string{"产品经理", "产品", "pm", "需求分析", "交互设计", "product"}},
	{"设计", []string{"设计师", "设计", "ui", "ue", "视觉", "平面", "美工", "设计部"}},
	{"运营", []string{"运营", "客服", "内容", "编辑", "新媒体", "社群", "用户增长"}},
	{"市场", []string{"市场", "营销", "销售", "商务", "品牌", "公关", "渠道", "bd", "拓展"}},
	{"职能", []string{
		"人力资源", "hr", "招聘", "行政", "财务", "会计", "法务", "审计", "董秘", "助理", "秘书",
		"供应链", "采购", "物流", "仓储", "风控", "合规",
	}},
}

// InferCategory 根据岗位名称与正文推断职位类别；无法判断时返回空串。
func InferCategory(title, body string) string {
	titleLower := strings.ToLower(title)
	bodyLower := strings.ToLower(body)

	// 标题的权重更高，先只在标题中找。
	for _, entry := range categoryKeywords {
		for _, kw := range entry.Keywords {
			if strings.Contains(titleLower, kw) {
				return entry.Category
			}
		}
	}
	// 再退化为全文匹配。
	for _, entry := range categoryKeywords {
		for _, kw := range entry.Keywords {
			if strings.Contains(bodyLower, kw) {
				return entry.Category
			}
		}
	}
	return ""
}

// InferEmploymentType 从文本中推断用工性质。
func InferEmploymentType(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "实习") || strings.Contains(lower, "intern"):
		return "实习"
	case strings.Contains(lower, "校招") || strings.Contains(lower, "校园招聘") || strings.Contains(lower, "应届") || strings.Contains(lower, "campus"):
		return "校招"
	case strings.Contains(lower, "社招") || strings.Contains(lower, "社会招聘"):
		return "社招"
	case strings.Contains(lower, "兼职") || strings.Contains(lower, "part-time"):
		return "兼职"
	case strings.Contains(lower, "全职") || strings.Contains(lower, "full-time"):
		return "全职"
	default:
		return ""
	}
}

func containsFold(list []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.EqualFold(item, target) || strings.Contains(target, item) || strings.Contains(item, target) {
			return true
		}
	}
	return false
}

// matchAny 返回第一个命中的关键词；exact=false 时忽略大小写做包含匹配。
func matchAny(haystack string, needles []string, _ bool) (string, bool) {
	haystack = strings.ToLower(haystack)
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle == "" {
			continue
		}
		if strings.Contains(haystack, needle) {
			return needle, true
		}
	}
	return "", false
}
