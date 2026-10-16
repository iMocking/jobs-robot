// fetch.go 实现页面抓取：静态 HTTP 优先，内容不足或站点要求时切换到无头浏览器。
//
// 关键取舍：
//   - 静态抓取使用 colly（本项目已依赖），它负责 gzip 解压与字符集纠正；
//     HTTP 连接池由我们自己构造的 http.Client 统一复用，避免每个请求新建连接。
//   - 「同一域名串行 + 最小间隔 + 随机抖动」由 hostScheduler 实现，而不是用
//     colly 的 LimitRule：robots.txt 声明的 CrawlDelay 是运行期才拿到的，
//     需要能按域名动态覆盖（见 SetHostDelay）。
//   - 无头渲染只作为兜底：静态正文长度不足时使用；站点规则要求强制无头时，
//     渲染失败才向上报错，其余情况一律回退静态结果，由调用方日志层记录。
package crawler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/gocolly/colly/v2"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

const (
	// defaultPerHostLimit 是未配置（≤0）时的同域名并发上限：串行最保守。
	defaultPerHostLimit = 1
	// defaultRequestTimeout 是未配置（≤0）时的单次请求超时。
	defaultRequestTimeout = 30 * time.Second
	// defaultHeadlessTimeout 是未配置（≤0）时的单次渲染超时。
	defaultHeadlessTimeout = 45 * time.Second
	// maxBodyBytes 限制单页正文体积，避免个别站点返回超大响应拖垮进程。
	maxBodyBytes = 8 << 20
	// backoffBase / backoffMax 控制重试退避：1s、2s、4s……最多 30s。
	backoffBase = time.Second
	backoffMax  = 30 * time.Second
	// maxRetryAfter 限制 Retry-After 的最大等待时间：站点点名要求等一小时时
	// 我们不能真的挂住整个采集任务，超过上限会放弃重试。
	maxRetryAfter = 2 * time.Minute
)

// NewFetcher 构造抓取器：静态 HTTP 优先，内容不足或强制时切换无头浏览器。
//
// renderer 为 nil 时退化为纯静态抓取；rules 决定是否强制无头渲染。
func NewFetcher(cfg config.Crawler, id config.Identity, rules *SiteRules, renderer Renderer) (Fetcher, error) {
	if strings.TrimSpace(id.BotName) == "" {
		// 身份头是本项目的合规基线，缺少名字时构建出的请求无法被站长识别，
		// 因此直接拒绝构造，而不是发出「匿名」请求。
		return nil, apperr.ErrFetchFailed.WithMessage("抓取器初始化失败：缺少机器人身份配置（crawler.identity.bot_name）")
	}
	if rules == nil {
		rules = &SiteRules{}
	}

	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	perHost := cfg.PerHostLimit
	if perHost <= 0 {
		perHost = defaultPerHostLimit
	}

	f := &staticFetcher{
		cfg:      cfg,
		identity: id,
		rules:    rules,
		renderer: renderer,
		hosts:    newHostScheduler(cfg.Delay, cfg.RandomDelay, perHost),
		client: &http.Client{
			// Timeout 覆盖「连接 + 读完整响应体」，等价于单次请求超时。
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				MaxIdleConns:          64,
				MaxIdleConnsPerHost:   perHost + 1,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   15 * time.Second,
				ExpectContinueTimeout: time.Second,
				ForceAttemptHTTP2:     true,
			},
		},
	}

	// 无头浏览器发出的请求也应当声明机器人身份；Renderer 由调用方构造，
	// 这里把配置中的 Identity 注入进去（见 chromeRenderer.setIdentity）。
	if setter, ok := renderer.(interface{ setIdentity(config.Identity) }); ok {
		setter.setIdentity(id)
	}
	return f, nil
}

