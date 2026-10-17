package service

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/company"
	"github.com/jsong/jobs-robot/internal/ent/crawlrun"
	"github.com/jsong/jobs-robot/internal/ent/jobposting"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// MetaService 提供前端所需的字典、枚举与统计信息。
type MetaService struct{ s *Services }

// MetaOption 是一个可选项（值 + 中文展示名）。
type MetaOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// MetaView 是字典集合，供前端渲染筛选器与状态标签。
type MetaView struct {
	JobTypes        []string     `json:"job_types"`
	Industries      []string     `json:"industries"`
	EmploymentTypes []string     `json:"employment_types"`
	Locations       []string     `json:"locations"`
	JobStatuses     []MetaOption `json:"job_statuses"`
	UserStatuses    []MetaOption `json:"user_statuses"`
	Roles           []MetaOption `json:"roles"`
	CrawlStatuses   []MetaOption `json:"crawl_statuses"`
	CrawlTriggers   []MetaOption `json:"crawl_triggers"`
	CodePurposes    []MetaOption `json:"code_purposes"`
}

// Dictionaries 返回全部字典数据（部分来自数据库配置，会随管理员修改而变化）。
func (svc *MetaService) Dictionaries(ctx context.Context) (MetaView, error) {
	scope, err := svc.s.Setting.CrawlScope(ctx)
	if err != nil {
		return MetaView{}, err
	}

	return MetaView{
		JobTypes:        nonNilSlice(scope.JobTypes),
		Industries:      nonNilSlice(scope.Industries),
		EmploymentTypes: nonNilSlice(scope.EmploymentTypes),
		Locations:       nonNilSlice(scope.Locations),
		JobStatuses: []MetaOption{
			{Value: string(jobposting.StatusPendingReview), Label: "待审核"},
			{Value: string(jobposting.StatusApproved), Label: "已发布"},
			{Value: string(jobposting.StatusRejected), Label: "已驳回"},
			{Value: string(jobposting.StatusOffline), Label: "已下线"},
			{Value: string(jobposting.StatusExpired), Label: "已过期"},
		},
		UserStatuses: []MetaOption{
			{Value: string(entuser.StatusPending), Label: "待验证"},
			{Value: string(entuser.StatusActive), Label: "正常"},
			{Value: string(entuser.StatusDisabled), Label: "已禁用"},
			{Value: string(entuser.StatusLocked), Label: "已锁定"},
		},
		Roles: []MetaOption{
			{Value: "admin", Label: "管理员"},
			{Value: "reviewer", Label: "审核员"},
			{Value: "user", Label: "用户"},
			{Value: "system", Label: "系统"},
		},
		CrawlStatuses: []MetaOption{
			{Value: string(crawlrun.StatusRunning), Label: "执行中"},
			{Value: string(crawlrun.StatusSuccess), Label: "成功"},
			{Value: string(crawlrun.StatusPartial), Label: "部分成功"},
			{Value: string(crawlrun.StatusFailed), Label: "失败"},
			{Value: string(crawlrun.StatusSkipped), Label: "已跳过"},
		},
		CrawlTriggers: []MetaOption{
			{Value: string(crawlrun.TriggerSchedule), Label: "定时任务"},
			{Value: string(crawlrun.TriggerManual), Label: "手动触发"},
			{Value: string(crawlrun.TriggerRetry), Label: "失败重试"},
		},
		CodePurposes: []MetaOption{
			{Value: "register", Label: "注册"},
			{Value: "reset_password", Label: "找回密码"},
			{Value: "change_email", Label: "更换邮箱"},
		},
	}, nil
}

// SystemStatusView 描述系统初始化状态，供前端决定是否展示初始化向导。
//
// 这是一个**公开**接口的返回体，因此只包含「系统是否已就绪」这类
// 不泄露业务数据的信息：没有任何邮箱、口令或抓取内容。
type SystemStatusView struct {
	Initialized bool `json:"initialized"` // 是否存在可用管理员
	UserCount   int  `json:"user_count"`
	AdminCount  int  `json:"admin_count"`
	// SetupAvailable 安装向导当前是否可用（前端据此决定是否强制跳 /setup）。
	//
	// 语义：setup_available = auth.setup.enabled && !initialized。
	// 与 initialized 分开两个字段而不是让前端自己算，是因为「向导是否开启」
	// 属于服务端配置，前端不应该（也无法）去猜。
	SetupAvailable bool `json:"setup_available"`
	// RegistrationOpen 注册功能当前是否真的可用。
	//
	// 语义：registration_open = !auth.register_need_verify || mail.smtp_configured。
	// 即「要求邮箱验证但没配 SMTP 时，注册实际走不通」——验证码发不出去，
	// 用户永远拿不到码。前端据此提示管理员先去后台配置邮件。
	RegistrationOpen bool             `json:"registration_open"`
	Mail             SystemMailStatus `json:"mail"`
	// BootstrapCredentialFileExists 初始凭据文件是否还在（在则提示运维登录后删除）。
	BootstrapCredentialFileExists bool `json:"bootstrap_credential_file_exists"`
}

