// extract.go 实现岗位详情页的字段抽取。
//
// 抽取按「结构化程度」分层，命中即用并记录 Extractor：
//
//	jsonld（0.90）> rules（0.85，站点自定义选择器）> dom（0.60，通用启发式）
//
// 只要页面提供了 schema.org 的 JobPosting 结构化数据就优先采信——它是站点主动
// 声明的机器可读数据，准确率远高于任何启发式；没有结构化数据时才退化为规则或
// DOM 推断。无论走哪条路径，最后都会过一遍统一后处理：薪资换算、地点拆分、
// 任职要求列表化、联系方式正则兜底、日期解析。
package crawler

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// ExtractJob 从岗位详情页抽取结构化岗位信息。
func ExtractJob(doc *Document, rules *SiteRules, base *url.URL) (*ExtractedJob, error) {
	if doc == nil || strings.TrimSpace(doc.HTML) == "" {
		return nil, apperr.ErrParseFailed.WithMessage("页面内容为空，无法抽取岗位信息")
	}
	if rules == nil {
		rules = &SiteRules{}
	}

	sourceURL := firstNonEmpty(strings.TrimSpace(doc.FinalURL), strings.TrimSpace(doc.URL))
	if base == nil {
		base = mustParseFirst(sourceURL)
	}

	parsed, err := goquery.NewDocumentFromReader(strings.NewReader(doc.HTML))
	if err != nil {
		return nil, apperr.ErrParseFailed.WithCause(err).WithMessage("页面 HTML 解析失败")
	}

	job := &ExtractedJob{
		SourceURL:  sourceURL,
		Extractor:  "dom",
		Confidence: 0.6,
		Raw: map[string]any{
			"headless": doc.Headless,
		},
	}

	switch {
	case applyJSONLD(job, parsed):
		job.Extractor = "jsonld"
		job.Confidence = 0.9
	case applyRulesFields(job, parsed, rules, base):
		job.Extractor = "rules"
		job.Confidence = 0.85
	default:
		applyDOMHeuristics(job, parsed, base)
		job.Extractor = "dom"
		job.Confidence = 0.6
	}
	job.Raw["extractor"] = job.Extractor

	// 统一后处理：三种抽取器共享同一套字段规整逻辑。
	postProcess(job, pageTextOf(parsed, doc.HTML))

	if job.Title == "" {
		return nil, apperr.ErrParseFailed.WithMessage("未能从页面中抽取到岗位名称，可能不是岗位详情页：" + sourceURL)
	}
	if isGenericTitle(job.Title) {
		return nil, apperr.ErrParseFailed.WithMessage("页面标题是站点通用页（如登录/首页），不是岗位详情页：" + job.Title)
	}
	return job, nil
}

// —— 1) schema.org JobPosting ——

// applyJSONLD 解析页面中的 application/ld+json，找到 JobPosting 并映射字段。
func applyJSONLD(job *ExtractedJob, parsed *goquery.Document) bool {
	if parsed == nil {
		return false
	}
	var node map[string]any
	parsed.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		payload, ok := decodeJSONLoose(s.Text())
		if !ok {
			return true
		}
		if found := findJobPostingNode(payload); found != nil {
			node = found
			return false
		}
		return true
	})
	if node == nil {
		return false
	}

	job.Raw["jsonld"] = node

	job.Title = cleanInlineText(stringValue(node["title"]))
	if desc := stringValue(node["description"]); desc != "" {
		text := htmlToText(desc)
		job.Description = text
		job.Responsibilities, job.Requirements = splitDescriptionSections(text)
	}
	job.PublishedAt = parseDateText(stringValue(node["datePosted"]))
	job.DeadlineAt = parseDateText(stringValue(node["validThrough"]))
	job.SourceUpdatedAt = parseDateText(stringValue(node["dateModified"]))
	job.EmploymentType = mapEmploymentType(stringValue(node["employmentType"]))
	job.Industry = stringValue(node["industry"])
	job.Experience = experienceString(node["experienceRequirements"])
	job.Education = stringValue(node["educationRequirements"])
	job.SourceJobID = identifierString(node["identifier"])
	if n, ok := numberValue(node["totalJobOpenings"]); ok && n > 0 {
		count := int(n)
		job.Headcount = &count
	}

	if org := asMap(node["hiringOrganization"]); org != nil {
		job.SourceCompanyName = firstNonEmpty(stringValue(org["name"]), stringValue(org["legalName"]))
	}

	// 地点：jobLocation 可能是对象、对象数组，address 可能是字符串或对象。
	for _, loc := range listValue(node["jobLocation"]) {
		applyAddress(job, loc)
	}
	if job.City == "" && job.Address == "" {
		applyAddress(job, asMap(node["jobLocation"]))
	}

	if mode := strings.ToUpper(stringValue(node["jobLocationType"])); mode != "" {
		if strings.Contains(mode, "TELECOMMUTE") {
			job.WorkMode = "远程"
		}
	}

	if salary := asMap(node["baseSalary"]); salary != nil {
		applyBaseSalary(job, salary)
	}

	// title 为空说明这条 JobPosting 没有可用信息，交给下一层抽取器。
	return job.Title != ""
}

// decodeJSONLoose 宽松解析 JSON：部分站点会在脚本里混入注释或多余字符。
func decodeJSONLoose(raw string) (any, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, false
	}
	var payload any
	if err := json.Unmarshal([]byte(text), &payload); err == nil {
		return payload, true
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		start = strings.Index(text, "[")
		end = strings.LastIndex(text, "]")
	}
	if start < 0 || end <= start {
		return nil, false
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return nil, false
	}
	return payload, true
}

// findJobPostingNode 在任意嵌套结构中寻找 @type 为 JobPosting 的节点。
func findJobPostingNode(v any) map[string]any {
	switch node := v.(type) {
	case map[string]any:
		if isJobPostingType(node["@type"]) {
			return node
		}
		// 常见容器优先，减少无意义递归。
		for _, key := range []string{"@graph", "itemListElement", "mainEntity", "mainEntityOfPage", "item", "hasPart"} {
			if child, ok := node[key]; ok {
				if found := findJobPostingNode(child); found != nil {
					return found
				}
			}
		}
		for _, child := range node {
			if found := findJobPostingNode(child); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range node {
			if found := findJobPostingNode(child); found != nil {
				return found
			}
		}
	}
	return nil
}

// isJobPostingType 判断 @type 是否为 JobPosting（大小写不敏感，支持数组）。
func isJobPostingType(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "JobPosting")
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok && strings.EqualFold(strings.TrimSpace(s), "JobPosting") {
				return true
			}
		}
	}
	return false
}

// applyAddress 把 schema.org 的 Place/PostalAddress 映射到岗位地点字段。
func applyAddress(job *ExtractedJob, loc map[string]any) {
	if job == nil || loc == nil {
		return
	}
	addr := asMap(loc["address"])
	if addr == nil {
		// 有些站点把地址字段直接放在 jobLocation 上。
		addr = loc
	}
	if job.City == "" {
		job.City = stringValue(addr["addressLocality"])
	}
	if job.Province == "" {
		job.Province = stringValue(addr["addressRegion"])
	}
	if job.Address == "" {
		job.Address = stringValue(addr["streetAddress"])
	}
	if job.Country == "" {
		country := addr["addressCountry"]
		if m := asMap(country); m != nil {
			job.Country = firstNonEmpty(stringValue(m["name"]), stringValue(m["@value"]))
		} else {
			job.Country = stringValue(country)
		}
	}
	// 结构化地址里可能只有一整串文本，交给地点解析兜底。
	if job.LocationText == "" {
		job.LocationText = cleanInlineText(strings.Join([]string{
			job.Country, job.Province, job.City, job.Address,
		}, ""))
	}
}

