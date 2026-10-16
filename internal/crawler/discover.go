// discover.go 实现「定位《加入我们》入口」与「列表页发现岗位链接」。
//
// 两件事都不依赖站点结构：先用可配置规则（SiteRules）精确命中，规则缺失时退化为
// 通用启发式打分。打分的意义在于可解释——每个链接为什么被选中都会记在
// Link.Reason 里，便于运营排查「为什么这家公司的入口找错了」。
package crawler

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// careersTextRule 是一组招聘入口文案及其权重（权重越高越典型）。
type careersTextRule struct {
	texts []string
	score int
	label string
}

// careersTextRules 是内置的招聘入口文案词表。
//
// 分层的原因：「加入我们」「Join Us」这类词几乎只出现在招聘入口上，命中即可
// 高置信度判定；而「招聘」「职位」也可能出现在新闻、介绍等页面，权重必须更低，
// 否则容易把「招聘会报道」之类的链接当成入口。
var careersTextRules = []careersTextRule{
	{[]string{"加入我们", "加入我们吧", "joinus", "join us", "joinourteam", "join our team", "careers", "career"}, 120, "文案命中"},
	{[]string{"招贤纳士", "诚聘英才", "人才招聘", "招聘英才", "社会招聘", "校园招聘", "人才加盟", "招才纳士"}, 100, "文案命中"},
	{[]string{"join", "recruit", "recruitment", "talent", "hiring", "workwithus", "work with us", "jobopenings", "job opening", "wearehiring"}, 90, "文案命中"},
	{[]string{"招聘", "诚聘", "职位", "岗位", "工作机会", "招人", "jobs", "job", "positions"}, 70, "文案命中"},
}

// careersPathRule 是 href 路径关键词及其权重。
type careersPathRule struct {
	keyword string
	score   int
}

// careersPathRules 是招聘入口常见路径。
var careersPathRules = []careersPathRule{
	{"/about/careers", 70},
	{"/careers", 65},
	{"/career", 60},
	{"/joinus", 60},
	{"/join-us", 60},
	{"/zhaopin", 55},
	{"/recruit", 55},
	{"/join", 50},
	{"/jobs", 50},
	{"/job", 45},
	{"/talent", 45},
	{"/hr", 45},
	{"/campus", 40},
	{"/position", 40},
	{"/subsite", 25},
}

// careersNegativeTexts 是明显与招聘无关的链接文案。
//
// 只有在完全没有正向信号时才用于排除，避免误杀「加入我们-联系我们」这类
// 同时包含正负词的链接。
var careersNegativeTexts = []string{
	"登录", "登陆", "注册", "首页", "关于我们", "联系我们", "联系方式", "新闻", "资讯", "动态",
	"隐私", "条款", "协议", "下载", "客户端", "微博", "微信", "公众号", "网站地图", "sitemap",
	"english", "en", "中文", "简体", "繁体", "copyright", "版权",
}

// careersSubdomainRe 匹配招聘子域（careers.xxx.com / zhaopin.xxx.com 等）。
//
// 有些官网把招聘站放在独立子域，甚至只在 JS 或 JSON 里出现地址，因此除了遍历
// <a> 之外还要对整页 HTML 做一次文本扫描。
var careersSubdomainRe = regexp.MustCompile(
	`(?i)https?://(?:careers|jobs?|hr|zhaopin|talent|recruit|campus)\.[a-z0-9][a-z0-9.-]*(?:/[^\s"'<>\\)\]}]*)?`)

// careersSubdomainScore 是招聘子域链接的基础分：几乎可以直接确认是招聘入口。
const careersSubdomainScore = 110