// SystemMailStatus 描述当前生效的邮件通道状态。
type SystemMailStatus struct {
	Driver string `json:"driver"` // console | smtp
	Source string `json:"source"` // config | database
	Ready  bool   `json:"ready"`  // 当前投递器是否可用
	// SmtpConfigured 是否已配置可用的 SMTP（driver=smtp 且 host/from_address 非空）。
	SmtpConfigured bool `json:"smtp_configured"`
}

// SystemStatus 汇总系统初始化状态。
//
// 为什么不复用 Mail.Get：那是管理端接口，配置损坏时会返回业务错误；
// 而本接口是前端启动时无条件调用的状态查询，必须始终能返回一份可用结果
// （拿不到就如实报告「未就绪」），否则初始化向导本身会因为 500 而白屏。
func (svc *MetaService) SystemStatus(ctx context.Context) (SystemStatusView, error) {
	var out SystemStatusView

	// user_count 是「真实用户数」：系统机器身份（爬虫/邮件机器人）不计入，
	// 否则前端会把它们当成需要运营维护的账号。
	userCount, err := svc.s.Store.Ent.User.Query().
		Where(entuser.KindNEQ(entuser.KindSystem)).
		Count(ctx)
	if err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	out.UserCount = userCount

	availableAdmins, err := svc.s.availableAdminIDs(ctx)
	if err != nil {
		return out, err
	}
	out.AdminCount = len(availableAdmins)
	out.Initialized = len(availableAdmins) > 0
	// 安装向导可用性复用同一套「可用管理员」口径（DecideSetupAvailable 是纯函数，
	// 已单测覆盖）：否则会出现「status 说未初始化、向导说已初始化」的自相矛盾。
	out.SetupAvailable = DecideSetupAvailable(svc.s.Cfg.Auth.Setup.Enabled, len(availableAdmins)).Available

	out.Mail = svc.systemMailStatus(ctx)

	// 注册可用性：要求邮箱验证却没有可用 SMTP 时，注册链路实际是断的。
	out.RegistrationOpen = !svc.s.Cfg.Auth.RegisterNeedVerify || out.Mail.SmtpConfigured

	// 凭据文件是否仍存在：文件还在说明运维尚未完成「登录 → 配 SMTP → 改密 → 删文件」。
	if path := strings.TrimSpace(svc.s.Cfg.Auth.Bootstrap.CredentialFile); path != "" {
		if _, err := os.Stat(path); err == nil {
			out.BootstrapCredentialFileExists = true
		}
	}

	return out, nil
}

// systemMailStatus 组装邮件通道状态；配置损坏时降级为「未就绪」而不是报错。
func (svc *MetaService) systemMailStatus(ctx context.Context) SystemMailStatus {
	m := svc.s.Cfg.Mail
	source := "config"

	if effective, fromDB, err := ResolveMailConfig(ctx, svc.s.Store, svc.s.Cfg); err == nil {
		m = effective
		if fromDB {
			source = "database"
		}
	}

	driver := strings.ToLower(strings.TrimSpace(m.Driver))
	if driver == "" {
		driver = "console"
	}
	smtpConfigured := driver == "smtp" &&
		strings.TrimSpace(m.SMTP.Host) != "" &&
		strings.TrimSpace(m.FromAddress) != ""

	return SystemMailStatus{
		Driver:         driver,
		Source:         source,
		Ready:          svc.s.Mailer != nil && svc.s.Mailer.Current() != nil,
		SmtpConfigured: smtpConfigured,
	}
}

// StatsView 是管理端概览统计。
type StatsView struct {
	Users         UserStats      `json:"users"`
	Jobs          JobStats       `json:"jobs"`
	Companies     CompanyStats   `json:"companies"`
	Crawl         CrawlStats     `json:"crawl"`
	Authorization map[string]int `json:"authorization"`
}