// applyBaseSalary 把 schema.org 的 MonetaryAmount 映射到薪资字段。
func applyBaseSalary(job *ExtractedJob, salary map[string]any) {
	currency := strings.ToUpper(stringValue(salary["currency"]))
	value := asMap(salary["value"])

	var minV, maxV float64
	var hasMin, hasMax bool
	unit := ""
	if value != nil {
		minV, hasMin = numberValue(value["minValue"])
		maxV, hasMax = numberValue(value["maxValue"])
		if !hasMax {
			maxV, hasMax = numberValue(value["value"])
		}
		unit = strings.ToUpper(stringValue(value["unitText"]))
	}
	if !hasMin && !hasMax {
		if n, ok := numberValue(salary["value"]); ok {
			minV, maxV, hasMin, hasMax = n, n, true, true
		}
	}
	if !hasMin && !hasMax {
		return
	}

	toMonthly := func(v float64) float64 {
		switch {
		case strings.Contains(unit, "YEAR"), strings.Contains(unit, "ANNUM"):
			return v / 12
		case strings.Contains(unit, "WEEK"):
			return v * (52.0 / 12.0)
		case strings.Contains(unit, "DAY"):
			return v * daysPerMonth
		case strings.Contains(unit, "HOUR"):
			return v * daysPerMonth * hoursPerDay
		default:
			return v
		}
	}
	if hasMin {
		n := int(math.Round(toMonthly(minV)))
		job.SalaryMin = &n
	}
	if hasMax {
		n := int(math.Round(toMonthly(maxV)))
		job.SalaryMax = &n
	}
	job.SalaryText = formatSalaryText(job.SalaryMin, job.SalaryMax, currency, unit)
}

// formatSalaryText 依据数值生成可读薪资文本。
func formatSalaryText(minV, maxV *int, currency, unit string) string {
	suffix := "元/月"
	switch {
	case strings.Contains(unit, "YEAR"), strings.Contains(unit, "ANNUM"):
		suffix = "元/年"
	case strings.Contains(unit, "WEEK"):
		suffix = "元/周"
	case strings.Contains(unit, "DAY"):
		suffix = "元/天"
	case strings.Contains(unit, "HOUR"):
		suffix = "元/小时"
	}
	if currency != "" && currency != "CNY" && currency != "RMB" {
		suffix = currency + "/月"
	}
	switch {
	case minV != nil && maxV != nil && *minV != *maxV:
		return fmt.Sprintf("%d-%d %s", *minV, *maxV, suffix)
	case minV != nil:
		return fmt.Sprintf("%d %s", *minV, suffix)
	case maxV != nil:
		return fmt.Sprintf("%d %s", *maxV, suffix)
	default:
		return ""
	}
}

// —— 2) 站点自定义选择器 ——

// applyRulesFields 按 SiteRules.DetailFields 抽取字段；命中任意字段返回 true。
func applyRulesFields(job *ExtractedJob, parsed *goquery.Document, rules *SiteRules, base *url.URL) bool {
	if parsed == nil || len(rules.DetailFields) == 0 {
		return false
	}
	hit := false

	for field, selector := range rules.DetailFields {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		node := parsed.Find(selector).First()
		if node.Length() == 0 {
			continue
		}
		name := normalizeFieldName(field)
		text := cleanInlineText(node.Text())
		inner := htmlToText(nodeInnerHTML(node))

		switch name {
		case "title", "job_title", "name":
			job.Title = cleanTitle(firstNonEmpty(text, job.Title))
		case "description", "job_description":
			job.Description = firstNonEmpty(inner, text, job.Description)
		case "responsibilities", "duty", "duties":
			job.Responsibilities = firstNonEmpty(inner, text, job.Responsibilities)
		case "requirements", "qualifications":
			job.Requirements = firstNonEmpty(inner, text, job.Requirements)
		case "benefits", "welfare":
			job.Benefits = firstNonEmpty(inner, text, job.Benefits)
		case "salary", "salary_text", "compensation":
			job.SalaryText = firstNonEmpty(text, job.SalaryText)
		case "location", "location_text":
			job.LocationText = firstNonEmpty(text, job.LocationText)
		case "address":
			job.Address = firstNonEmpty(text, job.Address)
		case "city":
			job.City = firstNonEmpty(text, job.City)
		case "province", "region":
			job.Province = firstNonEmpty(text, job.Province)
		case "district":
			job.District = firstNonEmpty(text, job.District)
		case "country":
			job.Country = firstNonEmpty(text, job.Country)
		case "experience":
			job.Experience = firstNonEmpty(text, job.Experience)
		case "education":
			job.Education = firstNonEmpty(text, job.Education)
		case "employment_type", "job_type":
			job.EmploymentType = firstNonEmpty(mapEmploymentType(text), InferEmploymentType(text), job.EmploymentType)
		case "category", "job_category":
			job.Category = firstNonEmpty(text, job.Category)
		case "industry":
			job.Industry = firstNonEmpty(text, job.Industry)
		case "department", "team":
			job.Department = firstNonEmpty(text, job.Department)
		case "work_mode":
			job.WorkMode = firstNonEmpty(text, job.WorkMode)
		case "contact_email", "email":
			job.ContactEmail = firstNonEmpty(findEmail(text), text, job.ContactEmail)
		case "contact_phone", "phone", "tel":
			job.ContactPhone = firstNonEmpty(findPhone(text), text, job.ContactPhone)
		case "contact_person", "contact":
			job.ContactPerson = firstNonEmpty(text, job.ContactPerson)
		case "contact_wechat", "wechat":
			job.ContactWechat = firstNonEmpty(text, job.ContactWechat)
		case "apply_url", "apply_link":
			href, _ := node.Attr("href")
			abs, ok := absoluteHTTPURL(base, firstNonEmpty(href, text))
			if ok {
				job.ApplyURL = firstNonEmpty(job.ApplyURL, abs)
			} else if job.ApplyURL == "" {
				job.ApplyURL = firstNonEmpty(text, job.ApplyURL)
			}
		case "published_at", "publish_time", "date_posted":
			if t := parseDateText(firstNonEmpty(text, attrValue(node, "datetime"))); t != nil {
				job.PublishedAt = t
			}
		case "deadline_at", "deadline", "valid_through":
			if t := parseDateText(firstNonEmpty(text, attrValue(node, "datetime"))); t != nil {
				job.DeadlineAt = t
			}
		case "source_job_id", "job_id", "job_code":
			job.SourceJobID = firstNonEmpty(job.SourceJobID, text)
		case "industry_tags", "tags", "labels":
			if len(job.Tags) == 0 {
				job.Tags = splitTagText(text)
			}
		case "headcount", "openings", "total_job_openings":
			if job.Headcount == nil {
				if n, ok := firstInt(text); ok {
					job.Headcount = &n
				}
			}
		default:
			// 未知字段名不报错：规则是运营配置的，多写字段不该导致抓取失败。
			continue
		}
		hit = true
	}
	return hit
}

// normalizeFieldName 统一字段名写法（大小写、连字符、空格）。
func normalizeFieldName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(name)
	return strings.Trim(name, "_")
}