// SetHostDelay 由调度层在读取 robots.txt 的 CrawlDelay 后调用，覆盖该域名的最小请求间隔。
//
// 只对实现了内部接口的抓取器生效；传入非本项目实现（例如测试替身）时静默忽略。
func SetHostDelay(f Fetcher, host string, d time.Duration) {
	if f == nil || d <= 0 {
		return
	}
	setter, ok := f.(interface {
		setHostDelay(host string, d time.Duration)
	})
	if !ok {
		return
	}
	setter.setHostDelay(hostKey(host), d)
}

// —— 抓取礼仪：同域名串行 + 最小间隔 + 并发上限 ——

// hostScheduler 维护每个域名的出站节奏。
//
// 它对每个域名维护一个容量为 PerHostLimit 的信号量（并发上限）和一把
// 串行「等待 + 记录起点」的互斥锁，从而保证：
//  1. 同一域名同时在飞的请求数不超过 PerHostLimit；
//  2. 相邻两次请求的发起间隔不小于 max(全局 Delay, 该域名 robots 建议间隔)；
//  3. 每次请求前再叠加 [0, RandomDelay) 的随机抖动，避免形成固定节拍。
type hostScheduler struct {
	baseDelay   time.Duration
	randomDelay time.Duration
	limit       int

	mu    sync.Mutex
	hosts map[string]*hostState
}

// hostState 是单个域名的调度状态。
type hostState struct {
	// sem 是并发闸门，容量即 PerHostLimit。
	sem chan struct{}

	mu    sync.Mutex
	last  time.Time
	delay time.Duration
}

func newHostScheduler(baseDelay, randomDelay time.Duration, limit int) *hostScheduler {
	if limit <= 0 {
		limit = defaultPerHostLimit
	}
	return &hostScheduler{
		baseDelay:   baseDelay,
		randomDelay: randomDelay,
		limit:       limit,
		hosts:       make(map[string]*hostState),
	}
}

func (s *hostScheduler) state(host string) *hostState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.hosts[host]
	if !ok {
		st = &hostState{sem: make(chan struct{}, s.limit)}
		s.hosts[host] = st
	}
	return st
}

// acquire 取得一次出站许可；返回的 release 必须在请求结束后调用。
func (s *hostScheduler) acquire(ctx context.Context, host string) (func(), error) {
	st := s.state(host)

	select {
	case st.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release := func() { <-st.sem }

	if err := st.waitTurn(ctx, s.baseDelay, s.randomDelay); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

// waitTurn 在真正发起请求前等待到该域名的下一个可用时间点。
func (st *hostState) waitTurn(ctx context.Context, base, jitter time.Duration) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	gap := base
	if st.delay > gap {
		gap = st.delay
	}
	if !st.last.IsZero() && gap > 0 {
		if wait := gap - time.Since(st.last); wait > 0 {
			if err := sleepContext(ctx, wait); err != nil {
				return err
			}
		}
	}
	if jitter > 0 {
		if err := sleepContext(ctx, time.Duration(mathrand.Int64N(int64(jitter)))); err != nil {
			return err
		}
	}
	st.last = time.Now()
	return nil
}

// setDelay 覆盖某域名的 robots 建议间隔。
func (s *hostScheduler) setDelay(host string, d time.Duration) {
	if d <= 0 {
		return
	}
	st := s.state(host)
	st.mu.Lock()
	st.delay = d
	st.mu.Unlock()
}

// hostKey 归一化域名：统一小写，并去掉 80/443 默认端口，避免同一站点被拆成多个桶。
func hostKey(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		return host
	}
	if p == "80" || p == "443" {
		return h
	}
	return h + ":" + p
}

// sleepContext 是可被取消的等待。
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// —— 静态抓取器 ——

// staticFetcher 是默认的 Fetcher 实现。
type staticFetcher struct {
	cfg      config.Crawler
	identity config.Identity
	rules    *SiteRules
	renderer Renderer

	client *http.Client
	hosts  *hostScheduler
}

// setHostDelay 实现 SetHostDelay 所需的内部接口。
func (f *staticFetcher) setHostDelay(host string, d time.Duration) {
	f.hosts.setDelay(host, d)
}

