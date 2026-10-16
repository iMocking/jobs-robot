// Package bootstrap 统一装配应用依赖，供 cmd 下的各可执行文件复用。
//
// 这样 api / worker / seed 三个入口共享同一套初始化与校验逻辑，
// 不会出现「某个入口忘了建表或忘了播种配置」这类环境差异。
package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/casbin"
	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/crawler"
	"github.com/jsong/jobs-robot/internal/mail"
	"github.com/jsong/jobs-robot/internal/pkg/logger"
	"github.com/jsong/jobs-robot/internal/pkg/secret"
	"github.com/jsong/jobs-robot/internal/service"
	"github.com/jsong/jobs-robot/internal/store"
)

// App 聚合已初始化的全部依赖。
type App struct {
	Cfg      *config.Config
	Store    *store.Store
	Authz    *casbin.Authorizer
	Mailer   mail.Mailer
	JWT      *auth.JWTManager
	Codes    *auth.CodeManager
	Sessions *auth.SessionManager
	Crawler  *crawler.Crawler
	Services *service.Services

	logCloser io.Closer
}

// Options 控制初始化行为。
type Options struct {
	// Migrate 是否执行数据库迁移。
	Migrate bool
	// WithCrawler 是否初始化爬虫（seed 等命令不需要）。
	WithCrawler bool
	// SeedSettings 是否播种内置配置项。
	SeedSettings bool
	// EnsureAdmin 是否在启动时保证存在一个可登录的管理员账号。
	//
	// api / worker 置 true（全新部署必须能进后台配 SMTP）；
	// cmd/seed 置 false —— 它按自己的显式参数创建管理员，不应被自举逻辑抢先。
	EnsureAdmin bool
}

// DefaultOptions 是常规启动参数。
func DefaultOptions() Options {
	return Options{Migrate: true, WithCrawler: true, SeedSettings: true, EnsureAdmin: true}
}