// attrValue 取节点属性（不存在时返回空串）。
func attrValue(node *goquery.Selection, name string) string {
	if node == nil || node.Length() == 0 {
		return ""
	}
	if v, ok := node.Attr(name); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// —— 3) DOM 通用启发式 ——

// 各类区块的标题关键词。命中靠前的词优先，泛化词放在最后作为兜底。
var (
	responsibilityHeadings = []string{
		"岗位职责", "工作内容", "工作职责", "职责描述", "职位描述", "岗位描述", "主要职责", "工作说明",
		"responsibilities", "job description", "what you will do",
	}
	requirementHeadings = []string{
		"任职要求", "任职资格", "岗位要求", "职位要求", "任职条件", "资格要求", "应聘条件", "岗位任职要求", "我们期待",
		"qualifications", "requirements", "what we are looking for", "who you are",
	}
	summaryHeadings = []string{
		"岗位介绍", "职位介绍", "岗位概述", "职位概述", "岗位简介", "职位概要",
		"job summary", "position summary", "about the role", "about this role",
	}
	benefitHeadings = []string{
		"福利待遇", "薪资福利", "薪酬福利", "员工福利", "我们提供", "福利", "待遇",
		"benefits", "perks", "what we offer",
	}
	locationHeadings   = []string{"工作地点", "办公地点", "工作地址", "职位地点", "上班地点", "工作城市", "location"}
	departmentHeadings = []string{
		"所属部门", "所属团队", "职位类别", "岗位类别", "职能类别", "部门", "团队",
		"department", "category", "team",
	}
	experienceHeadings = []string{"工作经验", "经验要求", "工作年限", "经验", "experience"}
	educationHeadings  = []string{"学历要求", "最低学历", "学历", "education"}
	salaryHeadings     = []string{"薪资范围", "薪资", "薪酬", "月薪", "年薪", "工资", "salary", "compensation"}
	headcountHeadings  = []string{"招聘人数", "需求人数", "招聘数量", "人数", "headcount", "openings"}
)

// sectionHeadingKeywords 是「新章节开始」的判定词，用于截断区块收集。
var sectionHeadingKeywords = concatStrings(
	responsibilityHeadings, requirementHeadings, summaryHeadings, benefitHeadings,
	locationHeadings, departmentHeadings, experienceHeadings, educationHeadings,
	salaryHeadings, headcountHeadings,
)

// applyDOMHeuristics 用标签语义 + 类名关键词做通用抽取。
func applyDOMHeuristics(job *ExtractedJob, parsed *goquery.Document, base *url.URL) {
	if parsed == nil {
		return
	}

	// 标题：h1 最可信，其次是语义类名、og:title，最后退化到 <title>。
	if job.Title == "" {
		job.Title = cleanTitle(firstNonEmpty(
			cleanInlineText(parsed.Find("h1").First().Text()),
			cleanInlineText(parsed.Find(".job-title, .jobTitle, [class*=job-title], [class*=jobTitle], [class*=position-title], [class*=positionTitle]").First().Text()),
			metaContent(parsed, `meta[property="og:title"]`),
			cleanInlineText(parsed.Find("title").First().Text()),
		))
	}

	mainText := mainContentText(parsed)

	if job.Responsibilities == "" {
		job.Responsibilities = firstNonEmpty(
			findSectionText(parsed, responsibilityHeadings),
			findSectionText(parsed, []string{"职责"}),
		)
	}
	if job.Requirements == "" {
		job.Requirements = firstNonEmpty(
			findSectionText(parsed, requirementHeadings),
			findSectionText(parsed, []string{"要求", "条件"}),
		)
	}
	if job.Summary == "" {
		job.Summary = findSectionText(parsed, summaryHeadings)
	}
	if job.Benefits == "" {
		job.Benefits = findSectionText(parsed, benefitHeadings)
	}
	if job.Description == "" {
		job.Description = mainText
	}

	if job.LocationText == "" {
		job.LocationText = findLabeledValue(parsed, locationHeadings)
	}
	if job.Department == "" {
		job.Department = findLabeledValue(parsed, departmentHeadings)
	}
	if job.Category == "" {
		job.Category = findLabeledValue(parsed, []string{"职位类别", "岗位类别", "职能类别", "职位类型", "category"})
	}
	if job.SalaryText == "" {
		job.SalaryText = findLabeledValue(parsed, salaryHeadings)
	}
	if job.Experience == "" {
		job.Experience = findLabeledValue(parsed, experienceHeadings)
	}
	if job.Education == "" {
		job.Education = findLabeledValue(parsed, educationHeadings)
	}
	if job.EmploymentType == "" {
		job.EmploymentType = InferEmploymentType(job.Title + "\n" + mainText)
	}
	if job.WorkMode == "" && containsAnyFold(mainText, []string{"远程办公", "居家办公", "可远程", "remote work", "work from home"}) {
		job.WorkMode = "远程"
	}
	if job.Headcount == nil {
		if value := findLabeledValue(parsed, headcountHeadings); value != "" {
			if n, ok := firstInt(value); ok && n > 0 {
				job.Headcount = &n
			}
		}
	}

	// 时间：meta 与 <time> 优先，其次「发布时间」等标签后的日期文本。
	if job.PublishedAt == nil {
		job.PublishedAt = firstTime(
			parseDateText(metaContent(parsed, `meta[property="article:published_time"]`)),
			parseDateText(attrValue(parsed.Find("time[datetime]").First(), "datetime")),
			parseDateText(findLabeledValue(parsed, []string{"发布时间", "发布日期", "发布时间", "posted", "published"})),
			dateAfterLabel(mainText, publishDateRe),
		)
	}
	if job.SourceUpdatedAt == nil {
		job.SourceUpdatedAt = firstTime(
			parseDateText(metaContent(parsed, `meta[property="article:modified_time"]`)),
			parseDateText(findLabeledValue(parsed, []string{"更新时间", "更新日期", "最后更新", "最新更新", "updated"})),
			dateAfterLabel(mainText, updateDateRe),
		)
	}

	// 联系方式：正文正则兜底。
	if job.ContactEmail == "" {
		job.ContactEmail = findEmail(mainText)
	}
	if job.ContactPhone == "" {
		job.ContactPhone = findPhone(mainText)
	}
	if job.ContactWechat == "" {
		job.ContactWechat = findWechat(mainText)
	}

	// 投递入口。
	if job.ApplyURL == "" {
		job.ApplyURL = findApplyURL(parsed, base)
	}
	if len(job.Tags) == 0 {
		job.Tags = splitTagText(job.Benefits)
	}
}

// mainContentText 返回页面主内容文本，作为岗位描述的整体兜底。
func mainContentText(parsed *goquery.Document) string {
	if parsed == nil {
		return ""
	}
	for _, selector := range []string{"main", "#content", ".content", "#main", ".main", "article", ".job-detail", ".detail"} {
		node := parsed.Find(selector).First()
		if node.Length() == 0 {
			continue
		}
		if text := htmlToText(nodeInnerHTML(node)); utf8.RuneCountInString(text) >= 20 {
			return text
		}
	}
	return htmlToText(nodeInnerHTML(parsed.Find("body").First()))
}

// findSectionText 寻找标题匹配关键词的区块正文。
//
// 收集规则：先看标题节点自身的可视文本（形如「岗位职责：xxx」），再把后续兄弟
// 节点一路收集到下一个章节标题为止；兄弟节点里没有内容时，退化为父容器的内容
// 去掉标题文本。多个候选时取最长的一个——内容越完整越可能是真正的区块。
func findSectionText(parsed *goquery.Document, keywords []string) string {
	if parsed == nil || len(keywords) == 0 {
		return ""
	}
	best := ""
	examined := 0
	parsed.Find("h1,h2,h3,h4,h5,h6,strong,b,dt,div,span,p,td,th,li").Each(func(_ int, s *goquery.Selection) {
		examined++
		if examined > 4000 {
			return
		}
		text := cleanInlineText(s.Text())
		if text == "" || utf8.RuneCountInString(text) > 40 {
			return
		}
		if !headingMatches(text, keywords) {
			return
		}
		section := collectFollowingText(s)
		if len(section) > len(best) {
			best = section
		}
	})
	return best
}

// headingMatches 判断节点文本是否以某个章节关键词开头。
func headingMatches(text string, keywords []string) bool {
	key := normalizeKey(text)
	if key == "" {
		return false
	}
	for _, kw := range keywords {
		k := normalizeKey(kw)
		if k != "" && strings.HasPrefix(key, k) {
			return true
		}
	}
	return false
}

// collectFollowingText 收集标题节点之后的正文，直到下一个章节标题。
func collectFollowingText(s *goquery.Selection) string {
	if s == nil || s.Length() == 0 {
		return ""
	}
	heading := cleanInlineText(s.Text())

	// 标题与内容同节点：「岗位职责：负责xxx」。
	if rest := stripHeadingPrefix(heading); rest != "" {
		return rest
	}

	parts := make([]string, 0, 4)
	for next := s.Next(); next.Length() > 0; next = next.Next() {
		if len(next.Nodes) == 0 {
			break
		}
		tag := strings.ToLower(next.Nodes[0].Data)
		if isHeadingTag(tag) {
			break
		}
		text := cleanInlineText(next.Text())
		if text == "" {
			continue
		}
		if isSectionHeadingText(text) {
			break
		}
		parts = append(parts, htmlToText(nodeInnerHTML(next)))
	}
	if len(parts) > 0 {
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}

	// 兄弟节点没有内容时取父容器内容（例如 <p><strong>岗位职责</strong></p> 之后的内容）。
	if parent := s.Parent(); parent.Length() > 0 {
		full := htmlToText(nodeInnerHTML(parent))
		full = strings.Replace(full, heading, "", 1)
		return strings.TrimSpace(full)
	}
	return ""
}

// stripHeadingPrefix 去掉「岗位职责：」这类前缀，返回同行剩余内容。
func stripHeadingPrefix(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	for _, kw := range sectionHeadingKeywords {
		if strings.HasPrefix(normalizeKey(trimmed), normalizeKey(kw)) {
			// 用原始文本定位冒号，避免归一化后下标错位。
			if idx := strings.IndexAny(trimmed, "：:"); idx >= 0 {
				return strings.TrimSpace(strings.TrimLeft(trimmed[idx+1:], " \t-—·"))
			}
			return ""
		}
	}
	return ""
}

// isSectionHeadingText 判断一段短文本是否开启了新章节。
func isSectionHeadingText(text string) bool {
	if utf8.RuneCountInString(text) > 16 {
		return false
	}
	return headingMatches(text, sectionHeadingKeywords)
}

// isHeadingTag 判断标签是否为标题标签。
func isHeadingTag(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	default:
		return false
	}
}

