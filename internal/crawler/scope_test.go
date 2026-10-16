package crawler

import (
	"net/url"
	"strings"
	"testing"

	"github.com/jsong/jobs-robot/internal/config"
)

func testIdentity() config.Identity {
	return config.Identity{
		BotName:      "JobsRobotBot",
		BotVersion:   "1.0",
		ContactEmail: "crawler@jobs-robot.local",
		Homepage:     "https://jobs-robot.local/bot",
		Purpose:      "聚合企业官网直招岗位信息，供求职者检索",
		IsAI:         true,
		Language:     "zh-CN,zh;q=0.9",
	}
}

func TestUserAgentCarriesIdentity(t *testing.T) {
	ua := UserAgent(testIdentity())

	for _, want := range []string{"JobsRobotBot/1.0", "AI-crawler", "crawler@jobs-robot.local", "https://jobs-robot.local/bot"} {
		if !strings.Contains(ua, want) {
			t.Errorf("User-Agent 应包含 %q，实际为 %q", want, ua)
		}
	}
	// 标准格式：名字/版本 (+注释; 注释)
	if !strings.HasPrefix(ua, "JobsRobotBot/1.0 (+") || !strings.HasSuffix(ua, ")") {
		t.Errorf("User-Agent 格式不符合惯例: %q", ua)
	}
}

func TestUserAgentDefaultsWhenMissing(t *testing.T) {
	ua := UserAgent(config.Identity{})
	if !strings.Contains(ua, "JobsRobotBot/1.0") {
		t.Errorf("缺少配置时应回退到默认名称与版本，实际 %q", ua)
	}
	if strings.Contains(ua, "AI-crawler") {
		t.Error("未声明 IsAI 时不应标注 AI-crawler")
	}
}

func TestBuildHeadersDeclaresRobotIdentity(t *testing.T) {
	target, _ := url.Parse("https://example.com/careers")
	h := BuildHeaders(testIdentity(), target)

	checks := map[string]string{
		"User-Agent":             "JobsRobotBot/1.0",
		"From":                   "crawler@jobs-robot.local",
		"X-Robot-Name":           "JobsRobotBot",
		"X-Robot-Version":        "1.0",
		"X-Robot-Contact":        "crawler@jobs-robot.local",
		"X-Robot-AI":             "true",
		"X-Robot-Type":           "ai-crawler",
		"X-Robot-Respect-Robots": "true",
		"X-Robot-Homepage":       "https://jobs-robot.local/bot",
	}
	for key, want := range checks {
		if got := h.Get(key); !strings.Contains(got, want) {
			t.Errorf("请求头 %s 应包含 %q，实际 %q", key, want, got)
		}
	}

	if h.Get("Accept") == "" || h.Get("Accept-Language") == "" {
		t.Error("应携带内容协商相关请求头，保证拿到与浏览器一致的页面")
	}
	if h.Get("Host") != "example.com" {
		t.Errorf("Host 头应为目标主机，实际 %q", h.Get("Host"))
	}
}

func TestHeaderValuePercentEncodesNonASCII(t *testing.T) {
	got := headerValue("聚合 岗位信息")
	if strings.ContainsAny(got, "聚合岗位") {
		t.Errorf("非 ASCII 字符必须被百分号编码，实际 %q", got)
	}
	if !strings.Contains(got, "%") {
		t.Errorf("应出现百分号编码，实际 %q", got)
	}
	if headerValue("") != "" {
		t.Error("空值应返回空串")
	}
	// 编码后的值必须是可见 ASCII，否则部分服务器会直接返回 400。
	for i := 0; i < len(got); i++ {
		if got[i] < 0x20 || got[i] >= 0x7f {
			t.Fatalf("编码结果含不可见字符: %q", got)
		}
	}
}

func TestEstimateTextLength(t *testing.T) {
	if n := estimateTextLength(""); n != 0 {
		t.Errorf("空 HTML 长度应为 0，实际 %d", n)
	}
	html := `<html><head><title>标题</title></head><body><p>正文内容</p></body></html>`
	got := estimateTextLength(html)
	if got == 0 {
		t.Error("应统计出非零正文长度")
	}
	if got >= len(html) {
		t.Errorf("正文长度应小于原始 HTML 长度（标签被剔除），got=%d html=%d", got, len(html))
	}
}

func job(title, category string) *ExtractedJob {
	return &ExtractedJob{
		Title:            title,
		Category:         category,
		Description:      strings.Repeat("岗位介绍内容。", 10),
		Responsibilities: strings.Repeat("职责描述。", 10),
		Requirements:     strings.Repeat("任职要求描述。", 10),
		City:             "北京",
		Address:          "北京市朝阳区望京",
	}
}

func TestScopeMatchExcludeKeyword(t *testing.T) {
	scope := Scope{ExcludeKeywords: []string{"外包", "劳务派遣"}}

	ok, reason := scope.Match(job("后端开发工程师", "技术"), "")
	if !ok {
		t.Fatalf("正常岗位不应被过滤，原因: %s", reason)
	}

	outsourced := job("后端开发工程师", "技术")
	outsourced.Description = "本岗位为外包性质，派驻客户现场"
	ok, reason = scope.Match(outsourced, "")
	if ok {
		t.Error("命中排除关键词的岗位应被过滤")
	}
	if !strings.Contains(reason, "外包") {
		t.Errorf("过滤原因应说明命中了哪个关键词，实际 %q", reason)
	}
}