// FindCareersLink 在官网/招聘入口页面上定位《加入我们》类的招聘入口链接。
//
// 只负责「当前页面」的定位；站点级递归发现在调度层完成，因此找不到时返回
// apperr.ErrCareersNotFound，调用方可以继续探测其它候选页。
func FindCareersLink(doc *Document, rules *SiteRules, base *url.URL) (*Link, error) {
	notFound := apperr.ErrCareersNotFound

	if doc == nil || strings.TrimSpace(doc.HTML) == "" {
		return nil, notFound.WithMessage("页面内容为空，无法定位招聘入口")
	}
	if rules == nil {
		rules = &SiteRules{}
	}
	if base == nil {
		base = mustParseFirst(doc.FinalURL, doc.URL)
	}
	if base == nil {
		return nil, notFound.WithMessage("缺少可用的页面地址，无法解析招聘入口链接")
	}

	// 1) 配置直接指定：最高优先级，无需任何推断。
	if raw := strings.TrimSpace(rules.CareersURL); raw != "" {
		abs, ok := absoluteHTTPURL(base, raw)
		if !ok {
			return nil, notFound.WithMessage("配置的招聘页地址无效：" + raw)
		}
		return &Link{URL: abs, Text: "配置指定", Score: 1000, Reason: "crawl_rules.careers_url"}, nil
	}

	parsed, err := goquery.NewDocumentFromReader(strings.NewReader(doc.HTML))
	if err != nil {
		return nil, notFound.WithCause(err).WithMessage("招聘入口页面解析失败")
	}

	// 2) 配置的选择器：命中第一个即返回，同样给最高分。
	if sel := strings.TrimSpace(rules.CareersLinkSelector); sel != "" {
		node := parsed.Find(sel).First()
		if node.Length() > 0 {
			if href, ok := node.Attr("href"); ok {
				if abs, ok := absoluteHTTPURL(base, strings.TrimSpace(href)); ok {
					return &Link{
						URL:    abs,
						Text:   linkText(node),
						Score:  900,
						Reason: "crawl_rules.careers_link_selector",
					}, nil
				}
			}
		}
		// 选择器没命中时不报错，继续走通用启发式，保证站点改版后仍能工作。
	}

	extraTexts := normalizeKeys(rules.CareersLinkTexts)

	best := (*Link)(nil)
	consider := func(candidate Link) {
		if candidate.URL == "" || candidate.Score <= 0 {
			return
		}
		if best == nil || candidate.Score > best.Score {
			clone := candidate
			best = &clone
		}
	}

	// 3) 遍历所有 <a> 打分。
	parsed.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		href = strings.TrimSpace(href)
		if href == "" || strings.HasPrefix(href, "#") {
			return
		}
		lowerHref := strings.ToLower(href)
		if strings.HasPrefix(lowerHref, "javascript:") ||
			strings.HasPrefix(lowerHref, "mailto:") ||
			strings.HasPrefix(lowerHref, "tel:") {
			return
		}

		abs, ok := absoluteHTTPURL(base, href)
		if !ok {
			return
		}
		text := linkText(s)
		haystack := normalizeKey(text)
		path := strings.ToLower(parsed4Path(abs))

		score := 0
		reasons := make([]string, 0, 3)

		// 3.1 文案命中（权重最高）
		textScore, textReason := matchCareersText(haystack, extraTexts)
		if textScore > 0 {
			score += textScore
			reasons = append(reasons, textReason)
		}

		// 3.2 href 路径命中
		if hit, pathScore := matchCareersPath(path); hit != "" {
			score += pathScore
			reasons = append(reasons, "路径命中:"+hit)
		}

		// 3.3 招聘子域：基本可以直接确认
		if isCareersSubdomain(abs) {
			score += careersSubdomainScore
			reasons = append(reasons, "招聘子域")
		}

		// 3.4 结构信号：导航位通常是站点主入口
		if s.ParentsFiltered("nav,header,footer").Length() > 0 {
			score += 15
			reasons = append(reasons, "位于导航/页脚")
		}
		if n := utf8.RuneCountInString(text); n >= 2 && n <= 30 {
			score += 10
		}

		// 3.5 无关项排除：仅在没有正向信号时生效
		if score == 0 {
			return
		}
		if score < 90 && careersTextIsNegative(haystack) {
			return
		}
		// 已经被 robots/抽取层的其它环节排除了跨站链接，这里允许跨站招聘子域。
		consider(Link{
			URL:    abs,
			Text:   text,
			Score:  score,
			Reason: strings.Join(reasons, "；"),
		})
	})

	// 4) 扫描整页文本里的招聘子域地址（可能不在 <a> 中）。
	for _, raw := range careersSubdomainRe.FindAllString(doc.HTML, 8) {
		abs, ok := absoluteHTTPURL(base, strings.TrimRight(raw, `.,;:!?)]}"'`))
		if !ok {
			continue
		}
		consider(Link{
			URL:    abs,
			Text:   "",
			Score:  careersSubdomainScore,
			Reason: "页面文本中的招聘子域",
		})
	}

	if best == nil {
		return nil, notFound
	}
	return best, nil
}