// Fetch 抓取页面：先静态，必要时再用无头浏览器渲染。
func (f *staticFetcher) Fetch(ctx context.Context, rawURL string) (*Document, error) {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, apperr.ErrFetchFailed.WithMessage("抓取地址无效：" + rawURL)
	}

	staticDoc, staticErr := f.fetchStatic(ctx, target)

	force := f.rules != nil && f.rules.ForceHeadless
	// 正文太短通常意味着页面由 JS 渲染，静态 HTML 里没有岗位内容。
	byLength := staticErr == nil && staticDoc != nil &&
		f.cfg.Headless.Enabled &&
		staticDoc.TextLength() < f.cfg.Headless.FallbackMinLength

	if f.renderer == nil || (!force && !byLength) {
		if staticErr != nil {
			return nil, staticErr
		}
		return staticDoc, nil
	}

	var (
		rendered  *Document
		renderErr error
	)
	// 无头渲染同样是对目标站点的请求，必须走同一套域名节奏控制。
	if release, err := f.hosts.acquire(ctx, hostKey(target.Host)); err != nil {
		renderErr = err
	} else {
		rendered, renderErr = f.renderer.Render(ctx, target.String())
		release()
	}

	if renderErr == nil && rendered != nil {
		rendered.Headless = true
		return rendered, nil
	}
	if renderErr == nil {
		renderErr = errors.New("渲染器未返回内容")
	}
	slog.Warn("无头渲染失败，按静态结果处理",
		"url", target.String(), "force_headless", force, "error", renderErr)

	if force {
		return nil, apperr.ErrFetchFailed.
			WithCause(renderErr).
			WithMessage("站点规则要求强制无头渲染，但渲染失败：" + target.String())
	}
	if staticErr != nil {
		return nil, staticErr
	}
	return staticDoc, nil
}

// Close 释放抓取器占用的连接资源。
func (f *staticFetcher) Close() error {
	if f.client != nil {
		f.client.CloseIdleConnections()
	}
	return nil
}

// fetchStatic 执行静态抓取，并按抓取礼仪重试。
func (f *staticFetcher) fetchStatic(ctx context.Context, target *url.URL) (*Document, error) {
	rawURL := target.String()
	host := hostKey(target.Host)

	maxRetries := f.cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	var (
		lastErr    error
		retryAfter time.Duration
		status     int
	)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			wait := backoffFor(attempt - 1)
			if retryAfter > wait {
				wait = retryAfter
			}
			if err := sleepContext(ctx, wait); err != nil {
				return nil, apperr.ErrFetchFailed.WithCause(err).WithMessage("抓取被取消：" + rawURL)
			}
		}

		release, err := f.hosts.acquire(ctx, host)
		if err != nil {
			return nil, apperr.ErrFetchFailed.WithCause(err).WithMessage("抓取被取消：" + rawURL)
		}
		doc, code, after, err := f.attempt(ctx, rawURL)
		release()

		if err == nil {
			return doc, nil
		}
		lastErr = err
		status = code
		retryAfter = after

		if !retryableStatus(code) || ctx.Err() != nil {
			break
		}
		slog.Debug("静态抓取失败，准备重试",
			"url", rawURL, "status", code, "attempt", attempt+1, "error", err)
	}

	if lastErr == nil {
		lastErr = errors.New("未知错误")
	}
	msg := "抓取页面失败：" + rawURL
	if status > 0 {
		msg = fmt.Sprintf("抓取页面失败：HTTP %d %s", status, rawURL)
	}
	return nil, apperr.ErrFetchFailed.WithCause(lastErr).WithMessage(msg)
}

