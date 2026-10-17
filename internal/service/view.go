package service

import (
	"time"

	"github.com/jsong/jobs-robot/internal/ent"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
)

// 本文件定义对外 API 的视图模型（DTO）。
//
// 为什么不直接序列化 ent 实体：实体包含 content_hash、raw_json、password_hash
// 等内部字段，直接暴露会同时造成信息泄漏与契约不稳定（实体随 schema 变化）。
// 视图层是前后端之间的正式契约，UI 设计稿到位后只需在此基础上增删字段。

// —— 用户 ——

// UserView 是用户信息视图。
type UserView struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Nickname      string `json:"nickname"`
	AvatarURL     string `json:"avatar_url"`
	Phone         string `json:"phone,omitempty"`
	Status        string `json:"status"`
	EmailVerified bool   `json:"email_verified"`
	// Kind 区分真人账号与系统机器身份（human/system）；前端据此展示「系统」标记
	// 并隐藏禁用、删除、改角色、重置密码等操作入口。
	Kind        string     `json:"kind"`
	SystemKey   string     `json:"system_key"`
	Roles       []string   `json:"roles"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	Remark      string     `json:"remark,omitempty"`
	// MustChangePassword 为 true 时前端应在登录后提示「请修改初始密码」。
	// 该标记由「首次启动自举的管理员」或「被提升为管理员的既有账号」置位，
	// 用户成功改密后自动清零。
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// NewUserView 组装用户视图。
func NewUserView(u *ent.User, roles []string) UserView {
	if roles == nil {
		roles = []string{}
	}
	v := UserView{
		ID:                 u.ID,
		Email:              u.Email,
		Nickname:           u.Nickname,
		AvatarURL:          u.AvatarURL,
		Status:             string(u.Status),
		EmailVerified:      u.EmailVerified,
		Kind:               string(u.Kind),
		SystemKey:          u.SystemKey,
		Roles:              roles,
		LastLoginAt:        u.LastLoginAt,
		Remark:             u.Remark,
		MustChangePassword: u.MustChangePassword,
		CreatedAt:          u.CreateTime,
		UpdatedAt:          u.UpdateTime,
	}
	if u.Phone != nil {
		v.Phone = *u.Phone
	}
	return v
}

// SessionView 是登录会话视图。
type SessionView struct {
	ID        string    `json:"id"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RoleView 是角色视图（角色元数据 + 该角色权限策略）。
type RoleView struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	Description string     `json:"description"`
	IsSystem    bool       `json:"is_system"`
	Level       int        `json:"level"`
	Permissions [][]string `json:"permissions,omitempty"`
	UserCount   int        `json:"user_count,omitempty"`
}

// NewRoleView 组装角色视图。
func NewRoleView(r *ent.Role) RoleView {
	return RoleView{
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Description: r.Description,
		IsSystem:    r.IsSystem,
		Level:       r.Level,
	}
}

// —— 公司 ——

// CompanyBriefView 是公司简要信息，用于岗位卡片等嵌入场景。
type CompanyBriefView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name,omitempty"`
	LogoURL   string `json:"logo_url,omitempty"`
	Industry  string `json:"industry,omitempty"`
	Location  string `json:"location,omitempty"`
	Verified  bool   `json:"verified"`
}

// NewCompanyBriefView 组装公司简要视图。
func NewCompanyBriefView(c *ent.Company) CompanyBriefView {
	return CompanyBriefView{
		ID:        c.ID,
		Name:      c.Name,
		ShortName: c.ShortName,
		LogoURL:   c.LogoURL,
		Industry:  c.Industry,
		Location:  c.Location,
		Verified:  c.Verified,
	}
}