// FindJobLinks 在招聘页（列表页）中发现岗位详情链接。
//
// 返回结果按页面出现顺序排列，最多 limit 条（limit ≤ 0 时使用 200）。
func FindJobLinks(doc *Document, rules *SiteRules, base *url.URL, limit int) []JobLink {
	if limit <= 0 {
		limit = 200
	}
	if doc == nil || strings.TrimSpace(doc.HTML) == "" {
		return nil
	}
	if rules == nil {
		rules = &SiteRules{}
	}
	parsed, err := goquery.NewDocumentFromReader(strings.NewReader(doc.HTML))
	if err != nil {
		return nil
	}
	if base == nil {
		base = mustParseFirst(doc.FinalURL, doc.URL)
	}
	if base == nil {
		return nil
	}

	var candidates []jobLinkCandidate
	if rules.ListSelector != "" || rules.ItemSelector != "" || rules.LinkSelector != "" {
		candidates = collectByRules(parsed, rules, base)
	}
	if len(candidates) == 0 {
		// 规则未命中或未配置时退回通用启发式，两者共享同一套结构化补全逻辑。
		candidates = collectByHeuristic(parsed, base)
	}

	out := make([]JobLink, 0, min(len(candidates), limit))
	seen := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		if c.score < jobLinkMinScore {
			continue
		}
		if seen[c.link.URL] {
			continue
		}
		seen[c.link.URL] = true
		out = append(out, c.link)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// DetectNoAI 判断页面是否声明拒绝 AI 抓取。
//
// 识别 <meta name="robots"> 等机器人元信息中的 noai / noimageai / nogptbot /
// noarchive，以及响应头 X-Robots-Tag；命中任意一项即认为站点拒绝 AI 抓取。
func DetectNoAI(doc *Document) bool {
	if doc == nil {
		return false
	}

	// 响应头优先：它可能来自 CDN，且不依赖页面能否解析。
	for name, values := range doc.Header {
		if !strings.EqualFold(name, "X-Robots-Tag") {
			continue
		}
		for _, v := range values {
			if containsNoAIToken(v) {
				return true
			}
		}
	}

	if strings.TrimSpace(doc.HTML) == "" {
		return false
	}
	parsed, err := goquery.NewDocumentFromReader(strings.NewReader(doc.HTML))
	if err != nil {
		return false
	}

	found := false
	parsed.Find("meta").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		name, _ := s.Attr("name")
		content, _ := s.Attr("content")
		if content == "" {
			// 兼容 <meta http-equiv="X-Robots-Tag" content="...">
			if httpEquiv, ok := s.Attr("http-equiv"); ok && strings.EqualFold(strings.TrimSpace(httpEquiv), "X-Robots-Tag") {
				name = "robots"
			}
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if !strings.Contains(name, "robot") && name != "gptbot" {
			return true
		}
		if containsNoAIToken(content) {
			found = true
			return false
		}
		return true
	})
	return found
}

// —— FindCareersLink 的辅助逻辑 ——