// findLabeledValue 在「标签：值」结构中取最短的值。
//
// 取最短值是刻意的：外层容器（例如整个信息卡片）也包含标签，但它会把多个字段
// 混在一起；越靠近标签的节点给出的值越精确。
func findLabeledValue(parsed *goquery.Document, keywords []string) string {
	if parsed == nil || len(keywords) == 0 {
		return ""
	}
	best := ""
	examined := 0
	parsed.Find("li,p,div,span,td,th,dd,dt,strong,b,h3,h4,h5,label").Each(func(_ int, s *goquery.Selection) {
		examined++
		if examined > 4000 {
			return
		}
		text := cleanInlineText(s.Text())
		if text == "" || utf8.RuneCountInString(text) > 60 {
			return
		}
		label := matchedLabel(text, keywords)
		if label == "" {
			return
		}
		for _, candidate := range []string{
			strings.TrimLeft(strings.TrimPrefix(text, label), " \t:：-—·"),
			cleanInlineText(s.Next().Text()),
			parentValue(s, text),
		} {
			candidate = cleanInlineText(candidate)
			if candidate == "" || utf8.RuneCountInString(candidate) > 60 {
				continue
			}
			if best == "" || utf8.RuneCountInString(candidate) < utf8.RuneCountInString(best) {
				best = candidate
			}
			break
		}
	})
	return best
}

// matchedLabel 返回文本开头命中的标签原文；未命中返回空串。
//
// 只接受靠前的匹配（24 字节以内），避免把正文段落里的词当成标签。
func matchedLabel(text string, keywords []string) string {
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		idx := strings.Index(lower, strings.ToLower(kw))
		if idx < 0 || idx > 24 {
			continue
		}
		return text[idx : idx+len(kw)]
	}
	return ""
}

// parentValue 从父容器文本里去掉标签，得到「标签与值分处不同节点」时的值。
func parentValue(s *goquery.Selection, label string) string {
	parent := s.Parent()
	if parent.Length() == 0 {
		return ""
	}
	text := cleanInlineText(parent.Text())
	text = strings.Replace(text, label, "", 1)
	return strings.TrimLeft(text, " \t:：-—·")
}

// findApplyURL 在页面中寻找「立即申请 / 投递简历」入口。
func findApplyURL(parsed *goquery.Document, base *url.URL) string {
	if parsed == nil {
		return ""
	}
	keywords := []string{"立即申请", "申请职位", "申请该职位", "投递简历", "简历投递", "在线申请", "马上申请", "我要应聘", "apply now", "apply", "投递"}
	found := ""
	parsed.Find("a[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		href, _ := s.Attr("href")
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(href)), "mailto:") {
			return true
		}
		text := normalizeKey(linkText(s))
		if text == "" {
			return true
		}
		for _, kw := range keywords {
			if !strings.Contains(text, normalizeKey(kw)) {
				continue
			}
			if abs, ok := absoluteHTTPURL(base, href); ok {
				found = abs
				return false
			}
		}
		return true
	})
	return found
}

// —— 统一后处理 ——

// postProcess 对三种抽取器产出的字段做统一规整。
func postProcess(job *ExtractedJob, pageText string) {
	if job == nil {
		return
	}
	job.Title = cleanTitle(job.Title)
	job.SourceCompanyName = cleanInlineText(job.SourceCompanyName)

	// 薪资：解析失败时保留原文，交由人工审核判断。
	if job.SalaryMin == nil && job.SalaryMax == nil {
		minV, maxV, months := parseSalaryText(job.SalaryText)
		job.SalaryMin, job.SalaryMax = minV, maxV
		if job.SalaryMonths == nil {
			job.SalaryMonths = months
		}
	} else if job.SalaryMonths == nil {
		if _, _, months := parseSalaryText(job.SalaryText); months != nil {
			job.SalaryMonths = months
		}
	}

	// 地点：把「北京市朝阳区望京SOHO」这类文本拆成省市区与详细地址。
	if job.LocationText == "" {
		job.LocationText = cleanInlineText(strings.Join([]string{job.Province, job.City, job.District, job.Address}, ""))
	}
	if job.City == "" || job.Province == "" || job.District == "" {
		country, province, city, district, address := parseLocationText(job.LocationText)
		if job.Country == "" {
			job.Country = country
		}
		if job.Province == "" {
			job.Province = province
		}
		if job.City == "" {
			job.City = city
		}
		if job.District == "" {
			job.District = district
		}
		if job.Address == "" {
			job.Address = address
		}
	}
	if job.WorkMode == "" && containsAnyFold(job.LocationText, []string{"远程", "remote"}) {
		job.WorkMode = "远程"
	}

	// 任职要求列表化——本系统最核心的检索字段之一。
	if job.Requirements != "" && len(job.RequirementsList) == 0 {
		job.RequirementsList = splitRequirementItems(job.Requirements)
	}

	// 联系方式与投递方式的兜底。
	if job.ContactEmail == "" {
		job.ContactEmail = findEmail(pageText)
	}
	if job.ContactPhone == "" {
		job.ContactPhone = findPhone(pageText)
	}
	if job.ContactWechat == "" {
		job.ContactWechat = findWechat(pageText)
	}
	switch {
	case job.ApplyURL != "":
		job.ApplyMethod = firstNonEmpty(job.ApplyMethod, "在线投递")
	case job.ContactEmail != "":
		job.ApplyMethod = firstNonEmpty(job.ApplyMethod, "邮箱投递")
	}

	if len(job.Tags) == 0 {
		job.Tags = splitTagText(job.Benefits)
	}
	if job.Category == "" {
		job.Category = InferCategory(job.Title, job.Description+job.Responsibilities+job.Requirements)
	}
	if job.EmploymentType == "" {
		job.EmploymentType = InferEmploymentType(job.Title + " " + job.Description + " " + job.Requirements)
	}
}

// —— 薪资解析 ——

const (
	// daysPerMonth 是法定月平均计薪天数，用于把日薪/时薪折算成月薪。
	daysPerMonth = 21.75
	hoursPerDay  = 8.0
)

var (
	salaryMonthsRe = regexp.MustCompile(`(\d{2})\s*薪`)
	// 单位在区间两端都可能出现：「25-40K」与「25k-40k」都要支持。
	salaryRangeRe  = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(万|w|W|k|K|千|元)?\s*(?:-|~|—|–|至|到)\s*(\d+(?:\.\d+)?)\s*(万|w|W|k|K|千|元)?`)
	salarySingleRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(万|w|W|k|K|千|元)`)
)