// attempt 发起一次静态抓取。
//
// 返回的 status 为 HTTP 状态码（网络层失败时为 0），retryAfter 为站点声明的
// Retry-After（未声明时为 0）。
func (f *staticFetcher) attempt(ctx context.Context, rawURL string) (*Document, int, time.Duration, error) {
	c := colly.NewCollector(
		// 同一个地址可能在重试或不同阶段被再次抓取，需要允许重复访问。
		colly.AllowURLRevisit(),
		colly.MaxBodySize(maxBodyBytes),
		colly.UserAgent(UserAgent(f.identity)),
	)
	// 复用同一个 http.Client，保留连接池。
	c.SetClient(f.client)
	// colly 自带的 robots.txt 抓取不会带上我们的身份头，会绕过身份声明；
	// robots 许可统一由调度层的 RobotsGate 判定，这里关闭它的二次抓取。
	c.IgnoreRobotsTxt = true
	// 让请求可以随上游 context 取消。
	c.Context = ctx

	var (
		doc        *Document
		status     int
		retryAfter time.Duration
		attemptErr error
		start      = time.Now()
	)

	// OnRequest 是唯一能覆盖全部请求头的时机：这里用 BuildHeaders 声明身份。
	c.OnRequest(func(r *colly.Request) {
		h := BuildHeaders(f.identity, r.URL)
		// BuildHeaders 声明 gzip, deflate, br。标准库与 colly 只能解 gzip，
		// 若继续声明 br，站点返回的 brotli 字节流无法解码，正文会变成乱码。
		// 因此出站前摘掉 br（deflate 也仅作为降级声明），保证拿到可解析的正文。
		h.Set("Accept-Encoding", "gzip")
		*r.Headers = h
	})

	c.OnResponse(func(r *colly.Response) {
		status = r.StatusCode
		finalURL := rawURL
		if r.Request != nil && r.Request.URL != nil {
			// colly 在跟随重定向后会把请求 URL 更新为最终地址。
			finalURL = r.Request.URL.String()
		}
		doc = &Document{
			URL:        rawURL,
			FinalURL:   finalURL,
			StatusCode: r.StatusCode,
			HTML:       string(r.Body),
			Header:     copyHeader(r.Headers),
			FetchedAt:  time.Now(),
			Elapsed:    time.Since(start),
			// NoAI 由抽取/调度层通过 DetectNoAI 判断，这里保持 false 表示未判定。
			NoAI: false,
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		if r != nil {
			status = r.StatusCode
			if r.Headers != nil {
				retryAfter = parseRetryAfter(*r.Headers)
			}
		}
		if err != nil {
			attemptErr = err
		}
	})

	if err := c.Visit(rawURL); err != nil && attemptErr == nil {
		attemptErr = err
	}

	switch {
	case attemptErr != nil:
		return nil, status, retryAfter, attemptErr
	case doc == nil:
		return nil, status, retryAfter, errors.New("未取得响应内容")
	case status >= 400:
		return nil, status, retryAfter, fmt.Errorf("HTTP %d", status)
	default:
		return doc, status, retryAfter, nil
	}
}

// retryableStatus 判断某个状态码是否值得重试。
//
// 只重试网络层错误（status==0）、429、408 与 5xx；其余 4xx 说明请求本身
// 有问题（404 不存在、403 明确拒绝），重试只会给对方添麻烦。
func retryableStatus(status int) bool {
	switch {
	case status == 0:
		return true
	case status == http.StatusTooManyRequests, status == http.StatusRequestTimeout:
		return true
	case status >= 500 && status <= 599:
		return true
	default:
		return false
	}
}

// backoffFor 返回第 n 次重试前的等待时长：1s、2s、4s……上限 30s。
func backoffFor(n int) time.Duration {
	if n < 0 {
		n = 0
	}
	if n > 30 {
		n = 30
	}
	d := backoffBase << n
	if d > backoffMax || d <= 0 {
		d = backoffMax
	}
	return d
}

// parseRetryAfter 解析 429/503 响应中的 Retry-After（秒数或 HTTP 日期）。
func parseRetryAfter(h http.Header) time.Duration {
	if h == nil {
		return 0
	}
	raw := strings.TrimSpace(h.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	var d time.Duration
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs <= 0 {
			return 0
		}
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(raw); err == nil {
		d = time.Until(t)
		if d <= 0 {
			return 0
		}
	} else {
		return 0
	}
	if d > maxRetryAfter {
		d = maxRetryAfter
	}
	return d
}

// copyHeader 复制响应头，避免上层拿到 nil 指针。
func copyHeader(h *http.Header) http.Header {
	if h == nil {
		return http.Header{}
	}
	return h.Clone()
}

// —— 无头浏览器渲染器 ——

// NewRenderer 按配置构造无头浏览器渲染器；未启用时返回 nil, nil。
//
// 找不到可用浏览器时返回明确错误，调用方据此降级为纯静态抓取。
func NewRenderer(cfg config.Headless) (Renderer, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	execPath, err := resolveBrowserPath(cfg.ExecutablePath)
	if err != nil {
		return nil, err
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultHeadlessTimeout
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 1
	}

	return &chromeRenderer{
		cfg:      cfg,
		execPath: execPath,
		sem:      make(chan struct{}, cfg.MaxConcurrent),
		// 默认身份：NewFetcher 会用配置里的 Identity 覆盖它。
		identity: config.Identity{BotName: "JobsRobotBot", BotVersion: "1.0", IsAI: true},
	}, nil
}

// chromeRenderer 基于 chromedp 的无头渲染器。
//
// 浏览器进程在首次渲染时惰性启动并被后续渲染复用；每个页面开一个独立标签页，
// 通过信号量限制并发渲染数。这样既避免反复冷启动浏览器，也不会让并发标签页
// 互相干扰。
type chromeRenderer struct {
	cfg      config.Headless
	execPath string
	sem      chan struct{}

	mu          sync.Mutex
	identity    config.Identity
	allocCancel context.CancelFunc
	browserCtx  context.Context
	browserStop context.CancelFunc
	closed      bool
}

// setIdentity 注入配置中的机器人身份，供无头浏览器声明 User-Agent 与 X-Robot-* 头。
func (r *chromeRenderer) setIdentity(id config.Identity) {
	if strings.TrimSpace(id.BotName) == "" {
		return
	}
	r.mu.Lock()
	r.identity = id
	r.mu.Unlock()
}

// Render 用无头浏览器打开页面并等待渲染完成。
func (r *chromeRenderer) Render(ctx context.Context, rawURL string) (*Document, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, apperr.ErrFetchFailed.WithMessage("无头渲染地址为空")
	}

	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, apperr.ErrFetchFailed.WithCause(ctx.Err()).WithMessage("无头渲染被取消")
	}
	defer func() { <-r.sem }()

	browserCtx, err := r.ensureBrowser()
	if err != nil {
		return nil, apperr.ErrFetchFailed.WithCause(err).WithMessage("无头渲染不可用")
	}

	// 每个页面一个独立标签页；标签页级超时，避免个别页面卡死拖垮整个浏览器。
	tabCtx, stopTab := chromedp.NewContext(browserCtx)
	defer stopTab()
	runCtx, stopRun := context.WithTimeout(tabCtx, r.cfg.Timeout)
	defer stopRun()

	r.mu.Lock()
	identity := r.identity
	r.mu.Unlock()

	start := time.Now()
	var (
		resp *network.Response
		html string
	)

	action := chromedp.Action[chromedp.Void](func(actx context.Context, t *chromedp.Target) (chromedp.Void, error) {
		// 先声明身份头，再做导航，保证主文档请求就带上 X-Robot-*。
		injectIdentityHeaders(actx, t, identity, rawURL)

		var err error
		resp, err = chromedp.NavigateResponse(rawURL)(actx, t)
		if err != nil {
			return chromedp.Void{}, err
		}
		// NavigateResponse 已在 load 事件后返回；再等一段可配置的稳定时间，
		// 给 SPA 的异步接口首屏渲染留出余量（等价于 networkIdle 的简化做法）。
		if r.cfg.WaitAfterLoad > 0 {
			if _, err := chromedp.Sleep(r.cfg.WaitAfterLoad)(actx, t); err != nil {
				return chromedp.Void{}, err
			}
		}
		html, err = chromedp.Evaluate[string](`document.documentElement.outerHTML`)(actx, t)
		return chromedp.Void{}, err
	})

	if err := chromedp.Do(runCtx, action); err != nil {
		return nil, apperr.ErrFetchFailed.WithCause(err).WithMessage("无头渲染失败：" + rawURL)
	}

	doc := &Document{
		URL:       rawURL,
		FinalURL:  rawURL,
		HTML:      html,
		Header:    http.Header{},
		FetchedAt: time.Now(),
		Elapsed:   time.Since(start),
		Headless:  true,
	}
	if resp != nil {
		if resp.URL != "" {
			doc.FinalURL = resp.URL
		}
		doc.StatusCode = int(resp.Status)
		doc.Header = convertNetworkHeaders(resp.Headers)
	}

	if strings.TrimSpace(doc.HTML) == "" {
		return nil, apperr.ErrFetchFailed.WithMessage("无头渲染未取到页面内容：" + rawURL)
	}
	if doc.StatusCode >= 400 {
		return nil, apperr.ErrFetchFailed.
			WithMessage(fmt.Sprintf("无头渲染返回 HTTP %d：%s", doc.StatusCode, rawURL))
	}
	return doc, nil
}