// matchCareersText 在归一化文案上匹配内置词表与站点补充词表，返回得分与原因。
func matchCareersText(haystack string, extra []string) (int, string) {
	if haystack == "" {
		return 0, ""
	}
	for _, kw := range extra {
		if kw != "" && strings.Contains(haystack, kw) {
			return 110, "扩展文案命中:" + kw
		}
	}
	for _, rule := range careersTextRules {
		for _, kw := range rule.texts {
			if strings.Contains(haystack, normalizeKey(kw)) {
				return rule.score, rule.label + ":" + kw
			}
		}
	}
	return 0, ""
}

// matchCareersPath 判断路径是否命中招聘入口关键词。
func matchCareersPath(path string) (string, int) {
	for _, rule := range careersPathRules {
		if strings.Contains(path, rule.keyword) {
			return rule.keyword, rule.score
		}
	}
	return "", 0
}

// careersTextIsNegative 判断链接文案是否属于明显无关项。
func careersTextIsNegative(haystack string) bool {
	for _, neg := range careersNegativeTexts {
		if strings.Contains(haystack, normalizeKey(neg)) {
			return true
		}
	}
	return false
}

// isCareersSubdomain 判断绝对地址是否落在常见招聘子域上。
func isCareersSubdomain(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, prefix := range []string{"careers", "career", "jobs", "job", "hr", "zhaopin", "talent", "recruit", "campus"} {
		if strings.HasPrefix(host, prefix+".") {
			return true
		}
	}
	return false
}

// linkText 取链接文案：优先可见文本，其次 title 属性，最后图片 alt。
func linkText(s *goquery.Selection) string {
	if s == nil {
		return ""
	}
	if text := cleanInlineText(s.Text()); text != "" {
		return text
	}
	if title, ok := s.Attr("title"); ok {
		if text := cleanInlineText(title); text != "" {
			return text
		}
	}
	if alt, ok := s.Find("img[alt]").First().Attr("alt"); ok {
		return cleanInlineText(alt)
	}
	return ""
}

// parsed4Path 返回用于关键词匹配的路径（含查询串，统一小写）。
func parsed4Path(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return strings.ToLower(raw)
	}
	path := u.Path
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return path
}

// absoluteHTTPURL 把相对地址解析为绝对地址，并只接受 http/https。
func absoluteHTTPURL(base *url.URL, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	// 协议相对地址（//example.com/careers）需要补 scheme。
	if strings.HasPrefix(raw, "//") {
		scheme := "https"
		if base != nil && base.Scheme != "" {
			scheme = base.Scheme
		}
		raw = scheme + ":" + raw
	}
	var (
		abs *url.URL
		err error
	)
	if base != nil {
		abs, err = base.Parse(raw)
	} else {
		abs, err = url.Parse(raw)
	}
	if err != nil || abs == nil || abs.Host == "" {
		return "", false
	}
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return "", false
	}
	abs.Fragment = ""
	return abs.String(), true
}

// mustParseFirst 依次解析候选地址，返回第一个可用结果。
func mustParseFirst(raws ...string) *url.URL {
	for _, raw := range raws {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			return u
		}
	}
	return nil
}

// —— FindJobLinks 的辅助逻辑 ——

// jobLinkMinScore 是判定「像岗位详情页」的最低分。
//
// 取 30 分：结构信号（同一容器重复 35 分、共享路径前缀 25 分）加上标题文本
// （10 分）就能达标，而只有单一弱信号、又没有可见文案的链接会被过滤掉。
const jobLinkMinScore = 30

// jobLinkCandidate 是列表页中一个待定的岗位链接。
type jobLinkCandidate struct {
	link   JobLink
	score  int
	sig    string // 父容器结构签名，用于识别「同一列表内的重复结构」
	prefix string // 路径前缀签名
	node   *goquery.Selection
}

