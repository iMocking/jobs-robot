// Package crawler 实现「官网直招岗位」采集能力。
//
// 采集链路：
//
//	公司官网/招聘入口 → 定位《加入我们》链接 → 列表页发现岗位链接
//	→ 逐个详情页抽取字段 → 白名单过滤与质量打分 → 交由业务层落库并进入审核流程
//
// 抓取礼仪（合规基线）：
//   - 每个请求都携带标准 AI 机器人身份头（见 identity.go）；
//   - 默认遵循 robots.txt，站点禁止即跳过；
//   - 同站点串行 + 固定延时 + 随机抖动，避免对目标站点造成压力；
//   - 站点声明 noai/nogptbot 等元信息时主动放弃抓取。
package crawler

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/jsong/jobs-robot/internal/config"
)

// Document 是一次页面抓取的结果。
type Document struct {
	// URL 是请求地址。
	URL string
	// FinalURL 是跟随重定向后的最终地址。
	FinalURL string
	// StatusCode 是 HTTP 状态码。
	StatusCode int
	// HTML 是页面内容（无头渲染时同样给出渲染后的 HTML）。
	HTML string
	// Header 是响应头。
	Header http.Header
	// FetchedAt 是抓取时间。
	FetchedAt time.Time
	// Elapsed 是本次抓取耗时。
	Elapsed time.Duration
	// Headless 表示该内容由无头浏览器渲染得到。
	Headless bool
	// NoAI 表示页面声明了 noai / nogptbot 等拒绝 AI 抓取的元信息。
	NoAI bool
}

// TextLength 粗略估计 HTML 正文长度（用于判断是否需要无头渲染兜底）。
func (d *Document) TextLength() int {
	if d == nil {
		return 0
	}
	return estimateTextLength(d.HTML)
}

// Fetcher 负责取回页面。
type Fetcher interface {
	// Fetch 抓取指定地址；错误应为 apperr 业务错误。
	Fetch(ctx context.Context, rawURL string) (*Document, error)
	// Close 释放资源。
	Close() error
}

// Renderer 是无头浏览器渲染器抽象，便于在测试中注入假实现。
type Renderer interface {
	// Render 使用无头浏览器打开页面并等待渲染完成。
	Render(ctx context.Context, rawURL string) (*Document, error)
	// Close 释放浏览器进程。
	Close() error
}

// Link 是页面中发现的一个链接。
type Link struct {
	// URL 是绝对地址。
	URL string
	// Text 是链接文本。
	Text string
	// Score 是匹配得分，越高越可能是目标链接。
	Score int
	// Reason 记录命中的规则，便于排查。
	Reason string
}

// SiteRules 是站点自定义抓取规则，对应 Company.crawl_rules（JSON）。
//
// 零值表示完全依赖通用启发式规则；对结构特殊的站点可逐项覆盖。
type SiteRules struct {
	// CareersURL 直接指定《加入我们》页面，跳过自动发现。
	CareersURL string `json:"careers_url,omitempty"`
	// CareersLinkSelector 定位招聘入口链接的 CSS 选择器。
	CareersLinkSelector string `json:"careers_link_selector,omitempty"`
	// CareersLinkTexts 补充的招聘入口链接文案（内置文案之外）。
	CareersLinkTexts []string `json:"careers_link_texts,omitempty"`
	// ListSelector 岗位列表容器的 CSS 选择器。
	ListSelector string `json:"list_selector,omitempty"`
	// ItemSelector 列表中单个岗位条目的 CSS 选择器。
	ItemSelector string `json:"item_selector,omitempty"`
	// LinkSelector 岗位条目内链接的 CSS 选择器。
	LinkSelector string `json:"link_selector,omitempty"`
	// DetailFields 是详情页字段选择器，键为字段名（title/location/...），值为 CSS 选择器。
	DetailFields map[string]string `json:"detail_fields,omitempty"`
	// ForceHeadless 强制使用无头浏览器。
	ForceHeadless bool `json:"force_headless,omitempty"`
	// MaxPages 单站点最多抓取的详情页数量，0 表示使用全局配置。
	MaxPages int `json:"max_pages,omitempty"`
}

