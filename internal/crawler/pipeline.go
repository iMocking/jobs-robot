package crawler

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// Crawler 编排一次完整的公司抓取：定位招聘入口 → 发现岗位链接 → 抽取字段 → 白名单过滤。
//
// 并发模型：公司级并发由调度层控制；单站点内部的请求速率由 Fetcher 负责
// （固定延时 + 随机抖动 + robots.txt 的 CrawlDelay + 每主机并发上限）。
type Crawler struct {
	cfg      config.Crawler
	robots   RobotsGate
	fetcher  Fetcher
	renderer Renderer

	// scope 可在运行时被替换（管理员调整收集范围后立即生效），
	// 而抓取是多协程并发的，因此用读写锁保护。
	scopeMu sync.RWMutex
	scope   Scope
}

// SetScope 更新收集范围（工作类型/行业白名单等）。
//
// 由调度层在每次抓取前调用，使后台配置改动无需重启服务即可生效。
func (c *Crawler) SetScope(scope Scope) {
	c.scopeMu.Lock()
	c.scope = scope
	c.scopeMu.Unlock()
}

// currentScope 读取当前收集范围。
func (c *Crawler) currentScope() Scope {
	c.scopeMu.RLock()
	defer c.scopeMu.RUnlock()
	return c.scope
}

// New 构造爬虫。
//
// 依赖均可注入，便于测试：Fetcher/Renderer/Robots 为空时按配置自动构造。
// 无头浏览器不可用只降级为纯静态抓取，不阻塞服务启动。
func New(opts Options) (*Crawler, error) {
	cfg := opts.Config
	if !cfg.Enabled {
		return nil, apperr.ErrCrawlerDisabled
	}

	robots := opts.Robots
	if robots == nil && cfg.RespectRobots {
		robots = newRobotsGate(cfg)
	}

	renderer := opts.Renderer
	if renderer == nil && cfg.Headless.Enabled {
		r, err := NewRenderer(cfg.Headless)
		if err != nil {
			slog.Warn("无头浏览器不可用，将仅使用静态抓取", "error", err)
		} else {
			renderer = r
		}
	}

	fetcher := opts.Fetcher
	if fetcher == nil {
		f, err := NewFetcher(cfg, cfg.Identity, nil, renderer)
		if err != nil {
			return nil, apperr.ErrInternal.WithCause(err)
		}
		fetcher = f
	}

	return &Crawler{
		cfg:      cfg,
		scope:    opts.Scope,
		robots:   robots,
		fetcher:  fetcher,
		renderer: renderer,
	}, nil
}