// collectByRules 按站点配置的选择器收集岗位链接。
func collectByRules(parsed *goquery.Document, rules *SiteRules, base *url.URL) []jobLinkCandidate {
	container := parsed.Selection
	if sel := strings.TrimSpace(rules.ListSelector); sel != "" {
		if found := parsed.Find(sel); found.Length() > 0 {
			container = found
		}
	}

	var out []jobLinkCandidate
	appendLink := func(linkSel *goquery.Selection, scope *goquery.Selection) {
		href, ok := linkSel.Attr("href")
		if !ok {
			return
		}
		abs, ok := absoluteHTTPURL(base, href)
		if !ok || abs == base.String() {
			return
		}
		link := JobLink{URL: abs, Title: linkText(linkSel)}
		fillJobLinkDetail(&link, scope)
		out = append(out, jobLinkCandidate{link: link, score: 100, node: linkSel})
	}

	switch {
	case strings.TrimSpace(rules.ItemSelector) != "":
		container.Find(rules.ItemSelector).Each(func(_ int, item *goquery.Selection) {
			linkSel := item
			if sel := strings.TrimSpace(rules.LinkSelector); sel != "" {
				linkSel = item.Find(sel).First()
			} else {
				linkSel = item.Find("a[href]").First()
			}
			if linkSel.Length() > 0 {
				appendLink(linkSel, item)
			}
		})
	case strings.TrimSpace(rules.LinkSelector) != "":
		container.Find(rules.LinkSelector).Each(func(_ int, s *goquery.Selection) {
			appendLink(s, jobItemScope(s))
		})
	default:
		container.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
			appendLink(s, jobItemScope(s))
		})
	}
	return out
}

// collectByHeuristic 在没有可用规则时用通用启发式发现岗位链接。
//
// 两类信号叠加：
//   - 路径信号：URL 里出现 job/detail/position 等岗位详情特征；
//   - 结构信号：同一容器内重复出现的同级链接（典型岗位列表），或共享同一路径前缀。
//
// 单靠路径会漏掉「/news/123」这类猜不出语义的站点，单靠结构会误收导航里的重复项，
// 两者叠加能在保证召回的同时压住误报。
func collectByHeuristic(parsed *goquery.Document, base *url.URL) []jobLinkCandidate {
	type raw struct {
		link      JobLink
		node      *goquery.Selection
		sig       string
		prefix    string
		pathHit   bool
		insideNav bool
	}

	var (
		items   []raw
		sigCnt  = map[string]int{}
		prefCnt = map[string]int{}
		scanned int
	)
	const maxScan = 4000

	parsed.Find("a[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		scanned++
		if scanned > maxScan {
			return false
		}
		href, _ := s.Attr("href")
		href = strings.TrimSpace(href)
		lower := strings.ToLower(href)
		if href == "" || strings.HasPrefix(href, "#") ||
			strings.HasPrefix(lower, "javascript:") ||
			strings.HasPrefix(lower, "mailto:") ||
			strings.HasPrefix(lower, "tel:") {
			return true
		}
		abs, ok := absoluteHTTPURL(base, href)
		if !ok || abs == base.String() {
			return true
		}
		u, err := url.Parse(abs)
		if err != nil {
			return true
		}
		// 只收集同一站点（含招聘子域）的链接，避免被外链带跑。
		if !sameSite(u.Hostname(), base.Hostname()) {
			return true
		}
		if isNonPageResource(u.Path) {
			return true
		}
		text := linkText(s)
		pathHit := jobPathPattern.MatchString(strings.ToLower(u.Path))
		// 导航类文案只有在路径也看不出岗位特征时才排除，避免误杀
		// 「关于XX工程师」这类真实岗位标题。
		if !pathHit && jobTextIsNegative(normalizeKey(text+" "+u.Path)) {
			return true
		}

		sig := selectionSignature(s)
		prefix := pathPrefix(u.Path)

		sigCnt[sig]++
		prefCnt[prefix]++

		items = append(items, raw{
			link:      JobLink{URL: abs, Title: text},
			node:      s,
			sig:       sig,
			prefix:    prefix,
			pathHit:   pathHit,
			insideNav: s.ParentsFiltered("nav,header,footer").Length() > 0,
		})
		return true
	})

	out := make([]jobLinkCandidate, 0, len(items))
	for _, item := range items {
		// 各路信号累加而不是互斥：路径语义强、结构重复也能相互印证。
		score := 0
		if item.pathHit {
			score += 50
		}
		if item.prefix != "" && item.sig != "" && sigCnt[item.sig] >= 3 {
			// 同一容器内重复结构：典型的岗位列表。
			// 要求 prefix 非空（即不落在新闻/关于等栏目下），避免把资讯列表当岗位列表。
			score += 35
		}
		if item.prefix != "" && prefCnt[item.prefix] >= 3 {
			// 共享同一级路径前缀：整块链接都指向同一个栏目。
			score += 25
		}
		// 列表项通常还有标题文本，纯图标/空文本的链接更可能是装饰。
		if strings.TrimSpace(item.link.Title) != "" {
			score += 10
		}
		if item.insideNav {
			score -= 30
		}
		if score <= 0 {
			continue
		}

		scope := jobItemScope(item.node)
		link := item.link
		fillJobLinkDetail(&link, scope)
		out = append(out, jobLinkCandidate{
			link:   link,
			score:  score,
			sig:    item.sig,
			prefix: item.prefix,
			node:   item.node,
		})
	}
	return out
}