// Close 释放浏览器进程。
func (r *chromeRenderer) Close() error {
	r.mu.Lock()
	cancel := r.allocCancel
	stop := r.browserStop
	r.allocCancel = nil
	r.browserStop = nil
	r.browserCtx = nil
	r.closed = true
	r.mu.Unlock()

	if stop != nil {
		stop()
	}
	if cancel != nil {
		// 取消分配器上下文会等待浏览器进程退出。
		cancel()
	}
	return nil
}

// ensureBrowser 惰性启动浏览器并返回可复用的浏览器上下文。
func (r *chromeRenderer) ensureBrowser() (context.Context, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.browserCtx != nil {
		return r.browserCtx, nil
	}
	if r.closed {
		return nil, errors.New("渲染器已关闭")
	}

	// 注意：NewExecAllocator 不会自动套用 DefaultExecAllocatorOptions，
	// 必须显式复制一份，否则会丢掉 headless 等关键参数。
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.ExecPath(r.execPath),
		chromedp.DisableGPU,
		chromedp.WindowSize(1440, 900),
		chromedp.UserAgent(UserAgent(r.identity)),
	)
	if runtime.GOOS == "linux" {
		// 容器内通常没有可用的内核沙箱能力，加上 no-sandbox 才能启动。
		opts = append(opts, chromedp.NoSandbox)
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, stopBrowser := chromedp.NewContext(allocCtx)

	// 第一次 Run 会真正启动浏览器进程；这里不带超时，避免超时连带杀死浏览器。
	if err := chromedp.Do(browserCtx); err != nil {
		stopBrowser()
		cancelAlloc()
		return nil, fmt.Errorf("启动无头浏览器失败: %w", err)
	}

	r.allocCancel = cancelAlloc
	r.browserCtx = browserCtx
	r.browserStop = stopBrowser
	return browserCtx, nil
}