// parseSalaryText 解析薪资文本，返回月薪下限/上限（元）与薪资月数。
//
// 支持「25-40K」「25~40K·14薪」「20-35万/年」「1.5-2.5万」「8000-15000元/月」
// 「日薪 300-500」「面议」等写法；解析不出来时返回 nil，由调用方保留原文。
// 年薪按 12 个月折算，日薪按 21.75 天/月、时薪按 8 小时/天折算。
func parseSalaryText(raw string) (minV, maxV, months *int) {
	text := cleanInlineText(raw)
	if text == "" {
		return nil, nil, nil
	}
	if containsAnyFold(text, []string{"面议", "面谈", "保密", "negotiable"}) {
		return nil, nil, nil
	}

	months = parseSalaryMonths(text)
	work := salaryMonthsRe.ReplaceAllString(text, " ")
	period := salaryPeriod(work)

	var (
		lo, hi   float64
		unit     string
		hasRange bool
	)
	if m := salaryRangeRe.FindStringSubmatch(work); m != nil {
		lo, _ = strconv.ParseFloat(m[1], 64)
		hi, _ = strconv.ParseFloat(m[3], 64)
		// 单位可能只写在任意一端，取第一个非空值。
		unit = m[4]
		if unit == "" {
			unit = m[2]
		}
		hasRange = true
	} else if m := salarySingleRe.FindStringSubmatch(work); m != nil {
		lo, _ = strconv.ParseFloat(m[1], 64)
		hi = lo
		unit = m[2]
	} else {
		return nil, nil, months
	}

	multiplier := salaryUnitMultiplier(unit)
	lo *= multiplier
	hi *= multiplier

	switch period {
	case "day":
		lo *= daysPerMonth
		hi *= daysPerMonth
	case "hour":
		lo *= daysPerMonth * hoursPerDay
		hi *= daysPerMonth * hoursPerDay
	case "week":
		lo *= 52.0 / 12.0
		hi *= 52.0 / 12.0
	case "year":
		lo /= 12
		hi /= 12
	}

	lo = math.Round(lo)
	hi = math.Round(hi)
	if lo > hi {
		lo, hi = hi, lo
	}
	// 合理性校验：过小或过大的数值多半是误匹配（例如把「3年经验」当薪资）。
	if !plausibleMonthlySalary(lo) || !plausibleMonthlySalary(hi) {
		return nil, nil, months
	}

	minInt, maxInt := int(lo), int(hi)
	minV, maxV = &minInt, &maxInt
	if !hasRange {
		maxV = minV
	}
	return minV, maxV, months
}

// parseSalaryMonths 解析「14薪」这类发放月数。
func parseSalaryMonths(text string) *int {
	m := salaryMonthsRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 12 || n > 24 {
		return nil
	}
	return &n
}

// salaryPeriod 判断薪资的计薪周期。
func salaryPeriod(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "日薪"), strings.Contains(lower, "/天"), strings.Contains(lower, "元/天"),
		strings.Contains(lower, "每天"), strings.Contains(lower, "per day"), strings.Contains(lower, "daily"):
		return "day"
	case strings.Contains(lower, "时薪"), strings.Contains(lower, "/小时"), strings.Contains(lower, "元/时"),
		strings.Contains(lower, "per hour"), strings.Contains(lower, "hourly"):
		return "hour"
	case strings.Contains(lower, "/周"), strings.Contains(lower, "元/周"), strings.Contains(lower, "per week"):
		return "week"
	case strings.Contains(lower, "年薪"), strings.Contains(lower, "/年"), strings.Contains(lower, "年包"),
		strings.Contains(lower, "元/年"), strings.Contains(lower, "per year"), strings.Contains(lower, "annum"):
		return "year"
	default:
		return "month"
	}
}

// salaryUnitMultiplier 返回薪资单位相对「元」的倍率。
func salaryUnitMultiplier(unit string) float64 {
	switch strings.ToLower(unit) {
	case "万", "w":
		return 10000
	case "k", "千":
		return 1000
	default:
		return 1
	}
}

// plausibleMonthlySalary 判断折算后的月薪是否落在合理区间。
func plausibleMonthlySalary(v float64) bool {
	return v >= 1000 && v <= 5_000_000
}

// —— 日期解析 ——

var (
	// 相对时间：2天前、3小时前、1个月前。
	relativeTimeRe = regexp.MustCompile(`(\d+)\s*(秒|分钟|分|小时|天|日|周|个?月|年)\s*前`)
	// 13 位毫秒时间戳 / 10 位秒时间戳。
	timestampRe = regexp.MustCompile(`\b(1[0-9]{9}|1[0-9]{12})\b`)
	// 完整日期（可带时间）。
	fullDateRe = regexp.MustCompile(`(\d{4})\s*[-/.年]\s*(\d{1,2})\s*[-/.月]\s*(\d{1,2})\s*日?(?:\s*[T ]\s*(\d{1,2})\s*[:：]\s*(\d{2})(?:\s*[:：]\s*(\d{2}))?)?`)
	// 无年份日期：5月1日。
	shortDateRe = regexp.MustCompile(`(\d{1,2})\s*月\s*(\d{1,2})\s*日`)
	// 发布时间/更新时间标签。
	publishDateRe = regexp.MustCompile(`(?i)(?:发布(?:时间|日期)|发表于|posted|published)`)
	updateDateRe  = regexp.MustCompile(`(?i)(?:更新(?:时间|日期)|最后更新|最新更新|updated|modified)`)
)

// parseDateText 从文本中解析日期时间，取不到返回 nil。
//
// 支持绝对日期（2024-05-01 / 2024/5/1 / 2024年5月1日 / 2024.05.01 / 带时间）、
// 只有月日（补当年）、相对时间（2天前/3小时前/昨天/今天）与 10/13 位时间戳。
// 结果统一使用 time.Local。
func parseDateText(raw string) *time.Time {
	text := cleanInlineText(raw)
	if text == "" {
		return nil
	}

	if t := parseRelativeTime(text); t != nil {
		return t
	}

	for _, m := range timestampRe.FindAllString(text, -1) {
		if t := parseTimestamp(m); t != nil {
			return t
		}
	}

	if m := fullDateRe.FindStringSubmatch(text); m != nil {
		year, _ := strconv.Atoi(m[1])
		month, _ := strconv.Atoi(m[2])
		day, _ := strconv.Atoi(m[3])
		hour, minute, second := 0, 0, 0
		if m[4] != "" {
			hour, _ = strconv.Atoi(m[4])
			minute, _ = strconv.Atoi(m[5])
			if m[6] != "" {
				second, _ = strconv.Atoi(m[6])
			}
		}
		return buildTime(year, month, day, hour, minute, second)
	}

	if m := shortDateRe.FindStringSubmatch(text); m != nil {
		month, _ := strconv.Atoi(m[1])
		day, _ := strconv.Atoi(m[2])
		return buildTime(time.Now().In(time.Local).Year(), month, day, 0, 0, 0)
	}
	return nil
}

// parseRelativeTime 解析「N天前」「昨天」「今天」等相对时间。
func parseRelativeTime(text string) *time.Time {
	now := time.Now().In(time.Local)

	if m := relativeTimeRe.FindStringSubmatch(text); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 0 {
			return nil
		}
		switch m[2] {
		case "秒":
			t := now.Add(-time.Duration(n) * time.Second)
			return &t
		case "分钟", "分":
			t := now.Add(-time.Duration(n) * time.Minute)
			return &t
		case "小时":
			t := now.Add(-time.Duration(n) * time.Hour)
			return &t
		case "天", "日":
			t := now.AddDate(0, 0, -n)
			return &t
		case "周":
			t := now.AddDate(0, 0, -7*n)
			return &t
		case "月", "个月":
			t := now.AddDate(0, -n, 0)
			return &t
		case "年":
			t := now.AddDate(-n, 0, 0)
			return &t
		}
	}

	switch {
	case strings.Contains(text, "刚刚"), strings.Contains(text, "刚才"), strings.Contains(text, "今天"), strings.Contains(text, "今日"):
		t := now
		return &t
	case strings.Contains(text, "昨天"), strings.Contains(text, "昨日"):
		t := now.AddDate(0, 0, -1)
		return &t
	case strings.Contains(text, "前天"):
		t := now.AddDate(0, 0, -2)
		return &t
	}
	return nil
}