// Close 释放抓取器与渲染器资源。
func (c *Crawler) Close() error {
	var firstErr error
	if c.fetcher != nil {
		if err := c.fetcher.Close(); err != nil {
			firstErr = err
		}
	}
	if c.renderer != nil {
		if err := c.renderer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// maxCareersCandidates 是定位招聘入口时允许探测的页面数上限。
//
// 刻意设得较小：发现入口只是入口，不应把官网整站爬一遍。
const maxCareersCandidates = 12

// CrawlCompany 执行一次公司抓取。
//
// 返回的 Result 中，被白名单过滤掉的岗位会以 stage=filter 的日志形式记录原因，
// 便于运营回答「为什么某个岗位没有被收录」。
func (c *Crawler) CrawlCompany(ctx context.Context, in CompanyInput) (*Result, error) {
	rules := in.Rules
	if rules == nil {
		rules = &SiteRules{}
	}
	if in.ForceHeadless {
		rules.ForceHeadless = true
	}
	maxPages := in.MaxPages
	if maxPages <= 0 {
		maxPages = c.cfg.MaxPagesPerSite
	}

	res := &Result{
		CompanyID:     in.CompanyID,
		CompanyName:   in.CompanyName,
		RobotsAllowed: true,
	}

	// 1) 定位《加入我们》入口
	careersURL, discoveryURL, err := c.locateCareers(ctx, in, rules, res)
	if err != nil {
		return res, err
	}
	res.CareersURL = careersURL
	if discoveryURL != "" {
		c.logf(res, "info", "discover", discoveryURL, "从该页面定位到招聘入口："+careersURL)
	}

	// 2) 抓取招聘页
	listDoc, err := c.fetchPage(ctx, careersURL, in, rules, res)
	if err != nil {
		return res, err
	}
	base, err := url.Parse(listDoc.FinalURL)
	if err != nil {
		base, _ = url.Parse(careersURL)
	}

	// 3) 发现岗位链接
	links := FindJobLinks(listDoc, rules, base, maxPages)
	res.JobLinksFound = len(links)
	c.logf(res, "info", "discover", careersURL,
		"招聘页发现岗位链接 "+itoa(len(links))+" 条")

	// 招聘页本身可能就是一个岗位详情页（单岗位官网很常见）。
	if len(links) == 0 {
		res.JobLinksFound = 1
		if job := c.extractOne(listDoc, rules, in, res); job != nil {
			res.Jobs = append(res.Jobs, job)
		}
		return res, nil
	}

	// 4) 逐个详情页抽取
	for index, link := range links {
		if ctx.Err() != nil {
			c.logf(res, "warn", "fetch", link.URL, "上下文已取消，提前结束本轮抓取")
			break
		}
		if index >= maxPages {
			c.logf(res, "info", "fetch", link.URL, "达到单站点抓取页面上限，停止发现更多岗位")
			break
		}

		doc, err := c.fetchPage(ctx, link.URL, in, rules, res)
		if err != nil {
			// 单个岗位页失败不影响整体，记录后继续。
			c.logf(res, "warn", "fetch", link.URL, "抓取岗位详情失败："+err.Error())
			continue
		}
		job := c.extractOne(doc, rules, in, res)
		if job == nil {
			continue
		}
		// 列表页已提供的摘要信息作为兜底补全。
		if job.Title == "" {
			job.Title = link.Title
		}
		if job.City == "" {
			job.City = link.Location
		}
		if job.Department == "" {
			job.Department = link.Department
		}
		if job.Category == "" {
			job.Category = link.Category
		}
		res.Jobs = append(res.Jobs, job)
	}

	c.logf(res, "info", "done", res.CareersURL,
		"本轮抓取完成：页面 "+itoa(res.PagesFetched)+" 个，岗位 "+itoa(len(res.Jobs))+" 条")
	return res, nil
}

// extractOne 从详情页抽取岗位并通过收集范围过滤；被过滤时记录原因并返回 nil。
func (c *Crawler) extractOne(doc *Document, rules *SiteRules, in CompanyInput, res *Result) *ExtractedJob {
	job, err := ExtractJob(doc, rules, mustParse(doc.FinalURL))
	if err != nil {
		c.logf(res, "warn", "parse", doc.FinalURL, "解析岗位信息失败："+err.Error())
		return nil
	}

	// 来源信息与推断字段补全。
	job.SourceURL = doc.FinalURL
	if job.SourceCompanyName == "" {
		job.SourceCompanyName = in.CompanyName
	}
	if job.Industry == "" {
		job.Industry = in.Industry
	}
	if job.Category == "" {
		job.Category = InferCategory(job.Title, job.Description+job.Responsibilities+job.Requirements)
	}
	if job.EmploymentType == "" {
		job.EmploymentType = InferEmploymentType(job.Title + " " + job.Description + " " + job.Requirements)
	}
	if job.Extractor == "" {
		job.Extractor = "dom"
	}

	// 收集范围过滤（取当前生效的配置）。
	scope := c.currentScope()
	ok, reason := scope.Match(job, in.Industry)
	if !ok {
		c.logf(res, "info", "filter", job.SourceURL, "岗位被过滤（"+reason+"）："+job.Title)
		return nil
	}

	// 质量分不足的岗位不进入审核队列，避免给审核员制造噪声。
	if score := ScoreJob(job); score < scope.MinQualityScore {
		c.logf(res, "info", "filter", job.SourceURL,
			"岗位信息完整度不足（得分 "+itoa(score)+" < "+itoa(scope.MinQualityScore)+"）："+job.Title)
		return nil
	}

	c.logf(res, "debug", "parse", job.SourceURL,
		"已抽取岗位："+job.Title+"（抽取器 "+job.Extractor+"）")
	return job
}

// locateCareers 定位公司的《加入我们》入口。
//
// 策略：先看配置里是否已指定；否则从官网首页开始做有界广度探测
// （首页 → 关于/公司/联系等候选页），任一页面命中招聘入口即返回。
// 探测页数被严格限制，避免对目标站点造成额外压力。
func (c *Crawler) locateCareers(ctx context.Context, in CompanyInput, rules *SiteRules, res *Result) (string, string, error) {
	if u := strings.TrimSpace(rules.CareersURL); u != "" {
		return u, "", nil
	}
	if u := strings.TrimSpace(in.CareersURL); u != "" {
		return u, "", nil
	}

	start := strings.TrimSpace(in.Website)
	if start == "" {
		return "", "", apperr.ErrCareersNotFound.
			WithMessage("公司未配置官网地址，无法自动发现《加入我们》入口")
	}

	type candidate struct {
		url   string
		depth int
	}
	visited := map[string]bool{}
	queue := []candidate{{url: start, depth: 0}}
	probed := 0

	for len(queue) > 0 && probed < maxCareersCandidates {
		item := queue[0]
		queue = queue[1:]
		if visited[item.url] {
			continue
		}
		visited[item.url] = true
		probed++

		doc, err := c.fetchPage(ctx, item.url, in, rules, res)
		if err != nil {
			c.logf(res, "debug", "discover", item.url, "探测页面失败："+err.Error())
			continue
		}
		base := mustParse(doc.FinalURL)

		if link, err := FindCareersLink(doc, rules, base); err == nil && link != nil && link.URL != "" {
			c.logf(res, "info", "discover", doc.FinalURL,
				"命中招聘入口（得分 "+itoa(link.Score)+"，规则 "+link.Reason+"）："+link.URL)
			return link.URL, doc.FinalURL, nil
		}

		if item.depth >= c.cfg.MaxDepth {
			continue
		}
		for _, next := range candidateInfoPages(doc, base, 6) {
			if !visited[next] {
				queue = append(queue, candidate{url: next, depth: item.depth + 1})
			}
		}
	}

	return "", "", apperr.ErrCareersNotFound.
		WithMessage("已在官网探测 " + itoa(probed) + " 个页面，仍未找到《加入我们》入口，可在公司配置中直接指定招聘页地址")
}

// fetchPage 抓取单个页面，并在此处统一执行抓取礼仪与 noai 检查。
func (c *Crawler) fetchPage(ctx context.Context, rawURL string, in CompanyInput, rules *SiteRules, res *Result) (*Document, error) {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return nil, apperr.ErrFetchFailed.WithMessage("无效的抓取地址：" + rawURL)
	}

	// robots.txt 许可判定与站点建议间隔。
	if c.robots != nil && in.RespectRobots {
		allowed, err := c.robots.Allowed(ctx, target, c.cfg.Identity.BotName)
		if err != nil {
			c.logf(res, "warn", "robots", rawURL, "robots 检查失败："+err.Error())
		} else if !allowed {
			res.RobotsAllowed = false
			c.logf(res, "warn", "robots", rawURL, ensureRobotsDenied(target))
			return nil, apperr.ErrRobotsDenied
		}
		if delay, err := c.robots.CrawlDelay(ctx, target, c.cfg.Identity.BotName); err == nil && delay > 0 {
			SetHostDelay(c.fetcher, target.Host, delay)
		}
	}

	doc, err := c.fetch(ctx, rawURL, rules)
	if err != nil {
		return nil, err
	}
	res.PagesFetched++
	if doc.Headless {
		res.UsedHeadless = true
	}

	// 站点声明拒绝 AI 抓取时主动放弃，这是对站点意愿的尊重。
	if c.cfg.Identity.RespectNoAI && DetectNoAI(doc) {
		c.logf(res, "warn", "robots", doc.FinalURL, "页面声明拒绝 AI 抓取（noai/nogptbot/noarchive），已跳过")
		return nil, apperr.ErrRobotsDenied.WithMessage("目标页面声明拒绝 AI 抓取")
	}
	return doc, nil
}

// fetch 选择静态抓取或无头渲染。
//
// 站点配置了 force_headless 时直接走渲染器：Fetcher 是全局单例（构造时 rules 为 nil），
// 而 rules 是「每公司一份」的，因此强制无头的判定必须放在这里，
// 否则 crawl_rules.force_headless 永远不会生效。
func (c *Crawler) fetch(ctx context.Context, rawURL string, rules *SiteRules) (*Document, error) {
	if rules != nil && rules.ForceHeadless {
		if c.renderer == nil {
			return nil, apperr.ErrFetchFailed.
				WithMessage("该公司配置了强制无头渲染，但本机未找到可用的 Chrome/Edge，请设置 crawler.headless.executable_path")
		}
		doc, err := c.renderer.Render(ctx, rawURL)
		if err != nil {
			return nil, apperr.ErrFetchFailed.WithCause(err)
		}
		return doc, nil
	}
	return c.fetcher.Fetch(ctx, rawURL)
}

// logf 同时写入结果日志与进程日志。
func (c *Crawler) logf(res *Result, level, stage, rawURL, message string) {
	res.Logs = append(res.Logs, LogEntry{Level: level, Stage: stage, URL: rawURL, Message: message})

	switch level {
	case "error":
		slog.Error("抓取日志", "stage", stage, "url", rawURL, "company", res.CompanyName, "message", message)
	case "warn":
		slog.Warn("抓取日志", "stage", stage, "url", rawURL, "company", res.CompanyName, "message", message)
	case "debug":
		slog.Debug("抓取日志", "stage", stage, "url", rawURL, "company", res.CompanyName, "message", message)
	default:
		slog.Info("抓取日志", "stage", stage, "url", rawURL, "company", res.CompanyName, "message", message)
	}
}

// infoPageKeywords 是「可能包含招聘入口」的次级页面关键词。
var infoPageKeywords = []string{
	"关于", "公司", "简介", "联系", "招聘", "加入", "人才", "更多", "网站地图", "导航",
	"about", "company", "contact", "career", "job", "join", "talent", "recruit", "sitemap", "hr",
}

// candidateInfoPages 挑出可能通往招聘入口的同域页面。
func candidateInfoPages(doc *Document, base *url.URL, max int) []string {
	if base == nil || doc == nil || doc.HTML == "" {
		return nil
	}
	parsed, err := goquery.NewDocumentFromReader(strings.NewReader(doc.HTML))
	if err != nil {
		return nil
	}

	seen := map[string]bool{}
	var out []string
	parsed.Find("a[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		href, ok := s.Attr("href")
		if !ok {
			return true
		}
		href = strings.TrimSpace(href)
		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
			return true
		}
		abs, err := base.Parse(href)
		if err != nil || abs.Host != base.Host {
			return true
		}
		abs.Fragment = ""
		key := abs.String()
		if seen[key] {
			return true
		}

		haystack := strings.ToLower(s.Text() + " " + abs.Path)
		for _, kw := range infoPageKeywords {
			if strings.Contains(haystack, strings.ToLower(kw)) {
				seen[key] = true
				out = append(out, key)
				break
			}
		}
		return len(out) < max
	})
	return out
}

// mustParse 解析 URL，失败时返回 nil（调用方需容忍）。
func mustParse(raw string) *url.URL {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return u
}

// itoa 本地整数转字符串，避免为个别日志引入 strconv。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