// CompanyView 是公司完整视图（管理端使用）。
type CompanyView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ShortName    string `json:"short_name"`
	Website      string `json:"website"`
	CareersURL   string `json:"careers_url"`
	DiscoveryURL string `json:"discovery_url"`
	LogoURL      string `json:"logo_url"`
	Industry     string `json:"industry"`
	SubIndustry  string `json:"sub_industry"`
	Scale        string `json:"scale"`
	Nature       string `json:"nature"`
	Location     string `json:"location"`
	Description  string `json:"description"`
	ContactEmail string `json:"contact_email"`
	ContactPhone string `json:"contact_phone"`

	Enabled  bool `json:"enabled"`
	Verified bool `json:"verified"`
	Priority int  `json:"priority"`

	CrawlEnabled         bool           `json:"crawl_enabled"`
	CrawlIntervalSeconds int            `json:"crawl_interval_seconds"`
	CrawlRules           map[string]any `json:"crawl_rules,omitempty"`
	ForceHeadless        bool           `json:"force_headless"`
	RespectRobots        bool           `json:"respect_robots"`
	LastCrawledAt        *time.Time     `json:"last_crawled_at,omitempty"`
	LastCrawlStatus      string         `json:"last_crawl_status,omitempty"`
	LastCrawlError       string         `json:"last_crawl_error,omitempty"`
	JobCount             int            `json:"job_count"`
	RobotsAllowed        bool           `json:"robots_allowed"`
	RobotsCheckedAt      *time.Time     `json:"robots_checked_at,omitempty"`

	Source    string    `json:"source"`
	Remark    string    `json:"remark,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewCompanyView 组装公司完整视图。
func NewCompanyView(c *ent.Company) CompanyView {
	return CompanyView{
		ID:                   c.ID,
		Name:                 c.Name,
		ShortName:            c.ShortName,
		Website:              c.Website,
		CareersURL:           c.CareersURL,
		DiscoveryURL:         c.DiscoveryURL,
		LogoURL:              c.LogoURL,
		Industry:             c.Industry,
		SubIndustry:          c.SubIndustry,
		Scale:                c.Scale,
		Nature:               c.Nature,
		Location:             c.Location,
		Description:          c.Description,
		ContactEmail:         c.ContactEmail,
		ContactPhone:         c.ContactPhone,
		Enabled:              c.Enabled,
		Verified:             c.Verified,
		Priority:             c.Priority,
		CrawlEnabled:         c.CrawlEnabled,
		CrawlIntervalSeconds: c.CrawlIntervalSeconds,
		CrawlRules:           c.CrawlRules,
		ForceHeadless:        c.ForceHeadless,
		RespectRobots:        c.RespectRobots,
		LastCrawledAt:        c.LastCrawledAt,
		LastCrawlStatus:      c.LastCrawlStatus,
		LastCrawlError:       c.LastCrawlError,
		JobCount:             c.JobCount,
		RobotsAllowed:        c.RobotsAllowed,
		RobotsCheckedAt:      c.RobotsCheckedAt,
		Source:               c.Source,
		Remark:               c.Remark,
		CreatedAt:            c.CreateTime,
		UpdatedAt:            c.UpdateTime,
	}
}

// —— 岗位 ——

// JobListView 是岗位列表项视图（信息密度优先，不含长文本）。
type JobListView struct {
	ID       string           `json:"id"`
	Title    string           `json:"title"`
	Company  CompanyBriefView `json:"company"`
	Category string           `json:"category,omitempty"`
	Industry string           `json:"industry,omitempty"`

	EmploymentType string   `json:"employment_type,omitempty"`
	City           string   `json:"city,omitempty"`
	District       string   `json:"district,omitempty"`
	SalaryText     string   `json:"salary_text,omitempty"`
	SalaryMin      *int     `json:"salary_min,omitempty"`
	SalaryMax      *int     `json:"salary_max,omitempty"`
	Experience     string   `json:"experience,omitempty"`
	Education      string   `json:"education,omitempty"`
	Tags           []string `json:"tags,omitempty"`

	Summary       string     `json:"summary,omitempty"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
	LastUpdatedAt time.Time  `json:"last_updated_at"`
	LastSeenAt    time.Time  `json:"last_seen_at"`

	IsDirectHire bool   `json:"is_direct_hire"`
	ApplyURL     string `json:"apply_url,omitempty"`
	SourceURL    string `json:"source_url"`
	SourceSite   string `json:"source_site,omitempty"`
	// CreatedBy 是创建者用户 ULID：抓取入库为爬虫系统身份，后台手工创建为管理员。
	CreatedBy *string `json:"created_by,omitempty"`

	Status       string `json:"status"`
	QualityScore int    `json:"quality_score"`
	ViewCount    int    `json:"view_count"`
}