// parseTimestamp 解析 10 位（秒）或 13 位（毫秒）时间戳。
func parseTimestamp(digits string) *time.Time {
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return nil
	}
	switch len(digits) {
	case 13:
		n /= 1000
	case 10:
	default:
		return nil
	}
	t := time.Unix(n, 0).In(time.Local)
	if t.Year() < 2000 || t.Year() > 2100 {
		return nil
	}
	return &t
}

// buildTime 构造本地时间并校验日期真实存在（拒绝 2024-02-31）。
func buildTime(year, month, day, hour, minute, second int) *time.Time {
	if year < 1990 || year > 2100 || month < 1 || month > 12 || day < 1 || day > 31 {
		return nil
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 61 {
		hour, minute, second = 0, 0, 0
	}
	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return nil
	}
	return &t
}

// dateAfterLabel 在标签词之后的一小段文本里解析日期。
func dateAfterLabel(text string, re *regexp.Regexp) *time.Time {
	if text == "" || re == nil {
		return nil
	}
	loc := re.FindStringIndex(text)
	if loc == nil {
		return nil
	}
	rest := []rune(text[loc[1]:])
	if len(rest) > 40 {
		rest = rest[:40]
	}
	return parseDateText(string(rest))
}

// firstTime 返回第一个非空时间。
func firstTime(times ...*time.Time) *time.Time {
	for _, t := range times {
		if t != nil {
			return t
		}
	}
	return nil
}

// —— 地点解析 ——

var (
	locProvinceRe = regexp.MustCompile(`([\p{Han}]{2,8}?)(省|自治区|特别行政区)`)
	locCityRe     = regexp.MustCompile(`([\p{Han}]{2,8}?)(市|地区|自治州|盟)`)
	locDistrictRe = regexp.MustCompile(`([\p{Han}]{2,10}?)(新区|区|县|旗)`)
	locSplitRe    = regexp.MustCompile(`[·•∙・\-—–~～/|\\,;，；、\s]+`)
)

// locMunicipalities 是四个直辖市：其 Province 与 City 都填市名。
var locMunicipalities = []string{"北京", "上海", "天津", "重庆"}

// locProvinceNames 是常见省级行政区（用于「浙江杭州」这类省略「省」的写法）。
var locProvinceNames = []string{
	"河北", "山西", "辽宁", "吉林", "黑龙江", "江苏", "浙江", "安徽", "福建", "江西",
	"山东", "河南", "湖北", "湖南", "广东", "海南", "四川", "贵州", "云南", "陕西",
	"甘肃", "青海", "台湾", "内蒙古", "广西", "西藏", "宁夏", "新疆", "香港", "澳门",
}

// locCityNames 是常见城市（用于「深圳-南山区」这类省略「市」的写法）。
var locCityNames = []string{
	"广州", "深圳", "杭州", "南京", "苏州", "成都", "武汉", "西安", "长沙", "郑州",
	"青岛", "大连", "宁波", "厦门", "福州", "济南", "合肥", "昆明", "南昌", "贵阳",
	"南宁", "太原", "石家庄", "沈阳", "哈尔滨", "长春", "兰州", "乌鲁木齐", "呼和浩特",
	"银川", "西宁", "拉萨", "海口", "三亚", "无锡", "佛山", "东莞", "珠海", "中山",
	"温州", "常州", "徐州", "烟台", "泉州", "唐山", "洛阳", "嘉兴", "南通", "绍兴",
	"惠州", "潍坊", "保定", "台州", "金华", "泰州", "扬州", "镇江", "芜湖", "株洲",
}

// parseLocationText 把地点文本拆成国家/省/市/区与详细地址。
//
// 例如「北京市朝阳区望京SOHO」→ 省=北京市、市=北京市、区=朝阳区、地址=望京SOHO；
// 也支持「上海·浦东新区」「深圳-南山区」「广东省深圳市南山区科技园」等写法。
func parseLocationText(raw string) (country, province, city, district, address string) {
	text := cleanInlineText(raw)
	if text == "" {
		return "", "", "", "", ""
	}
	// 纯远程/不限类描述不是地址。
	if containsAnyFold(text, []string{"远程", "不限", "全国", "面议", "多地"}) &&
		!strings.ContainsAny(text, "省市区县路号") {
		return "", "", "", "", ""
	}

	work := text
	switch {
	case strings.Contains(text, "中国") || strings.Contains(strings.ToLower(text), "china"):
		country = "中国"
		work = strings.NewReplacer("中国", " ", "China", " ", "china", " ").Replace(work)
	case strings.Contains(text, "新加坡") || strings.Contains(strings.ToLower(text), "singapore"):
		country = "新加坡"
	}

	// 直辖市
	for _, name := range locMunicipalities {
		if !strings.Contains(work, name) {
			continue
		}
		province = name + "市"
		city = name + "市"
		work = strings.Replace(work, name+"市", " ", 1)
		work = strings.Replace(work, name, " ", 1)
		break
	}

	if province == "" {
		if m := locProvinceRe.FindStringSubmatch(work); m != nil {
			province = m[1] + m[2]
			work = strings.Replace(work, m[0], " ", 1)
		} else if name := matchLeadingName(work, locProvinceNames); name != "" {
			province = name + "省"
			work = strings.Replace(work, name, " ", 1)
		}
	}
	if city == "" {
		if m := locCityRe.FindStringSubmatch(work); m != nil {
			city = m[1] + m[2]
			work = strings.Replace(work, m[0], " ", 1)
		} else if name := matchLeadingName(work, locCityNames); name != "" {
			city = name + "市"
			work = strings.Replace(work, name, " ", 1)
		}
	}

	// 「深圳-南山区」这类用分隔符连接的写法：第一段城市名，后面是区与地址。
	parts := splitLocationParts(work)
	if city == "" && len(parts) >= 2 && isCityLikeToken(parts[0]) {
		city = parts[0] + "市"
		work = strings.Replace(work, parts[0], " ", 1)
	}

	if m := locDistrictRe.FindStringSubmatch(work); m != nil {
		district = m[1] + m[2]
		work = strings.Replace(work, m[0], " ", 1)
	}

	address = cleanInlineText(locSplitRe.ReplaceAllString(work, " "))
	address = strings.Trim(address, " ,，、;；:：")
	if address != "" && !containsHanOrDigit(address) {
		address = ""
	}
	return country, province, city, district, address
}