// UserStats 是用户维度统计。
type UserStats struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Disabled int `json:"disabled"`
	Locked   int `json:"locked"`
}

// JobStats 是岗位维度统计。
type JobStats struct {
	Total         int `json:"total"`
	PendingReview int `json:"pending_review"`
	Approved      int `json:"approved"`
	Rejected      int `json:"rejected"`
	Offline       int `json:"offline"`
	Expired       int `json:"expired"`
}

// CompanyStats 是公司维度统计。
type CompanyStats struct {
	Total        int `json:"total"`
	Enabled      int `json:"enabled"`
	CrawlEnabled int `json:"crawl_enabled"`
	Verified     int `json:"verified"`
}

// CrawlStats 是抓取维度统计。
type CrawlStats struct {
	RunsTotal   int `json:"runs_total"`
	RunsRunning int `json:"runs_running"`
	RunsFailed  int `json:"runs_failed"`
	RunsToday   int `json:"runs_today"`
}

// Stats 汇总管理端概览数据。
func (svc *MetaService) Stats(ctx context.Context) (StatsView, error) {
	client := svc.s.Store.Ent
	var out StatsView
	var err error

	// 用户维度统计一律排除系统机器身份，与 /system/status 的 user_count 口径一致。
	humanUsers := func() *ent.UserQuery {
		return client.User.Query().Where(entuser.KindNEQ(entuser.KindSystem))
	}
	if out.Users.Total, err = humanUsers().Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Users.Active, err = humanUsers().Where(entuser.StatusEQ(entuser.StatusActive)).Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Users.Disabled, err = humanUsers().Where(entuser.StatusEQ(entuser.StatusDisabled)).Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Users.Locked, err = humanUsers().Where(entuser.StatusEQ(entuser.StatusLocked)).Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}

	if out.Jobs.Total, err = client.JobPosting.Query().Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	countByStatus := func(s jobposting.Status) (int, error) {
		return client.JobPosting.Query().Where(jobposting.StatusEQ(s)).Count(ctx)
	}
	if out.Jobs.PendingReview, err = countByStatus(jobposting.StatusPendingReview); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Jobs.Approved, err = countByStatus(jobposting.StatusApproved); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Jobs.Rejected, err = countByStatus(jobposting.StatusRejected); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Jobs.Offline, err = countByStatus(jobposting.StatusOffline); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Jobs.Expired, err = countByStatus(jobposting.StatusExpired); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}

	if out.Companies.Total, err = client.Company.Query().Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Companies.Enabled, err = client.Company.Query().Where(company.EnabledEQ(true)).Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Companies.CrawlEnabled, err = client.Company.Query().Where(company.CrawlEnabledEQ(true)).Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Companies.Verified, err = client.Company.Query().Where(company.VerifiedEQ(true)).Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}

	if out.Crawl.RunsTotal, err = client.CrawlRun.Query().Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Crawl.RunsRunning, err = client.CrawlRun.Query().Where(crawlrun.StatusEQ(crawlrun.StatusRunning)).Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Crawl.RunsFailed, err = client.CrawlRun.Query().Where(crawlrun.StatusEQ(crawlrun.StatusFailed)).Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}
	if out.Crawl.RunsToday, err = client.CrawlRun.Query().
		Where(crawlrun.StartedAtGTE(startOfDay(time.Now()))).Count(ctx); err != nil {
		return out, apperr.ErrInternal.WithCause(err)
	}

	out.Authorization = svc.s.Authz.Stats()
	return out, nil
}

// HealthView 是健康检查结果。
type HealthView struct {
	Status   string         `json:"status"`
	Postgres string         `json:"postgres"`
	Redis    string         `json:"redis"`
	Details  map[string]any `json:"details,omitempty"`
}

// Health 检查依赖可用性。
func (svc *MetaService) Health(ctx context.Context) HealthView {
	view := HealthView{Status: "ok", Postgres: "up", Redis: "up"}
	if err := svc.s.Store.Health(ctx); err != nil {
		view.Status = "degraded"
		view.Details = map[string]any{"error": err.Error()}
		// 粗粒度标记：出错时无法区分是哪个依赖，交由 details 说明。
		view.Postgres = "unknown"
		view.Redis = "unknown"
	}
	view.Details = mergeDetails(view.Details, map[string]any{
		"authorization": svc.s.Authz.Stats(),
		"crawler":       svc.s.Cfg.Crawler.Enabled,
	})
	return view
}

func nonNilSlice(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func mergeDetails(base, extra map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any, len(extra))
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}