// NewJobListView 组装岗位列表项视图。
func NewJobListView(j *ent.JobPosting) JobListView {
	v := JobListView{
		ID:             j.ID,
		Title:          j.Title,
		Category:       j.Category,
		Industry:       j.Industry,
		EmploymentType: j.EmploymentType,
		City:           j.City,
		District:       j.District,
		SalaryText:     j.SalaryText,
		SalaryMin:      j.SalaryMin,
		SalaryMax:      j.SalaryMax,
		Experience:     j.Experience,
		Education:      j.Education,
		Tags:           j.Tags,
		Summary:        j.Summary,
		PublishedAt:    j.PublishedAt,
		LastUpdatedAt:  j.LastUpdatedAt,
		LastSeenAt:     j.LastSeenAt,
		IsDirectHire:   j.IsDirectHire,
		ApplyURL:       j.ApplyURL,
		SourceURL:      j.SourceURL,
		SourceSite:     j.SourceSite,
		CreatedBy:      j.CreatedBy,
		Status:         string(j.Status),
		QualityScore:   j.QualityScore,
		ViewCount:      j.ViewCount,
	}
	if c := j.Edges.Company; c != nil {
		v.Company = NewCompanyBriefView(c)
	}
	return v
}

// JobDetailView 是岗位详情视图，包含用户要求的全部关键信息：
// 任职要求、岗位介绍、发布时间、联系方式、工作地址、最后更新时间。
type JobDetailView struct {
	JobListView

	JobCode          string   `json:"job_code,omitempty"`
	Department       string   `json:"department,omitempty"`
	Headcount        *int     `json:"headcount,omitempty"`
	WorkMode         string   `json:"work_mode,omitempty"`
	SalaryMonths     *int     `json:"salary_months,omitempty"`
	Country          string   `json:"country,omitempty"`
	Province         string   `json:"province,omitempty"`
	Address          string   `json:"address,omitempty"`
	LocationText     string   `json:"location_text,omitempty"`
	Description      string   `json:"description,omitempty"`
	Responsibilities string   `json:"responsibilities,omitempty"`
	Requirements     string   `json:"requirements,omitempty"`
	RequirementsList []string `json:"requirements_list,omitempty"`
	Benefits         string   `json:"benefits,omitempty"`

	ContactPerson string `json:"contact_person,omitempty"`
	ContactEmail  string `json:"contact_email,omitempty"`
	ContactPhone  string `json:"contact_phone,omitempty"`
	ContactWechat string `json:"contact_wechat,omitempty"`
	ApplyMethod   string `json:"apply_method,omitempty"`

	DeadlineAt      *time.Time `json:"deadline_at,omitempty"`
	SourceUpdatedAt *time.Time `json:"source_updated_at,omitempty"`
	CrawledAt       time.Time  `json:"crawled_at"`

	Revision          int     `json:"revision"`
	Extractor         string  `json:"extractor,omitempty"`
	ParseConfidence   float64 `json:"parse_confidence"`
	SourceCompanyName string  `json:"source_company_name,omitempty"`

	ReviewerID   *string    `json:"reviewer_id,omitempty"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	RejectReason string     `json:"reject_reason,omitempty"`
	ExpiredAt    *time.Time `json:"expired_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	Revisions []RevisionView `json:"revisions,omitempty"`
}