// splitLocationParts 按分隔符切分地点文本。
func splitLocationParts(text string) []string {
	raw := locSplitRe.Split(text, -1)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// matchLeadingName 判断文本（忽略前导分隔符）是否以某个地名开头。
func matchLeadingName(text string, names []string) string {
	trimmed := strings.TrimLeft(text, " \t·•∙・-—–,，、;；")
	for _, name := range names {
		if strings.HasPrefix(trimmed, name) {
			return name
		}
	}
	return ""
}

// isCityLikeToken 判断一个片段是否像城市名（避免把「南山区」当成城市）。
func isCityLikeToken(token string) bool {
	n := utf8.RuneCountInString(token)
	if n < 2 || n > 6 {
		return false
	}
	for _, bad := range []string{"区", "县", "路", "号", "室", "楼", "大厦", "街", "道", "镇", "村", "园", "院", "省"} {
		if strings.Contains(token, bad) {
			return false
		}
	}
	return true
}

// containsHanOrDigit 判断文本是否包含汉字或数字（过滤纯符号残留）。
func containsHanOrDigit(text string) bool {
	for _, r := range text {
		if unicode.IsDigit(r) || unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// —— 文本清洗 ——

// blockTags 是需要换行分隔的块级标签。
var blockTags = map[string]bool{
	"address": true, "article": true, "blockquote": true, "br": true, "dd": true, "div": true,
	"dl": true, "dt": true, "fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hr": true, "li": true, "main": true, "nav": true, "ol": true, "p": true,
	"pre": true, "section": true, "table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "tr": true, "ul": true,
}

// skipTags 是不参与文本抽取的标签。
var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true, "head": true,
	"title": true, "meta": true, "link": true, "iframe": true, "svg": true,
}

// htmlToText 把 HTML 转成纯文本，保留段落与列表的换行语义。
//
// <br>、</p>、</li> 等块级边界会转成换行；连续空白与空行会被压缩；
// HTML 实体（含 &nbsp;）会被解码。
func htmlToText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "<") {
		return normalizeMultiline(xhtml.UnescapeString(raw))
	}
	root, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil || root == nil {
		return normalizeMultiline(xhtml.UnescapeString(stripTagsFallback(raw)))
	}
	var sb strings.Builder
	writeNodeText(&sb, root)
	return normalizeMultiline(sb.String())
}

