// Package scheduler 负责定时任务的编排：周期性抓取与岗位过期判定。
//
// 独立成包的原因：API 进程与 worker 进程都要能启动它。
// 多副本部署时依赖 Redis 分布式锁（见 service.CrawlService）保证同一公司不被重复抓取。
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/service"
)

// Scheduler 封装 cron 调度器。
type Scheduler struct {
	cron *cron.Cron
	svc  *service.Services
	cfg  *config.Config
}

// New 构造调度器。
//
// 使用 cron.WithSeconds() 之外的标准 5 段表达式（分 时 日 月 周），
// 与 configs/config.yaml 中的 crawler.schedule 保持一致。
func New(svc *service.Services, cfg *config.Config) (*Scheduler, error) {
	return &Scheduler{
		cron: cron.New(
			cron.WithLocation(time.Local),
			cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
		),
		svc: svc,
		cfg: cfg,
	}, nil
}

// Start 注册任务并启动调度。
func (s *Scheduler) Start() error {
	if !s.cfg.Crawler.Enabled {
		slog.Warn("爬虫已在配置中关闭，调度器不会执行抓取任务")
	}

	if s.cfg.Crawler.Enabled {
		spec := s.cfg.Crawler.Schedule
		if spec == "" {
			spec = "0 */6 * * *"
		}
		if _, err := s.cron.AddFunc(spec, s.runCrawlAll); err != nil {
			return err
		}
		slog.Info("已注册定时抓取任务", "schedule", spec)
	}

	// 每天凌晨 3:30 判定岗位是否长期未再出现（已下线）。
	if _, err := s.cron.AddFunc("30 3 * * *", s.runExpire); err != nil {
		return err
	}
	slog.Info("已注册岗位过期判定任务", "schedule", "30 3 * * *")

	s.cron.Start()
	return nil
}

// Stop 停止调度并等待正在执行的任务结束。
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(30 * time.Second):
		slog.Warn("等待调度任务结束超时")
	}
}

// RunCrawlNow 立即执行一轮全量抓取（供命令行手动触发）。
func (s *Scheduler) RunCrawlNow(ctx context.Context) error {
	s.runCrawlAll()
	return nil
}

// runCrawlAll 执行一轮全量抓取。
func (s *Scheduler) runCrawlAll() {
	// 用独立上下文：不与任何请求生命周期绑定。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	started := time.Now()
	slog.Info("开始执行定时抓取")
	runs, err := s.svc.Crawl.CrawlAll(ctx, "schedule")
	if err != nil {
		slog.Error("定时抓取执行失败", "error", err)
		return
	}
	slog.Info("定时抓取完成",
		"runs", len(runs),
		"elapsed", time.Since(started).String(),
	)
}

// runExpire 执行岗位过期判定。
func (s *Scheduler) runExpire() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	n, err := s.svc.Crawl.ExpireStaleJobs(ctx)
	if err != nil {
		slog.Error("岗位过期判定失败", "error", err)
		return
	}
	slog.Info("岗位过期判定完成", "expired", n)
}