// JobLink 是列表页中发现的岗位链接及其摘要信息。
type JobLink struct {
	URL         string
	Title       string
	Category    string
	Location    string
	PublishedAt string
	Department  string
}

// ExtractedJob 是从详情页抽取出的岗位信息（尚未落库）。
//
// 指针字段表示「页面未提供」，与「提供了但为空」区分开。
type ExtractedJob struct {
	SourceURL         string
	SourceCompanyName string
	SourceJobID       string

	Title          string
	JobCode        string
	Category       string
	Industry       string
	EmploymentType string
	Department     string
	Headcount      *int
	Experience     string
	Education      string
	WorkMode       string

	SalaryText   string
	SalaryMin    *int
	SalaryMax    *int
	SalaryMonths *int

	Country      string
	Province     string
	City         string
	District     string
	Address      string
	LocationText string

	Summary          string
	Description      string
	Responsibilities string
	Requirements     string
	RequirementsList []string
	Benefits         string
	Tags             []string

	ContactPerson string
	ContactEmail  string
	ContactPhone  string
	ContactWechat string
	ApplyURL      string
	ApplyMethod   string

	PublishedAt     *time.Time
	DeadlineAt      *time.Time
	SourceUpdatedAt *time.Time

	// Extractor 记录最终生效的抽取器：jsonld / rules / dom / headless。
	Extractor string
	// Confidence 是解析置信度 0~1。
	Confidence float64
	// Raw 保存抽取到的原始结构化数据，便于回溯与二次解析。
	Raw map[string]any
}

// LogEntry 是一条抓取日志，会随运行记录一起落库。
type LogEntry struct {
	Level   string
	Stage   string
	URL     string
	Message string
	Context map[string]any
}

// CompanyInput 是一次公司抓取的输入。
type CompanyInput struct {
	CompanyID     string
	CompanyName   string
	Website       string
	CareersURL    string
	Industry      string
	Rules         *SiteRules
	ForceHeadless bool
	RespectRobots bool
	MaxPages      int
}

// Result 是一次公司抓取的输出。
type Result struct {
	CompanyID     string
	CompanyName   string
	CareersURL    string
	UsedHeadless  bool
	PagesFetched  int
	RobotsAllowed bool
	// JobLinksFound 是招聘页上发现的岗位链接数。
	//
	// 它与 len(Jobs) 是两个不同口径：Jobs 只包含通过白名单过滤的岗位，
	// 而运营更关心「页面上本来有多少个岗位、其中被过滤掉几个」。
	JobLinksFound int
	// Skipped 为真表示未执行抓取（robots 禁止、未找到入口等）。
	Skipped    bool
	SkipReason string
	Jobs       []*ExtractedJob
	Logs       []LogEntry
}

// Options 是构造 Crawler 的参数。
type Options struct {
	Config config.Crawler
	// Robots 为 nil 时按 Config.RespectRobots 自动构造。
	Robots RobotsGate
	// Renderer 为 nil 时按 Config.Headless 自动构造无头渲染器。
	Renderer Renderer
	// Fetcher 为 nil 时自动构造。测试可注入假实现。
	Fetcher Fetcher
	// Scope 是收集范围（工作类型/行业白名单等）。
	Scope Scope
}

// RobotsGate 判定站点许可与建议抓取间隔。
type RobotsGate interface {
	// Allowed 判断 userAgent 是否允许抓取 target。
	Allowed(ctx context.Context, target *url.URL, userAgent string) (bool, error)
	// CrawlDelay 返回站点建议的抓取间隔；未声明时返回 0。
	CrawlDelay(ctx context.Context, target *url.URL, userAgent string) (time.Duration, error)
}
