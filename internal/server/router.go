// Package server 负责 HTTP 服务的装配：中间件链、路由分组与优雅退出。
package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
	"github.com/jsong/jobs-robot/internal/server/handler"
	"github.com/jsong/jobs-robot/internal/server/middleware"
	"github.com/jsong/jobs-robot/internal/service"
)

// Deps 是装配路由所需的依赖。
type Deps struct {
	Cfg      *config.Config
	Services *service.Services
	// JWT 用于解析访问令牌（Auth 中间件需要它）。
	JWT *auth.JWTManager
}

// NewRouter 构造 gin 引擎。
//
// 中间件顺序（自上而下）：
// RequestID → Recovery → AccessLog → CORS → 业务鉴权（Auth → RequireAuth → Authorize）。
// 鉴权放在分组上而不是全局，是因为本项目存在公开只读接口
// （岗位列表、岗位详情、字典等），它们不应被登录要求拦住。
func NewRouter(deps Deps) *gin.Engine {
	cfg := deps.Cfg

	switch {
	case cfg.App.IsProduction():
		gin.SetMode(gin.ReleaseMode)
	case cfg.App.Debug:
		gin.SetMode(gin.DebugMode)
	default:
		gin.SetMode(gin.TestMode)
	}

	engine := gin.New()
	engine.RedirectTrailingSlash = false
	engine.HandleMethodNotAllowed = true

	engine.Use(middleware.RequestID())
	engine.Use(middleware.Recovery())
	engine.Use(middleware.AccessLog(cfg.Log.AccessLog))
	if cfg.CORS.Enabled {
		engine.Use(middleware.CORS(cfg.CORS))
	}

	// 探针接口不鉴权、不记访问日志（AccessLog 内部已对探针降噪）。
	h := handler.New(deps.Services)
	engine.GET("/healthz", h.Health)
	engine.GET("/readyz", h.Ready)

	// 未匹配路由与方法的统一响应，保持与业务接口一致的信封结构。
	engine.NoRoute(func(c *gin.Context) {
		response.Fail(c, apperr.ErrNotFound.WithMessage(
			"接口不存在："+c.Request.Method+" "+c.Request.URL.Path))
	})
	engine.NoMethod(func(c *gin.Context) {
		response.Fail(c, apperr.ErrNotFound.WithMessage(
			"请求方法不被支持："+c.Request.Method+" "+c.Request.URL.Path))
	})

	api := engine.Group(cfg.App.APIPrefix)
	rdb := deps.Services.Store.Redis
	keyPrefix := cfg.Redis.KeyPrefix

	// —— 公开接口：注册、登录、找回密码 ——
	//
	// 这些接口是撞库与邮件轰炸的主要目标，因此逐接口配置限流：
	// 发送验证码最严格，登录次之，注册再次之。
	//
	// 阈值来自 auth.rate_limit 配置而非写死：本地联调、自动化测试与生产公网
	// 对认证接口的压力差异极大，写死会导致测试环境取不到验证码，
	// 或生产环境形同不限流。阈值为 0 或 enabled=false 时不限流。
	rl := cfg.Auth.RateLimit
	guard := func(key string, limit int, window time.Duration) gin.HandlerFunc {
		if !rl.Enabled || limit <= 0 || window <= 0 {
			return func(c *gin.Context) { c.Next() }
		}
		return middleware.RateLimit(rdb, keyPrefix, key, limit, window)
	}

	authGroup := api.Group("/auth")
	{
		authGroup.POST("/codes", guard("auth:code", rl.SendCodeLimit, rl.SendCodeWindow), h.SendCode)
		authGroup.POST("/register", guard("auth:register", rl.RegisterLimit, rl.RegisterWindow), h.Register)
		authGroup.POST("/login", guard("auth:login", rl.LoginLimit, rl.LoginWindow), h.Login)
		authGroup.POST("/refresh", h.Refresh)
		authGroup.POST("/logout", h.Logout)
		authGroup.POST("/password/forgot", guard("auth:forgot", rl.ForgotLimit, rl.ForgotWindow), h.ForgotPassword)
		authGroup.POST("/password/reset", guard("auth:reset", rl.ResetLimit, rl.ResetWindow), h.ResetPassword)
	}

	// —— 公开只读接口：浏览已发布岗位 ——
	api.GET("/jobs", h.ListJobs)
	api.GET("/jobs/:id", h.JobDetail)
	api.GET("/jobs/:id/related", h.JobRelatedJobs)
	api.GET("/companies", h.ListCompanies)
	api.GET("/companies/:id", h.CompanyDetail)
	api.GET("/meta/dictionaries", h.MetaDictionaries)

	// 系统初始化状态：前端据此渲染初始化向导（是否已有管理员、注册通道是否可用）。
	// 放在公开分组是因为「还没有任何账号可登录」正是它要回答的场景。
	api.GET("/system/status", h.SystemStatus)

	// 首次安装向导（前端 /setup）。同样必须公开：此时系统里还没有任何账号。
	// 安全性不依赖鉴权，而依赖 service 层的「无可用管理员」判定 + 咨询锁串行化；
	// 这里补上路由级限流——未初始化的窗口期内，不限流就等于把建号与发信能力
	// 开放给任意来源。limit<=0 时 RateLimit 直接放行（视为不限流）。
	api.POST("/system/setup",
		middleware.RateLimit(rdb, keyPrefix, "system:setup",
			cfg.Auth.RateLimit.SetupLimit, cfg.Auth.RateLimit.SetupWindow),
		h.SystemSetup)
	api.POST("/system/setup/mail/test",
		middleware.RateLimit(rdb, keyPrefix, "system:setup-mail",
			cfg.Auth.RateLimit.SetupLimit, cfg.Auth.RateLimit.SetupWindow),
		h.SetupMailTest)

	// —— 登录用户接口 ——
	//
	// Auth 负责解析令牌（无令牌不拦截），RequireAuth 负责强制登录，
	// Authorize 按 Casbin 策略做接口级授权。
	authed := api.Group("")
	authed.Use(middleware.Auth(deps.Services.Sessions, deps.JWT))
	authed.Use(middleware.RequireAuth())
	authed.Use(middleware.Authorize(deps.Services.Authz))
	{
		authed.GET("/me", h.Me)
		authed.PATCH("/me", h.UpdateProfile)
		authed.POST("/me/password", h.ChangePassword)
		authed.GET("/me/sessions", h.Sessions)
		authed.DELETE("/me/sessions/:sid", h.RevokeSession)
	}

	// —— 管理端接口（管理员与审核员，按 Casbin 策略区分权限范围）——
	admin := api.Group("/admin")
	admin.Use(middleware.Auth(deps.Services.Sessions, deps.JWT))
	admin.Use(middleware.RequireAuth())
	admin.Use(middleware.Authorize(deps.Services.Authz))
	{
		admin.GET("/stats", h.AdminStats)

		// 岗位审核
		admin.GET("/jobs", h.AdminListJobs)
		admin.POST("/jobs", h.AdminCreateJob)
		admin.GET("/jobs/:id", h.AdminJobDetail)
		admin.PATCH("/jobs/:id", h.AdminUpdateJob)
		admin.DELETE("/jobs/:id", h.AdminDeleteJob)
		admin.POST("/jobs/:id/review", h.AdminReviewJob)
		admin.POST("/jobs/:id/status", h.AdminSetJobStatus)
		admin.GET("/jobs/:id/revisions", h.AdminJobRevisions)

		// 公司与抓取
		admin.GET("/companies", h.AdminListCompanies)
		admin.POST("/companies", h.AdminCreateCompany)
		admin.GET("/companies/:id", h.AdminCompanyDetail)
		admin.PATCH("/companies/:id", h.AdminUpdateCompany)
		admin.DELETE("/companies/:id", h.AdminDeleteCompany)
		admin.POST("/companies/:id/enabled", h.AdminSetCompanyEnabled)
		admin.POST("/companies/:id/discover", h.AdminDiscoverCareers)
		admin.POST("/companies/:id/crawl", h.AdminTriggerCrawl)
		admin.GET("/crawl-runs", h.AdminListCrawlRuns)
		admin.GET("/crawl-runs/:id", h.AdminCrawlRunDetail)
		admin.POST("/crawl/run-all", h.AdminCrawlAll)

		// 用户与角色
		admin.GET("/users", h.AdminListUsers)
		admin.GET("/users/:id", h.AdminUserDetail)
		admin.DELETE("/users/:id", h.AdminDeleteUser)
		admin.POST("/users/:id/status", h.AdminSetUserStatus)
		admin.POST("/users/:id/roles", h.AdminSetUserRoles)
		admin.POST("/users/:id/reset-password", h.AdminResetUserPassword)
		admin.GET("/roles", h.AdminListRoles)

		// 系统配置（收集范围）
		admin.GET("/settings", h.AdminListSettings)
		admin.GET("/crawl-scope", h.AdminGetCrawlScope)
		admin.PUT("/crawl-scope", h.AdminSaveCrawlScope)
		admin.GET("/settings/:key", h.AdminGetSetting)
		admin.PUT("/settings/:key", h.AdminUpsertSetting)
		admin.DELETE("/settings/:key", h.AdminDeleteSetting)

		// 邮件设置（后台在线配置 SMTP，保存即热生效，并可发送测试邮件）
		admin.GET("/mail", h.AdminGetMailConfig)
		admin.PUT("/mail", h.AdminSaveMailConfig)
		admin.DELETE("/mail", h.AdminRevertMailConfig)
		admin.POST("/mail/test", h.AdminSendTestMail)

		// 审计与授权策略
		admin.GET("/audit-logs", h.AdminListAuditLogs)
		admin.GET("/authz/policies", h.AdminListPolicies)
		admin.POST("/authz/policies", h.AdminAddPolicy)
		admin.DELETE("/authz/policies", h.AdminRemovePolicy)
	}

	return engine
}

// HTTPServer 按配置构造 http.Server。
func HTTPServer(cfg *config.Config, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.Server.Addr(),
		Handler:           h,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
}
