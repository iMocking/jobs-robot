// Command worker 只运行抓取调度与过期判定，不提供 HTTP 接口。
//
// 与 API 分离部署的好处：抓取是长耗时、易受目标站点影响的任务，
// 独立进程可以单独扩容、重启与限流，不会波及线上接口的可用性。
//
// 用法：
//
//	go run ./cmd/worker                     # 按 crawler.schedule 周期执行
//	go run ./cmd/worker -once               # 立即跑一轮全量抓取后退出
//	go run ./cmd/worker -expire             # 只执行一次岗位过期判定后退出
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jsong/jobs-robot/internal/bootstrap"
	"github.com/jsong/jobs-robot/internal/scheduler"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	once := flag.Bool("once", false, "立即执行一轮全量抓取后退出")
	expireOnly := flag.Bool("expire", false, "只执行一次岗位过期判定后退出")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// worker 不负责迁移（由 API 或部署脚本执行），避免多副本同时改表。
	app, err := bootstrap.Init(ctx, *configPath, bootstrap.Options{
		Migrate:      false,
		WithCrawler:  true,
		SeedSettings: true,
		// worker 也可能先于 API 启动（例如只跑抓取的部署），
		// 同样保证系统里有一个可登录的管理员账号。逻辑幂等，不会重复创建。
		EnsureAdmin: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
	defer app.Close()

	sched, err := scheduler.New(app.Services, app.Cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "初始化调度器失败:", err)
		os.Exit(1)
	}

	switch {
	case *expireOnly:
		n, err := app.Services.Crawl.ExpireStaleJobs(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "岗位过期判定失败:", err)
			os.Exit(1)
		}
		slog.Info("岗位过期判定完成", "expired", n)
		return

	case *once:
		if err := sched.RunCrawlNow(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "抓取执行失败:", err)
			os.Exit(1)
		}
		return
	}

	if !app.Cfg.Crawler.Enabled {
		slog.Warn("爬虫已在配置中关闭（crawler.enabled=false），worker 将空转，直接退出")
		return
	}

	if err := sched.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "启动调度器失败:", err)
		os.Exit(1)
	}
	slog.Info("抓取 worker 已启动，等待定时任务触发", "schedule", app.Cfg.Crawler.Schedule)

	<-ctx.Done()
	slog.Info("收到退出信号，等待当前任务结束")
	sched.Stop()
	slog.Info("worker 已退出")
}