// Init 按顺序初始化配置、日志、存储、授权、邮件、认证与（可选）爬虫。
//
// 任一步失败都会清理已获取的资源并返回错误，避免进程带着半个环境继续跑。
func Init(ctx context.Context, configPath string, opts Options) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	if loc, err := time.LoadLocation(cfg.App.Timezone); err == nil {
		time.Local = loc
	}

	_, closer, err := logger.Init(logger.Options{
		Level:  cfg.Log.Level,
		Format: cfg.Log.Format,
		Output: cfg.Log.Output,
	})
	if err != nil {
		return nil, err
	}

	app := &App{Cfg: cfg, logCloser: closer}

	st, err := store.Open(ctx, cfg)
	if err != nil {
		app.cleanup()
		return nil, err
	}
	app.Store = st

	if opts.Migrate && cfg.Database.AutoMigrate {
		if err := st.Migrate(ctx); err != nil {
			app.cleanup()
			return nil, err
		}
	}

	authz, err := casbin.New(ctx, st.SQL())
	if err != nil {
		app.cleanup()
		return nil, err
	}
	app.Authz = authz

	// 邮件配置：以配置文件为基线，被数据库中的后台设置覆盖。
	// 必须在构造 Services 之前解析，因为投递器要在启动时就装载好。
	mailCfg, mailFromDB, err := service.ResolveMailConfig(ctx, st, cfg)
	if err != nil {
		// 配置损坏（例如主密钥变更导致密码解不开）不应阻止服务启动：
		// 回落配置文件并告警，管理员可在后台重新填写。
		slog.Warn("数据库中的邮件配置不可用，已回落到配置文件", "error", err)
		mailCfg = cfg.Mail
		mailFromDB = false
	}
	mailer, err := mail.New(mailCfg)
	if err != nil {
		app.cleanup()
		return nil, err
	}
	// 用可热替换的包装器承载投递器：后台修改邮件配置后立即生效。
	dynamicMailer := mail.NewDynamic(mailer)
	app.Mailer = dynamicMailer
	slog.Info("邮件投递器已装载",
		"driver", dynamicMailer.Name(),
		"source", map[bool]string{true: "database", false: "config"}[mailFromDB],
	)

	app.JWT = auth.NewJWTManager(cfg.JWT)
	app.Codes = auth.NewCodeManager(st.Redis, auth.CodeOptions{
		KeyPrefix: cfg.Redis.KeyPrefix,
		// 验证码摘要使用独立的派生密钥，避免与 JWT 密钥共用同一份材料。
		Secret:      secret.DeriveKey(cfg.JWT.Secret, "verification-code"),
		Length:      cfg.Auth.CodeLength,
		TTL:         cfg.Auth.CodeTTL,
		Cooldown:    cfg.Auth.CodeSendCooldown,
		MaxAttempts: cfg.Auth.CodeMaxAttempts,
		DailyLimit:  cfg.Auth.CodeDailyLimit,
	})
	app.Sessions = auth.NewSessionManager(st.Redis, cfg.Redis.KeyPrefix, app.JWT, cfg.JWT.RefreshTTL)

	app.Services = service.New(service.Params{
		Cfg:      cfg,
		Store:    st,
		Authz:    authz,
		Mailer:   dynamicMailer,
		Codes:    app.Codes,
		Sessions: app.Sessions,
	})

	if opts.SeedSettings {
		if err := app.Services.Setting.EnsureDefaults(ctx, service.SystemActor()); err != nil {
			app.cleanup()
			return nil, err
		}
		// 首次启动把配置文件中的邮件配置落库，之后以数据库为准（后台可在线修改）。
		if err := app.Services.Mail.EnsureDefaults(ctx, service.SystemActor()); err != nil {
			app.cleanup()
			return nil, err
		}
	}

	// 自动自举初始管理员：必须在构造爬虫之前完成，让「HTTP 服务已启动」的
	// 日志里一定已经能看到可登录凭据（运维只看启动输出就能进后台）。
	//
	// 这里不判断 cfg.Auth.Bootstrap.Enabled：那个开关由服务内部统一裁决
	// （EnsureBootstrapAdmin 读同一个 Bootstrap 配置），避免出现
	// 「入口不查开关、服务查」或反之导致行为不一致。
	//
	// 失败即启动失败：没有账号就配不了 SMTP，带着这个状态继续跑只会让
	// 运维面对一个「能访问但永远登不进去」的系统，比直接报错更难排查。
	if opts.EnsureAdmin {
		if _, err := app.Services.EnsureBootstrapAdmin(ctx, cfg.Auth.Bootstrap); err != nil {
			app.cleanup()
			return nil, fmt.Errorf("自举初始管理员失败: %w", err)
		}
	}

	if opts.WithCrawler {
		// 收集范围以「数据库配置覆盖静态配置」的合并结果为准。
		scope, err := app.Services.Setting.CrawlScope(ctx)
		if err != nil {
			app.cleanup()
			return nil, err
		}
		cr, err := crawler.New(crawler.Options{Config: cfg.Crawler, Scope: scope})
		if err != nil {
			// 爬虫初始化失败不阻塞 API 启动（例如仅前台查询场景），但必须显式告警。
			if cfg.Crawler.Enabled {
				app.cleanup()
				return nil, fmt.Errorf("初始化爬虫失败: %w", err)
			}
		} else {
			app.Crawler = cr
			app.Services.Crawler = cr
		}
	}

	return app, nil
}

// Close 释放全部资源。
func (a *App) Close() {
	if a.Crawler != nil {
		_ = a.Crawler.Close()
	}
	a.cleanup()
}

func (a *App) cleanup() {
	if a.Store != nil {
		_ = a.Store.Close()
		a.Store = nil
	}
	if a.logCloser != nil {
		_ = a.logCloser.Close()
		a.logCloser = nil
	}
}