// writeNodeText 递归写出节点文本，块级标签两侧补换行。
func writeNodeText(sb *strings.Builder, n *xhtml.Node) {
	switch n.Type {
	case xhtml.TextNode:
		sb.WriteString(n.Data)
		return
	case xhtml.ElementNode:
		tag := strings.ToLower(n.Data)
		if skipTags[tag] {
			return
		}
		if tag == "br" {
			sb.WriteString("\n")
			return
		}
		block := blockTags[tag]
		if block {
			sb.WriteString("\n")
		}
		if tag == "li" {
			sb.WriteString("• ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeNodeText(sb, c)
		}
		if block {
			sb.WriteString("\n")
		}
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeNodeText(sb, c)
	}
}

// stripTagsFallback 是解析失败时的粗粒度去标签。
func stripTagsFallback(raw string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range raw {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			sb.WriteRune('\n')
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// normalizeMultiline 解码实体、压缩连续空白与空行。
func normalizeMultiline(s string) string {
	s = xhtml.UnescapeString(s)
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\u00a0", " ", "\u3000", " ").Replace(s)
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" || line == "•" {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// cleanInlineText 把任意文本压成单行：解码实体、去掉零宽字符、压缩空白。
func cleanInlineText(s string) string {
	if s == "" {
		return ""
	}
	s = xhtml.UnescapeString(s)
	s = strings.NewReplacer("\u00a0", " ", "\u3000", " ", "\u200b", "", "\ufeff", "").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// normalizeKey 生成用于关键词包含匹配的归一化文本（小写、去空白）。
func normalizeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// normalizeKeys 批量归一化关键词，并丢弃空项。
func normalizeKeys(list []string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if key := normalizeKey(item); key != "" {
			out = append(out, key)
		}
	}
	return out
}

// nodeInnerHTML 返回节点的内部 HTML。
func nodeInnerHTML(sel *goquery.Selection) string {
	if sel == nil || sel.Length() == 0 {
		return ""
	}
	html, err := sel.Html()
	if err != nil {
		return ""
	}
	return html
}

// metaContent 读取 meta 标签的 content。
func metaContent(parsed *goquery.Document, selector string) string {
	if parsed == nil {
		return ""
	}
	node := parsed.Find(selector).First()
	if node.Length() == 0 {
		return ""
	}
	return cleanInlineText(attrValue(node, "content"))
}

// pageTextOf 取整页纯文本，用于联系方式等全文兜底抽取。
func pageTextOf(parsed *goquery.Document, rawHTML string) string {
	if parsed != nil {
		if body := parsed.Find("body").First(); body.Length() > 0 {
			if text := htmlToText(nodeInnerHTML(body)); text != "" {
				return text
			}
		}
	}
	return htmlToText(rawHTML)
}

// —— 标题规整 ——

// titleSeparatorRe 匹配标题中用于分隔「岗位名 / 站点名」的符号。
var titleSeparatorRe = regexp.MustCompile(`\s*[-–—_|｜]+\s*`)

// cleanTitle 清洗标题：去掉装饰符号与公司/站点后缀。
func cleanTitle(raw string) string {
	title := cleanInlineText(raw)
	if title == "" {
		return ""
	}
	title = strings.Trim(title, "【】[]()（）<>《》-–—_|｜· \t")

	parts := titleSeparatorRe.Split(title, -1)
	if len(parts) < 2 {
		return title
	}
	head := strings.TrimSpace(parts[0])
	if utf8.RuneCountInString(head) < 2 {
		return title
	}
	for _, tail := range parts[1:] {
		if tailIsSiteLike(strings.TrimSpace(tail), head) {
			return head
		}
	}
	return title
}

// tailIsSiteLike 判断被分隔符切出的后段是否像公司名/站点名。
func tailIsSiteLike(tail, head string) bool {
	if tail == "" {
		return false
	}
	lower := strings.ToLower(tail)
	for _, kw := range []string{
		"公司", "集团", "有限", "股份", "官网", "招聘", "人才", "人力", "银行", "证券", "保险",
		"大学", "学院", "研究院", "事务所", "企业", "科技", "co.,ltd", "inc.", "corp", "llc", "ltd",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// 中文岗位名后跟纯英文/数字后缀，通常是站点名而不是岗位方向。
	if containsHan(head) && isASCIIOnly(tail) && utf8.RuneCountInString(tail) <= 24 {
		return true
	}
	return false
}

// genericTitleWords 是站点通用页标题。
//
// 刻意不收录「验证」「首页」这类词的片段形式，避免误杀「IC验证工程师」等真实岗位；
// 判定时按整体相等或前后缀（且总长度接近）来判断。
var genericTitleWords = []string{
	"登录", "登陆", "注册", "首页", "404", "403", "500", "页面不存在", "无法访问",
	"出错了", "错误", "无权限", "请稍候", "加载中", "跳转中", "安全验证", "验证中",
	"index", "login", "signin", "home", "notfound", "error",
}

// isGenericTitle 判断标题是否像站点通用页而非岗位详情页。
func isGenericTitle(title string) bool {
	key := normalizeKey(title)
	if key == "" {
		return true
	}
	length := utf8.RuneCountInString(key)
	for _, word := range genericTitleWords {
		w := normalizeKey(word)
		if w == "" {
			continue
		}
		if key == w {
			return true
		}
		if (strings.HasPrefix(key, w) || strings.HasSuffix(key, w)) && length <= utf8.RuneCountInString(w)+8 {
			return true
		}
	}
	return false
}

//—— 联系方式抽取 ——

var (
	emailRe    = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9\-]+(?:\.[A-Za-z0-9\-]+)+`)
	mobileRe   = regexp.MustCompile(`(?:^|[^0-9])(1[3-9]\d{9})(?:[^0-9]|$)`)
	landlineRe = regexp.MustCompile(`(?:^|[^0-9])(0\d{2,3}[- ]?\d{7,8})(?:[^0-9]|$)`)
	wechatRe   = regexp.MustCompile(`(?i)(?:微信|微信号|wechat|weixin)\s*[:：]?\s*([A-Za-z0-9_\-]{5,24})`)
)

// 招聘相关邮箱前缀：命中时优先作为投递邮箱。
var contactEmailHints = []string{"hr", "job", "jobs", "recruit", "zhaopin", "talent", "resume", "cv", "contact", "career"}

// findEmail 抽取邮箱；优先返回招聘相关邮箱。
func findEmail(text string) string {
	matches := emailRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return ""
	}
	first := ""
	for _, raw := range matches {
		mail := strings.Trim(raw, ".,;:、，。；")
		if mail == "" || isAssetLikeEmail(mail) {
			continue
		}
		if first == "" {
			first = mail
		}
		lower := strings.ToLower(mail)
		for _, hint := range contactEmailHints {
			if strings.HasPrefix(lower, hint) || strings.Contains(lower, hint) {
				return mail
			}
		}
	}
	return first
}

// isAssetLikeEmail 过滤形如 sprite@2x.png 的误匹配。
func isAssetLikeEmail(mail string) bool {
	lower := strings.ToLower(mail)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".css", ".js", ".json", ".ico"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// findPhone 抽取手机号或座机号（优先手机号）。
func findPhone(text string) string {
	if m := mobileRe.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	if m := landlineRe.FindStringSubmatch(text); m != nil {
		return strings.ReplaceAll(m[1], " ", "")
	}
	return ""
}

// findWechat 抽取微信号。
func findWechat(text string) string {
	if m := wechatRe.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

//—— 列表化与标签 ——

// requirementSplitRe 匹配任职要求条目之间的分隔符（换行由调用方处理）。
var requirementSplitRe = regexp.MustCompile(`[；;•·●○▪◦◆■□※]|、|(?:^|\s)\d{1,2}\s*[).、）]`)

// requirementBulletRe 匹配条目开头的编号/项目符号。
var requirementBulletRe = regexp.MustCompile(`^\s*(?:[-*•·●○▪◦◆■□※>»]+|\(?\d{1,2}[).、）]|[一二三四五六七八九十]{1,2}[、).）])\s*`)

// splitRequirementItems 把任职要求文本切成条目列表。
func splitRequirementItems(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	out := make([]string, 0, 16)
	seen := make(map[string]bool, 16)
	for _, line := range strings.Split(text, "\n") {
		for _, piece := range requirementSplitRe.Split(line, -1) {
			item := cleanRequirementItem(piece)
			if item == "" || utf8.RuneCountInString(item) < 2 {
				continue
			}
			if !containsHanOrDigit(item) {
				continue
			}
			if seen[item] {
				continue
			}
			seen[item] = true
			out = append(out, item)
			if len(out) >= 60 {
				return out
			}
		}
	}
	return out
}

// cleanRequirementItem 去掉条目开头的项目符号与编号。
func cleanRequirementItem(raw string) string {
	item := cleanInlineText(raw)
	if item == "" {
		return ""
	}
	item = requirementBulletRe.ReplaceAllString(item, "")
	item = strings.Trim(item, " \t-—·:：,，。;；")
	return item
}

// tagSplitRe 匹配标签之间的分隔符。
var tagSplitRe = regexp.MustCompile(`[,，、;；|/]|\s{2,}`)

// splitTagText 把「五险一金、弹性工作」这类文本切成标签。
func splitTagText(text string) []string {
	text = cleanInlineText(text)
	if text == "" {
		return nil
	}
	out := make([]string, 0, 8)
	seen := make(map[string]bool, 8)
	for _, piece := range tagSplitRe.Split(text, -1) {
		tag := strings.TrimSpace(strings.TrimLeft(piece, "•·-—*"))
		if tag == "" || utf8.RuneCountInString(tag) > 24 {
			continue
		}
		if !containsHanOrDigit(tag) || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
		if len(out) >= 12 {
			break
		}
	}
	return out
}

// splitDescriptionSections 从职位描述文本里尝试切出岗位职责与任职要求。
//
// 结构化数据里的 description 往往是整段文本，这里按小标题做一次尽力而为的切分；
// 切不出来时保持空值，由 DOM 层或人工补全。
func splitDescriptionSections(text string) (responsibilities, requirements string) {
	if strings.TrimSpace(text) == "" {
		return "", ""
	}
	lines := strings.Split(text, "\n")
	var (
		current  string
		respBuf  []string
		reqBuf   []string
		otherBuf []string
	)
	flush := func() {
		switch current {
		case "resp":
			respBuf = append(respBuf, otherBuf...)
		case "req":
			reqBuf = append(reqBuf, otherBuf...)
		}
		otherBuf = otherBuf[:0]
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		short := utf8.RuneCountInString(trimmed) <= 20
		switch {
		case short && headingMatches(trimmed, responsibilityHeadings):
			flush()
			current = "resp"
			continue
		case short && headingMatches(trimmed, requirementHeadings):
			flush()
			current = "req"
			continue
		case short && headingMatches(trimmed, summaryHeadings), short && headingMatches(trimmed, benefitHeadings):
			flush()
			current = ""
			continue
		}
		otherBuf = append(otherBuf, trimmed)
	}
	flush()
	return strings.Join(respBuf, "\n"), strings.Join(reqBuf, "\n")
}

//—— 通用小工具 ——

// asMap 把任意值转为 map（不是对象时返回 nil）。
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// listValue 把单对象或对象数组统一成切片。
func listValue(v any) []map[string]any {
	switch node := v.(type) {
	case map[string]any:
		return []map[string]any{node}
	case []any:
		out := make([]map[string]any, 0, len(node))
		for _, item := range node {
			if m := asMap(item); m != nil {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

// stringValue 把 JSON 值转成可读字符串。
func stringValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if s := stringValue(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "、")
	case map[string]any:
		for _, key := range []string{"name", "value", "credentialCategory", "@value", "description", "text"} {
			if s := stringValue(x[key]); s != "" {
				return s
			}
		}
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if s := stringValue(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "、")
	default:
		return ""
	}
}

// numberValue 把 JSON 值转成数字。
func numberValue(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// identifierString 解析 schema.org 的 identifier（字符串或 PropertyValue）。
func identifierString(v any) string {
	if m := asMap(v); m != nil {
		return firstNonEmpty(stringValue(m["value"]), stringValue(m["name"]), stringValue(m["@id"]))
	}
	return stringValue(v)
}

// experienceString 解析 experienceRequirements。
func experienceString(v any) string {
	if m := asMap(v); m != nil {
		if months, ok := numberValue(m["monthsOfExperience"]); ok && months > 0 {
			if int(months)%12 == 0 {
				return fmt.Sprintf("%d年经验", int(months)/12)
			}
			return fmt.Sprintf("%d个月经验", int(months))
		}
	}
	return stringValue(v)
}

// mapEmploymentType 把 schema.org 的用工性质映射成中文。
func mapEmploymentType(raw string) string {
	value := strings.ToUpper(cleanInlineText(raw))
	switch {
	case value == "":
		return ""
	case strings.Contains(value, "FULL_TIME"), strings.Contains(value, "FULLTIME"), strings.Contains(value, "FULL-TIME"):
		return "全职"
	case strings.Contains(value, "INTERN"):
		return "实习"
	case strings.Contains(value, "PART_TIME"), strings.Contains(value, "PARTTIME"), strings.Contains(value, "PART-TIME"):
		return "兼职"
	case strings.Contains(value, "CONTRACTOR"), strings.Contains(value, "CONTRACT"):
		return "合同"
	case strings.Contains(value, "TEMPORARY"), strings.Contains(value, "TEMP"):
		return "临时"
	case strings.Contains(value, "VOLUNTEER"):
		return "志愿者"
	case strings.Contains(value, "OTHER"):
		return "其他"
	default:
		// 已经是中文（或站点自定义文案）时原样返回，交由上层归一化。
		return cleanInlineText(raw)
	}
}

// intRe 用于从文本中取出第一个整数。
var intRe = regexp.MustCompile(`\d+`)

// firstInt 从文本中取第一个整数。
func firstInt(text string) (int, bool) {
	m := intRe.FindString(text)
	if m == "" {
		return 0, false
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return 0, false
	}
	return n, true
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// containsAnyFold 判断文本是否包含任一关键词（大小写不敏感）。
func containsAnyFold(text string, keywords []string) bool {
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// containsHan 判断是否包含汉字。
func containsHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// isASCIIOnly 判断是否只包含 ASCII 可见字符。
func isASCIIOnly(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r > unicode.MaxASCII || r < 0x20 {
			return false
		}
	}
	return true
}

// concatStrings 拼接多个字符串切片。
func concatStrings(groups ...[]string) []string {
	total := 0
	for _, g := range groups {
		total += len(g)
	}
	out := make([]string, 0, total)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}