// jobTextIsNegative 判断链接文本/路径是否属于导航类无关项。
var jobNavKeywords = []string{
	"登录", "注册", "首页", "关于", "联系", "新闻", "资讯", "隐私", "条款", "协议",
	"下载", "微博", "微信", "公众号", "地图", "sitemap", "english", "中文",
	"login", "signin", "signup", "register", "home", "about", "contact", "news", "privacy",
}

func jobTextIsNegative(haystack string) bool {
	for _, kw := range jobNavKeywords {
		if strings.Contains(haystack, normalizeKey(kw)) {
			return true
		}
	}
	return false
}

// jobPathPattern 匹配「看起来像岗位详情页」的路径特征。
var jobPathPattern = regexp.MustCompile(
	`(?:^|[/_\-.])(?:job|jobs|jobdetail|job-detail|detail|details|position|positions|apply|recruit|recruitment|zhiwei|opening|openings|vacancy|vacancies|career|careers|post|posts)(?:[/_\-.]|$)`)

// isNonPageResource 过滤明显不是详情页的资源与文件。
func isNonPageResource(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".zip", ".rar", ".7z",
		".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".ico", ".mp4", ".mp3", ".css", ".js",
	} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// selectionSignature 生成节点的结构签名（标签名 + class + 父级标签），
// 用于识别「同一列表容器内的重复结构」。
func selectionSignature(s *goquery.Selection) string {
	if s == nil || len(s.Nodes) == 0 {
		return ""
	}
	node := s.Nodes[0]
	parts := []string{node.Data}
	if class, ok := s.Attr("class"); ok {
		parts = append(parts, firstClassToken(class))
	}
	if parent := s.Parent(); parent.Length() > 0 && len(parent.Nodes) > 0 {
		parts = append(parts, parent.Nodes[0].Data)
		if class, ok := parent.Attr("class"); ok {
			parts = append(parts, firstClassToken(class))
		}
	}
	sig := strings.Join(parts, ".")
	if sig == "." {
		return ""
	}
	return sig
}

// firstClassToken 取 class 属性中第一个有意义的词。
func firstClassToken(class string) string {
	for _, token := range strings.Fields(class) {
		if token != "" {
			return token
		}
	}
	return ""
}

// pathPrefix 取路径的第一级作为前缀签名（/jobs/123 → /jobs）。
//
// 只取一级：同一列表下的详情页通常共享第一级路径，而按两级切分时每个链接都会
// 得到各自不同的前缀，分组统计就失去意义了。
func pathPrefix(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		return ""
	}
	prefix := "/" + segments[0]
	// 栏目型路径（新闻/关于/产品等）即使重复出现也不是岗位列表。
	if nonJobPathSegments[strings.ToLower(segments[0])] {
		return ""
	}
	return prefix
}

