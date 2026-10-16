// Command api 启动「直招好工作」的 HTTP API 服务。
//
// 用法：
//
//	go run ./cmd/api -config configs/config.yaml
//	go run ./cmd/api -scheduler        # 同时在进程内启动定时抓取（单机部署便利模式）
//
// 端口被占用时可以临时换端口，不必改配置文件：
//
//	$env:JOBSROBOT_SERVER_PORT='8091'; go run ./cmd/api
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/jsong/jobs-robot/internal/bootstrap"
	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/scheduler"
	"github.com/jsong/jobs-robot/internal/server"
)

// main 只负责把退出码交给操作系统。
//
// 真正的启动逻辑放在 run() 里，是为了让 defer 一定执行（关闭数据库、Redis、调度器），
// 同时能把「端口被占用」这类启动失败与「收到信号正常退出」区分成不同的退出码——
// 之前的写法在两种情况下都会打印「收到退出信号，开始优雅关闭」，排查时极易误判。
func main() {
	os.Exit(run())
}

func run() int {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径（不存在时使用内置默认值+环境变量）")
	migrate := flag.Bool("migrate", true, "启动时执行数据库迁移")
	withScheduler := flag.Bool("scheduler", false, "在本进程内同时启动定时抓取调度器（生产建议独立运行 cmd/worker）")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := bootstrap.Init(ctx, *configPath, bootstrap.Options{
		Migrate:      *migrate,
		WithCrawler:  true,
		SeedSettings: true,
		// 全新部署必须能立刻登录后台配 SMTP，因此 API 启动时自动保证有管理员。
		EnsureAdmin: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		return 1
	}
	defer app.Close()

	engine := server.NewRouter(server.Deps{
		Cfg:      app.Cfg,
		Services: app.Services,
		JWT:      app.JWT,
	})
	srv := server.HTTPServer(app.Cfg, engine)

	// 可选：进程内调度器
	if *withScheduler {
		sched, err := scheduler.New(app.Services, app.Cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "初始化调度器失败:", err)
			return 1
		}
		if err := sched.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "启动调度器失败:", err)
			return 1
		}
		defer sched.Stop()
		slog.Info("定时抓取调度器已在本进程内启动")
	}

	// 监听放在独立 goroutine，错误经 channel 回到主流程。
	serveErr := make(chan error, 1)
	go func() {
		slog.Info("HTTP 服务已启动",
			"addr", app.Cfg.Server.Addr(),
			"env", app.Cfg.App.Env,
			"api_prefix", app.Cfg.App.APIPrefix,
			"crawler_enabled", app.Cfg.Crawler.Enabled,
			"mail_driver", app.Mailer.Name(),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		reportServeError(app.Cfg.Server, err)
		return 1
	case <-ctx.Done():
		slog.Info("收到退出信号，开始优雅关闭")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), app.Cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("优雅关闭失败", "error", err)
		return 1
	}
	slog.Info("服务已退出")
	return 0
}

// reportServeError 把监听失败翻译成可以直接照做的提示。
//
// 端口占用是本地开发中最常见的启动失败原因，只打印一句原始 errno 往往要查半天；
// 这里直接给出「怎么查占用者」与「怎么临时换端口」两条出路。
func reportServeError(serverCfg config.Server, err error) {
	addr := serverCfg.Addr()
	port := serverCfg.Port

	if isAddrInUse(err) {
		slog.Error("端口已被占用，服务未启动",
			"addr", addr,
			"hint", "该端口上已有进程在监听，通常是上一次启动的实例还没有退出",
			"how_to_check", "netstat -ano | findstr :"+strconv.Itoa(port)+"  然后 taskkill /PID <PID> /F",
			"how_to_change_port", "$env:JOBSROBOT_SERVER_PORT='"+strconv.Itoa(port+1)+"'; go run ./cmd/api",
			"error", err,
		)
		return
	}
	slog.Error("HTTP 服务启动失败", "addr", addr, "error", err)
}

// isAddrInUse 判断错误是否为「端口已被占用」。
//
// 同时匹配 errno 与错误文本：Windows 的报错是
// "Only one usage of each socket address ... is normally permitted"，
// 与类 Unix 的 "address already in use" 完全不同，只判断 errno 容易漏掉。
func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "only one usage of each socket address")
}