// NewJobDetailView 组装岗位详情视图；revisions 可为 nil。
func NewJobDetailView(j *ent.JobPosting, revisions []*ent.JobRevision) JobDetailView {
	v := JobDetailView{
		JobListView:       NewJobListView(j),
		JobCode:           j.JobCode,
		Department:        j.Department,
		Headcount:         j.Headcount,
		WorkMode:          j.WorkMode,
		SalaryMonths:      j.SalaryMonths,
		Country:           j.Country,
		Province:          j.Province,
		Address:           j.Address,
		LocationText:      j.LocationText,
		Description:       j.Description,
		Responsibilities:  j.Responsibilities,
		Requirements:      j.Requirements,
		RequirementsList:  j.RequirementsList,
		Benefits:          j.Benefits,
		ContactPerson:     j.ContactPerson,
		ContactEmail:      j.ContactEmail,
		ContactPhone:      j.ContactPhone,
		ContactWechat:     j.ContactWechat,
		ApplyMethod:       j.ApplyMethod,
		DeadlineAt:        j.DeadlineAt,
		SourceUpdatedAt:   j.SourceUpdatedAt,
		CrawledAt:         j.CrawledAt,
		Revision:          j.Revision,
		Extractor:         j.Extractor,
		ParseConfidence:   j.ParseConfidence,
		SourceCompanyName: j.SourceCompanyName,
		ReviewerID:        j.ReviewerID,
		ReviewedAt:        j.ReviewedAt,
		RejectReason:      j.RejectReason,
		ExpiredAt:         j.ExpiredAt,
		CreatedAt:         j.CreateTime,
		UpdatedAt:         j.UpdateTime,
	}
	if len(revisions) > 0 {
		v.Revisions = make([]RevisionView, 0, len(revisions))
		for _, r := range revisions {
			v.Revisions = append(v.Revisions, NewRevisionView(r))
		}
	}
	return v
}

// RevisionView 是岗位内容变更记录视图。
type RevisionView struct {
	ID            string         `json:"id"`
	Revision      int            `json:"revision"`
	ChangedFields []string       `json:"changed_fields,omitempty"`
	ContentHash   string         `json:"content_hash,omitempty"`
	SourceURL     string         `json:"source_url,omitempty"`
	Trigger       string         `json:"trigger"`
	Snapshot      map[string]any `json:"snapshot,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}

// NewRevisionView 组装变更记录视图。
func NewRevisionView(r *ent.JobRevision) RevisionView {
	return RevisionView{
		ID:            r.ID,
		Revision:      r.Revision,
		ChangedFields: r.ChangedFields,
		ContentHash:   r.ContentHash,
		SourceURL:     r.SourceURL,
		Trigger:       r.Trigger,
		Snapshot:      r.Snapshot,
		CreatedAt:     r.CreateTime,
	}
}

// —— 抓取记录 ——

// CrawlRunView 是抓取运行记录视图。
type CrawlRunView struct {
	ID          string `json:"id"`
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name,omitempty"`
	Trigger     string `json:"trigger"`
	// TriggeredBy 是触发者用户 ULID：手动触发为管理员，定时/自动为爬虫系统身份。
	TriggeredBy   *string        `json:"triggered_by,omitempty"`
	Status        string         `json:"status"`
	SkipReason    string         `json:"skip_reason,omitempty"`
	CareersURL    string         `json:"careers_url,omitempty"`
	StartedAt     time.Time      `json:"started_at"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty"`
	DurationMs    int64          `json:"duration_ms"`
	PagesFetched  int            `json:"pages_fetched"`
	JobsFound     int            `json:"jobs_found"`
	JobsCreated   int            `json:"jobs_created"`
	JobsUpdated   int            `json:"jobs_updated"`
	JobsUnchanged int            `json:"jobs_unchanged"`
	JobsFiltered  int            `json:"jobs_filtered"`
	JobsExpired   int            `json:"jobs_expired"`
	UsedHeadless  bool           `json:"used_headless"`
	ErrorMessage  string         `json:"error_message,omitempty"`
	Detail        map[string]any `json:"detail,omitempty"`
}