// nonJobPathSegments 是不会承载岗位列表的一级路径。
var nonJobPathSegments = map[string]bool{
	"news": true, "article": true, "articles": true, "blog": true, "press": true, "media": true,
	"about": true, "aboutus": true, "contact": true, "contactus": true, "support": true,
	"product": true, "products": true, "solution": true, "solutions": true, "service": true,
	"services": true, "download": true, "downloads": true, "login": true, "register": true,
	"user": true, "users": true, "account": true, "help": true, "faq": true, "policy": true,
	"privacy": true, "terms": true, "index": true, "html": true, "en": true, "cn": true,
	"static": true, "assets": true, "css": true, "js": true, "images": true, "img": true,
}

// jobItemScope 返回用于补全地点/时间/部门等字段的条目容器。
func jobItemScope(s *goquery.Selection) *goquery.Selection {
	if s == nil {
		return nil
	}
	if scope := s.Closest("li,tr,article,dd,section,div"); scope.Length() > 0 {
		return scope
	}
	return s.Parent()
}

// fillJobLinkDetail 从条目容器里顺带抽取地点、发布时间、部门等摘要字段。
//
// 抽不到就留空——列表页信息缺失是常态，详情页抽取才是权威来源。
func fillJobLinkDetail(link *JobLink, scope *goquery.Selection) {
	if link == nil || scope == nil || scope.Length() == 0 {
		return
	}
	if link.Title == "" {
		link.Title = cleanInlineText(firstText(scope,
			"h1", "h2", "h3", "h4", "h5", "h6",
			"[class*=job-title]", "[class*=jobTitle]", "[class*=position-title]", "[class*=title]", "[class*=name]",
			"strong", "b"))
	}
	if link.Location == "" {
		link.Location = cleanInlineText(firstText(scope,
			"[class*=location]", "[class*=address]", "[class*=city]", "[class*=area]", "[class*=region]", ".addr", ".place"))
	}
	if link.PublishedAt == "" {
		if node := scope.Find("time").First(); node.Length() > 0 {
			if dt, ok := node.Attr("datetime"); ok && strings.TrimSpace(dt) != "" {
				link.PublishedAt = strings.TrimSpace(dt)
			}
		}
	}
	if link.PublishedAt == "" {
		link.PublishedAt = cleanInlineText(firstText(scope,
			"[class*=date]", "[class*=time]", "[class*=published]", "[class*=update]", ".date", ".time"))
	}
	if link.Department == "" {
		link.Department = cleanInlineText(firstText(scope,
			"[class*=department]", "[class*=dept]", "[class*=team]", "[class*=job-family]", "[class*=category]"))
	}
	if link.Category == "" {
		link.Category = cleanInlineText(firstText(scope,
			"[class*=category]", "[class*=type]", "[class*=tag]", "[class*=label]"))
	}
}

// firstText 返回选择器列表中第一个非空文本。
func firstText(scope *goquery.Selection, selectors ...string) string {
	for _, sel := range selectors {
		if scope == nil || scope.Length() == 0 {
			return ""
		}
		if text := cleanInlineText(scope.Find(sel).First().Text()); text != "" {
			return text
		}
	}
	return ""
}

// sameSite 判断两个主机名是否属于同一站点。
//
// 比较最后两级域名，使 www.example.com 与 careers.example.com 视为同站；
// IP 地址直接比较。
func sameSite(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	// 至少一侧是 IP 时不做后缀比较。
	if isIPLike(a) || isIPLike(b) {
		return false
	}
	return registrableSuffix(a) == registrableSuffix(b)
}

// isIPLike 粗判主机名是否为 IP 字面量。
func isIPLike(host string) bool {
	if host == "" {
		return false
	}
	if strings.Contains(host, ":") {
		return true
	}
	for _, r := range host {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return strings.Contains(host, ".")
}

// registrableSuffix 返回主机名的最后两级标签。
func registrableSuffix(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// containsNoAIToken 判断一段元信息文本是否包含拒绝 AI 抓取的标记。
func containsNoAIToken(s string) bool {
	lower := strings.ToLower(s)
	if lower == "" {
		return false
	}
	for _, token := range []string{"noai", "noimageai", "nogptbot", "noarchive"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}
