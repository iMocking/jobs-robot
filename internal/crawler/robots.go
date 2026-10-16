package crawler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/temoto/robotstxt"

	"github.com/jsong/jobs-robot/internal/config"
)

// robotsEntry 是某个站点 robots.txt 的解析结果缓存项。
type robotsEntry struct {
	group     *robotstxt.Group
	allowAll  bool
	denyAll   bool
	delay     time.Duration
	fetchedAt time.Time
	// ttl 单独存放，便于对「取不到 robots.txt」的情况使用更短缓存。
	ttl time.Duration
}

// robotsGate 拉取并缓存 robots.txt，据此判定抓取许可。
//
// 失败策略（偏保守，宁可少抓也不越界）：
//   - 404/410：站点未声明规则 → 放行
//   - 401/403：明确拒绝 → 禁止
//   - 5xx / 网络错误 / 解析失败：暂时禁止，并用较短 TTL 缓存以便稍后重试
type robotsGate struct {
	identity config.Identity
	ttl      time.Duration
	client   *http.Client

	mu    sync.RWMutex
	cache map[string]*robotsEntry
}

const robotsCacheMaxEntries = 512

func newRobotsGate(cfg config.Crawler) RobotsGate {
	return &robotsGate{
		identity: cfg.Identity,
		ttl:      cfg.RobotsTTL,
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		cache: make(map[string]*robotsEntry),
	}
}

// Allowed 判断目标地址是否允许抓取。
func (g *robotsGate) Allowed(ctx context.Context, target *url.URL, _ string) (bool, error) {
	entry := g.entry(ctx, target)
	switch {
	case entry.allowAll:
		return true, nil
	case entry.denyAll, entry.group == nil:
		return false, nil
	}
	return entry.group.Test(robotsPath(target)), nil
}

// CrawlDelay 返回站点声明的抓取间隔（未声明为 0）。
func (g *robotsGate) CrawlDelay(ctx context.Context, target *url.URL, _ string) (time.Duration, error) {
	return g.entry(ctx, target).delay, nil
}

// robotsPath 返回用于规则匹配的路径（含查询串）。
func robotsPath(target *url.URL) string {
	path := target.EscapedPath()
	if path == "" {
		path = "/"
	}
	if target.RawQuery != "" {
		path += "?" + target.RawQuery
	}
	return path
}

func (g *robotsGate) entry(ctx context.Context, target *url.URL) *robotsEntry {
	key := strings.ToLower(target.Scheme + "://" + target.Host)

	g.mu.RLock()
	cached, ok := g.cache[key]
	g.mu.RUnlock()
	if ok && time.Since(cached.fetchedAt) < cached.ttl {
		return cached
	}

	entry := g.fetch(ctx, target)
	g.mu.Lock()
	if len(g.cache) >= robotsCacheMaxEntries {
		// 简单淘汰：整体清空，避免无界增长。
		g.cache = make(map[string]*robotsEntry, robotsCacheMaxEntries)
	}
	g.cache[key] = entry
	g.mu.Unlock()
	return entry
}

func (g *robotsGate) fetch(ctx context.Context, target *url.URL) *robotsEntry {
	now := time.Now()
	robotsURL := &url.URL{Scheme: target.Scheme, Host: target.Host, Path: "/robots.txt"}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL.String(), nil)
	if err != nil {
		return &robotsEntry{denyAll: true, fetchedAt: now, ttl: 5 * time.Minute}
	}
	for k, vs := range BuildHeaders(g.identity, robotsURL) {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}

	resp, err := g.client.Do(req)
	if err != nil {
		slog.Warn("获取 robots.txt 失败，暂时不抓取该站点", "host", target.Host, "error", err)
		return &robotsEntry{denyAll: true, fetchedAt: now, ttl: 5 * time.Minute}
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		// 未声明 robots.txt，视为放行。
		return &robotsEntry{allowAll: true, fetchedAt: now, ttl: g.ttl}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		slog.Warn("robots.txt 明确拒绝访问", "host", target.Host, "status", resp.StatusCode)
		return &robotsEntry{denyAll: true, fetchedAt: now, ttl: g.ttl}
	case resp.StatusCode >= 400:
		slog.Warn("robots.txt 返回异常状态，暂时不抓取该站点", "host", target.Host, "status", resp.StatusCode)
		return &robotsEntry{denyAll: true, fetchedAt: now, ttl: 5 * time.Minute}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return &robotsEntry{denyAll: true, fetchedAt: now, ttl: 5 * time.Minute}
	}

	data, err := robotstxt.FromBytes(body)
	if err != nil {
		slog.Warn("解析 robots.txt 失败，暂时不抓取该站点", "host", target.Host, "error", err)
		return &robotsEntry{denyAll: true, fetchedAt: now, ttl: 5 * time.Minute}
	}

	group := data.FindGroup(g.identity.BotName)
	entry := &robotsEntry{group: group, fetchedAt: now, ttl: g.ttl}
	if group != nil {
		entry.delay = group.CrawlDelay
	}
	slog.Debug("robots.txt 已解析",
		"host", target.Host,
		"crawl_delay", entry.delay.String(),
		"allow_root", group == nil || group.Test("/"),
	)
	return entry
}

// ensureRobotsDenied 生成统一格式的拒绝说明。
func ensureRobotsDenied(target *url.URL) string {
	return fmt.Sprintf("站点 %s 的 robots.txt 不允许抓取该地址", target.Host)
}