// injectIdentityHeaders 把 BuildHeaders 生成的身份头注入浏览器，
// 使页面请求（含主文档）在站长日志里同样可被识别。
//
// User-Agent、Accept* 等由浏览器自行管理的头不在这里覆盖；Host 也不能通过
// 该接口设置。注入失败不影响渲染，只记一条告警。
func injectIdentityHeaders(ctx context.Context, t *chromedp.Target, id config.Identity, rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	h := BuildHeaders(id, u)
	extra := network.Headers{}
	for _, name := range []string{
		"From",
		"X-Robot-Name",
		"X-Robot-Version",
		"X-Robot-Contact",
		"X-Robot-Homepage",
		"X-Robot-Purpose",
		"X-Robot-AI",
		"X-Robot-Type",
		"X-Robot-Respect-Robots",
	} {
		if v := strings.TrimSpace(h.Get(name)); v != "" {
			extra[name] = v
		}
	}
	if len(extra) == 0 {
		return
	}
	if _, err := cdp.Call(ctx, t, network.SetExtraHTTPHeaders, network.SetExtraHTTPHeadersParams{Headers: extra}); err != nil {
		slog.Warn("向无头浏览器注入机器人身份头失败，已忽略", "url", rawURL, "error", err)
	}
}

// convertNetworkHeaders 把 CDP 的响应头转成标准 http.Header。
func convertNetworkHeaders(h network.Headers) http.Header {
	out := http.Header{}
	for k, v := range h {
		switch val := v.(type) {
		case string:
			out.Set(k, val)
		case []string:
			for _, item := range val {
				out.Add(k, item)
			}
		case []any:
			for _, item := range val {
				if s, ok := item.(string); ok {
					out.Add(k, s)
				}
			}
		case float64:
			out.Set(k, strconv.FormatFloat(val, 'f', -1, 64))
		case bool:
			out.Set(k, strconv.FormatBool(val))
		}
	}
	return out
}