// NewCrawlRunView 组装抓取运行视图。
func NewCrawlRunView(r *ent.CrawlRun) CrawlRunView {
	v := CrawlRunView{
		ID:            r.ID,
		CompanyID:     r.CompanyID,
		Trigger:       string(r.Trigger),
		TriggeredBy:   r.TriggeredBy,
		Status:        string(r.Status),
		SkipReason:    r.SkipReason,
		CareersURL:    r.CareersURL,
		StartedAt:     r.StartedAt,
		FinishedAt:    r.FinishedAt,
		DurationMs:    r.DurationMs,
		PagesFetched:  r.PagesFetched,
		JobsFound:     r.JobsFound,
		JobsCreated:   r.JobsCreated,
		JobsUpdated:   r.JobsUpdated,
		JobsUnchanged: r.JobsUnchanged,
		JobsFiltered:  r.JobsFiltered,
		JobsExpired:   r.JobsExpired,
		UsedHeadless:  r.UsedHeadless,
		ErrorMessage:  r.ErrorMessage,
		Detail:        r.Detail,
	}
	if c := r.Edges.Company; c != nil {
		v.CompanyName = c.Name
	}
	return v
}

// CrawlLogView 是抓取明细日志视图。
type CrawlLogView struct {
	ID        string         `json:"id"`
	RunID     string         `json:"run_id"`
	Level     string         `json:"level"`
	Stage     string         `json:"stage,omitempty"`
	URL       string         `json:"url,omitempty"`
	Message   string         `json:"message"`
	Context   map[string]any `json:"context,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// NewCrawlLogView 组装抓取日志视图。
func NewCrawlLogView(l *ent.CrawlLog) CrawlLogView {
	return CrawlLogView{
		ID:        l.ID,
		RunID:     l.RunID,
		Level:     string(l.Level),
		Stage:     l.Stage,
		URL:       l.URL,
		Message:   l.Message,
		Context:   l.Context,
		CreatedAt: l.CreateTime,
	}
}

// —— 系统配置 ——

// SettingView 是配置项视图。
type SettingView struct {
	ID          string         `json:"id"`
	Key         string         `json:"key"`
	Value       map[string]any `json:"value"`
	Description string         `json:"description,omitempty"`
	Group       string         `json:"group,omitempty"`
	IsSystem    bool           `json:"is_system"`
	UpdatedBy   string         `json:"updated_by,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// NewSettingView 组装配置项视图。
func NewSettingView(s *ent.Setting) SettingView {
	return SettingView{
		ID:          s.ID,
		Key:         s.Key,
		Value:       s.Value,
		Description: s.Description,
		Group:       s.Group,
		IsSystem:    s.IsSystem,
		UpdatedBy:   s.UpdatedBy,
		UpdatedAt:   s.UpdateTime,
	}
}

// —— 审计 ——

// AuditLogView 是审计日志视图。
type AuditLogView struct {
	ID         string  `json:"id"`
	ActorID    *string `json:"actor_id,omitempty"`
	ActorEmail string  `json:"actor_email,omitempty"`
	// ActorName 在 actor_id 命中系统机器身份时填其昵称（如 "Jobs AI Robot"），
	// 便于日志列表直接显示机器身份；普通用户留空，由前端展示邮箱。
	ActorName  string         `json:"actor_name,omitempty"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource,omitempty"`
	ResourceID string         `json:"resource_id,omitempty"`
	Method     string         `json:"method,omitempty"`
	Path       string         `json:"path,omitempty"`
	StatusCode int            `json:"status_code"`
	Success    bool           `json:"success"`
	IP         string         `json:"ip,omitempty"`
	UserAgent  string         `json:"user_agent,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// NewAuditLogView 组装审计日志视图。
func NewAuditLogView(l *ent.AuditLog) AuditLogView {
	v := AuditLogView{
		ID:         l.ID,
		ActorID:    l.ActorID,
		ActorEmail: l.ActorEmail,
		Action:     l.Action,
		Resource:   l.Resource,
		ResourceID: l.ResourceID,
		Method:     l.Method,
		Path:       l.Path,
		StatusCode: l.StatusCode,
		Success:    l.Success,
		IP:         l.IP,
		UserAgent:  l.UserAgent,
		Detail:     l.Detail,
		CreatedAt:  l.CreateTime,
	}
	// 只有系统身份才回填昵称：普通用户已有 actor_email，再塞昵称会让日志列表
	// 同时出现「邮箱 + 人名」两套标识，反而难扫读。
	if actor := l.Edges.Actor; actor != nil && actor.Kind == entuser.KindSystem {
		v.ActorName = actor.Nickname
	}
	return v
}