func TestScopeMatchJobTypes(t *testing.T) {
	scope := Scope{JobTypes: []string{"技术", "产品"}}

	if ok, reason := scope.Match(job("Golang 后端工程师", ""), ""); !ok {
		t.Errorf("应能自动推断出「技术」类别并通过，原因: %s", reason)
	}
	if ok, _ := scope.Match(job("行政前台", ""), ""); ok {
		t.Error("职能类岗位不在白名单内，应被过滤")
	}
}

func TestScopeMatchIndustryUnknownIsLenient(t *testing.T) {
	scope := Scope{Industries: []string{"互联网"}}

	// 行业无法判断时放行，交由人工审核判断，避免误杀大量合规岗位。
	ok, reason := scope.Match(job("后端工程师", "技术"), "")
	if !ok {
		t.Errorf("行业未知时应放行，原因: %s", reason)
	}

	// 能判断出行业且不在白名单时拦截。
	ok, reason = scope.Match(job("后端工程师", "技术"), "房地产")
	if ok {
		t.Error("行业不在白名单时应被过滤")
	}
	if !strings.Contains(reason, "房地产") {
		t.Errorf("过滤原因应包含行业名，实际 %q", reason)
	}

	// 岗位自带的行业优先于公司行业。
	j := job("后端工程师", "技术")
	j.Industry = "互联网"
	if ok, reason := scope.Match(j, "房地产"); !ok {
		t.Errorf("岗位自身行业命中白名单时应放行，原因: %s", reason)
	}
}

func TestScopeMatchKeywordsRequired(t *testing.T) {
	scope := Scope{Keywords: []string{"golang", "kubernetes"}}

	if ok, _ := scope.Match(job("Golang 后端工程师", "技术"), ""); !ok {
		t.Error("命中目标关键词应放行")
	}
	if ok, reason := scope.Match(job("销售代表", "市场"), ""); ok {
		t.Error("未命中任何目标关键词应被过滤")
	} else if !strings.Contains(reason, "关键词") {
		t.Errorf("过滤原因应说明关键词未命中，实际 %q", reason)
	}
}

func TestScopeMatchLocations(t *testing.T) {
	scope := Scope{Locations: []string{"北京", "上海"}}

	if ok, _ := scope.Match(job("后端工程师", "技术"), ""); !ok {
		t.Error("北京岗位应放行")
	}

	remote := job("后端工程师", "技术")
	remote.City = "成都"
	remote.Address = "成都市高新区"
	if ok, reason := scope.Match(remote, ""); ok {
		t.Error("地点不在白名单应被过滤")
	} else if !strings.Contains(reason, "地点") {
		t.Errorf("过滤原因应说明地点不符，实际 %q", reason)
	}
}

func TestInferCategory(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"高级后端开发工程师", "技术"},
		{"算法工程师（NLP）", "技术"},
		{"前端开发", "技术"},
		{"产品经理", "产品"},
		{"UI 设计师", "设计"},
		{"新媒体运营", "运营"},
		{"市场推广专员", "市场"},
		{"人力资源专员", "职能"},
		{"未知岗位名称", ""},
	}
	for _, tc := range cases {
		if got := InferCategory(tc.title, ""); got != tc.want {
			t.Errorf("InferCategory(%q) = %q，期望 %q", tc.title, got, tc.want)
		}
	}
}

func TestInferEmploymentType(t *testing.T) {
	cases := map[string]string{
		"2025 校园招聘启动": "校招",
		"招聘实习生（可转正）":  "实习",
		"社会招聘：资深架构师":  "社招",
		"兼职数据标注":      "兼职",
		"全职岗位":        "全职",
		"没有明确说明":      "",
	}
	for text, want := range cases {
		if got := InferEmploymentType(text); got != want {
			t.Errorf("InferEmploymentType(%q) = %q，期望 %q", text, got, want)
		}
	}
}

func TestScoreJobRewardsCompleteness(t *testing.T) {
	minimal := &ExtractedJob{Title: "工程师"}
	if score := ScoreJob(minimal); score <= 0 {
		t.Errorf("至少有标题的岗位应得到正分，实际 %d", score)
	}

	complete := job("高级后端工程师", "技术")
	complete.Summary = "负责核心交易系统"
	complete.RequirementsList = []string{"精通 Go", "熟悉分布式系统", "有高并发经验"}
	complete.Benefits = "五险一金、弹性工作"
	complete.Tags = []string{"五险一金"}
	complete.ContactEmail = "hr@example.com"
	complete.ContactPhone = "13800000000"
	complete.ApplyURL = "https://example.com/apply"
	complete.EmploymentType = "社招"
	complete.Experience = "3-5 年"
	complete.Education = "本科"
	complete.SalaryText = "30-50K"

	if ScoreJob(complete) <= ScoreJob(minimal) {
		t.Error("信息完整的岗位得分应高于仅有标题的岗位")
	}
	if got := ScoreJob(complete); got > 100 {
		t.Errorf("评分上限应为 100，实际 %d", got)
	}
	if got := ScoreJob(nil); got != 0 {
		t.Errorf("nil 岗位评分应为 0，实际 %d", got)
	}
}