// —— 浏览器可执行文件探测 ——

// resolveBrowserPath 解析无头浏览器可执行文件路径。
//
// 显式配置优先；未配置时按平台探测常见的 Chrome / Edge / Chromium 安装位置，
// 最后回退到 PATH 查找。全部失败时返回可读错误，提示配置项名称。
func resolveBrowserPath(configured string) (string, error) {
	if p := strings.TrimSpace(configured); p != "" {
		if full, ok := lookupExecutable(p); ok {
			return full, nil
		}
		return "", apperr.ErrFetchFailed.WithMessage(fmt.Sprintf(
			"未找到可用的 Chrome/Edge：crawler.headless.executable_path=%q 指向的文件不存在，请检查配置", p))
	}
	for _, candidate := range browserCandidates() {
		if full, ok := lookupExecutable(candidate); ok {
			return full, nil
		}
	}
	return "", apperr.ErrFetchFailed.WithMessage(
		"未找到可用的 Chrome/Edge，无法启用无头渲染：请安装 Chrome 或 Edge，" +
			"或设置 crawler.headless.executable_path 指向浏览器可执行文件")
}

// lookupExecutable 判断给定路径（或 PATH 中的命令名）是否可用。
func lookupExecutable(path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	// 绝对/相对路径优先直接判断文件是否存在。
	if strings.ContainsAny(path, `/\`) {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	if full, err := exec.LookPath(path); err == nil {
		return full, true
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path, true
	}
	return "", false
}

// browserCandidates 返回各平台常见的浏览器可执行文件候选路径。
func browserCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return windowsBrowserCandidates()
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Microsoft Edge Beta.app/Contents/MacOS/Microsoft Edge Beta",
			"google-chrome",
			"chromium",
			"microsoft-edge",
		}
	default:
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/usr/bin/microsoft-edge",
			"/usr/bin/microsoft-edge-stable",
			"/snap/bin/chromium",
			"/opt/google/chrome/chrome",
			"google-chrome",
			"google-chrome-stable",
			"chromium",
			"chromium-browser",
			"microsoft-edge",
		}
	}
}

// windowsBrowserCandidates 列出 Windows 下常见的 Chrome / Edge / Chromium 安装位置。
func windowsBrowserCandidates() []string {
	var candidates []string
	roots := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("LOCALAPPDATA"),
		os.Getenv("ProgramW6432"),
	}
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		candidates = append(candidates,
			filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(root, "Google", "Chrome Beta", "Application", "chrome.exe"),
			filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(root, "Microsoft", "Edge Beta", "Application", "msedge.exe"),
			filepath.Join(root, "Chromium", "Application", "chrome.exe"),
		)
	}
	// 兜底：交给 PATH 解析（含 scoop / choco / winget 的 shim）。
	return append(candidates,
		"chrome.exe",
		"msedge.exe",
		"chrome",
		"msedge",
	)
}
