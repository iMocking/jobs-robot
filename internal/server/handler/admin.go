package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/crawler"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
	"github.com/jsong/jobs-robot/internal/service"
)

// 本文件实现管理端接口：公司、用户与角色、系统配置、抓取记录、审计与授权策略。
//
// 所有写操作都会在 service 层自动落审计记录，handler 只负责参数搬运。

// —— 请求体 ——

type companyCreateRequest struct {
	Name                 string         `json:"name" binding:"required"`
	ShortName            string         `json:"short_name"`
	Website              string         `json:"website"`
	CareersURL           string         `json:"careers_url"`
	LogoURL              string         `json:"logo_url"`
	Industry             string         `json:"industry"`
	SubIndustry          string         `json:"sub_industry"`
	Scale                string         `json:"scale"`
	Nature               string         `json:"nature"`
	Location             string         `json:"location"`
	Description          string         `json:"description"`
	ContactEmail         string         `json:"contact_email"`
	ContactPhone         string         `json:"contact_phone"`
	Enabled              *bool          `json:"enabled"`
	Verified             *bool          `json:"verified"`
	Priority             *int           `json:"priority"`
	CrawlEnabled         *bool          `json:"crawl_enabled"`
	CrawlIntervalSeconds *int           `json:"crawl_interval_seconds"`
	CrawlRules           map[string]any `json:"crawl_rules"`
	ForceHeadless        *bool          `json:"force_headless"`
	RespectRobots        *bool          `json:"respect_robots"`
	Remark               string         `json:"remark"`
}

type companyUpdateRequest struct {
	Name                 *string        `json:"name"`
	ShortName            *string        `json:"short_name"`
	Website              *string        `json:"website"`
	CareersURL           *string        `json:"careers_url"`
	LogoURL              *string        `json:"logo_url"`
	Industry             *string        `json:"industry"`
	SubIndustry          *string        `json:"sub_industry"`
	Scale                *string        `json:"scale"`
	Nature               *string        `json:"nature"`
	Location             *string        `json:"location"`
	Description          *string        `json:"description"`
	ContactEmail         *string        `json:"contact_email"`
	ContactPhone         *string        `json:"contact_phone"`
	Enabled              *bool          `json:"enabled"`
	Verified             *bool          `json:"verified"`
	Priority             *int           `json:"priority"`
	CrawlEnabled         *bool          `json:"crawl_enabled"`
	CrawlIntervalSeconds *int           `json:"crawl_interval_seconds"`
	CrawlRules           map[string]any `json:"crawl_rules"`
	ForceHeadless        *bool          `json:"force_headless"`
	RespectRobots        *bool          `json:"respect_robots"`
	Remark               *string        `json:"remark"`
}

type setEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

type setStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type setRolesRequest struct {
	Roles []string `json:"roles" binding:"required,min=1"`
}

type resetUserPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}

type upsertSettingRequest struct {
	Value       map[string]any `json:"value" binding:"required"`
	Description string         `json:"description"`
	Group       string         `json:"group"`
}

type crawlScopeRequest struct {
	JobTypes        []string `json:"job_types"`
	Industries      []string `json:"industries"`
	EmploymentTypes []string `json:"employment_types"`
	Locations       []string `json:"locations"`
	Keywords        []string `json:"keywords"`
	ExcludeKeywords []string `json:"exclude_keywords"`
	MinQualityScore int      `json:"min_quality_score"`
}

type policyRequest struct {
	Role string `json:"role" binding:"required"`
	Obj  string `json:"obj" binding:"required"`
	Act  string `json:"act" binding:"required"`
}

// —— 公司管理 ——

// AdminListCompanies 公司列表。
//
// GET /api/v1/admin/companies
func (h *Handler) AdminListCompanies(c *gin.Context) {
	page, err := h.svc.Company.List(c.Request.Context(), service.CompanyListFilter{
		Keyword:      strings.TrimSpace(c.Query("keyword")),
		Industry:     strings.TrimSpace(c.Query("industry")),
		Enabled:      queryBool(c, "enabled"),
		CrawlEnabled: queryBool(c, "crawl_enabled"),
		Verified:     queryBool(c, "verified"),
		OrderBy:      strings.TrimSpace(c.Query("order_by")),
		Page:         queryInt(c, "page", 1),
		PageSize:     queryInt(c, "page_size", 20),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, page)
}

// AdminCompanyDetail 公司详情。
//
// GET /api/v1/admin/companies/:id
func (h *Handler) AdminCompanyDetail(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Company.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminCreateCompany 新增公司。
//
// POST /api/v1/admin/companies
func (h *Handler) AdminCreateCompany(c *gin.Context) {
	var req companyCreateRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Company.Create(c.Request.Context(), service.CompanyCreateInput{
		Name:                 req.Name,
		ShortName:            req.ShortName,
		Website:              req.Website,
		CareersURL:           req.CareersURL,
		LogoURL:              req.LogoURL,
		Industry:             req.Industry,
		SubIndustry:          req.SubIndustry,
		Scale:                req.Scale,
		Nature:               req.Nature,
		Location:             req.Location,
		Description:          req.Description,
		ContactEmail:         req.ContactEmail,
		ContactPhone:         req.ContactPhone,
		Enabled:              req.Enabled,
		Verified:             req.Verified,
		Priority:             req.Priority,
		CrawlEnabled:         req.CrawlEnabled,
		CrawlIntervalSeconds: req.CrawlIntervalSeconds,
		CrawlRules:           req.CrawlRules,
		ForceHeadless:        req.ForceHeadless,
		RespectRobots:        req.RespectRobots,
		Remark:               req.Remark,
	}, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, view)
}

// AdminUpdateCompany 更新公司。
//
// PATCH /api/v1/admin/companies/:id
func (h *Handler) AdminUpdateCompany(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	var req companyUpdateRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Company.Update(c.Request.Context(), id, service.CompanyUpdateInput{
		Name:                 req.Name,
		ShortName:            req.ShortName,
		Website:              req.Website,
		CareersURL:           req.CareersURL,
		LogoURL:              req.LogoURL,
		Industry:             req.Industry,
		SubIndustry:          req.SubIndustry,
		Scale:                req.Scale,
		Nature:               req.Nature,
		Location:             req.Location,
		Description:          req.Description,
		ContactEmail:         req.ContactEmail,
		ContactPhone:         req.ContactPhone,
		Enabled:              req.Enabled,
		Verified:             req.Verified,
		Priority:             req.Priority,
		CrawlEnabled:         req.CrawlEnabled,
		CrawlIntervalSeconds: req.CrawlIntervalSeconds,
		CrawlRules:           req.CrawlRules,
		ForceHeadless:        req.ForceHeadless,
		RespectRobots:        req.RespectRobots,
		Remark:               req.Remark,
	}, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminSetCompanyEnabled 启停公司。
//
// POST /api/v1/admin/companies/:id/enabled
func (h *Handler) AdminSetCompanyEnabled(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	var req setEnabledRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Company.SetEnabled(c.Request.Context(), id, req.Enabled, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminDeleteCompany 删除公司。
//
// DELETE /api/v1/admin/companies/:id
func (h *Handler) AdminDeleteCompany(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	if err := h.svc.Company.Delete(c.Request.Context(), id, h.actor(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.NoContent(c)
}

// AdminDiscoverCareers 自动发现并回填《加入我们》入口。
//
// POST /api/v1/admin/companies/:id/discover
func (h *Handler) AdminDiscoverCareers(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Crawl.DiscoverCareers(c.Request.Context(), id, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminTriggerCrawl 手动触发单个公司的抓取。
//
// POST /api/v1/admin/companies/:id/crawl
func (h *Handler) AdminTriggerCrawl(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Crawl.CrawlCompany(c.Request.Context(), id, "manual", h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminCrawlAll 手动触发全量抓取。
//
// POST /api/v1/admin/crawl/run-all
func (h *Handler) AdminCrawlAll(c *gin.Context) {
	views, err := h.svc.Crawl.CrawlAll(c.Request.Context(), "manual")
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"runs": views, "count": len(views)})
}

// AdminListCrawlRuns 抓取记录列表。
//
// GET /api/v1/admin/crawl-runs
func (h *Handler) AdminListCrawlRuns(c *gin.Context) {
	page, err := h.svc.Crawl.ListRuns(c.Request.Context(), service.CrawlRunListFilter{
		CompanyID: queryString(c, "company_id"),
		Status:    strings.TrimSpace(c.Query("status")),
		Trigger:   strings.TrimSpace(c.Query("trigger")),
		Page:      queryInt(c, "page", 1),
		PageSize:  queryInt(c, "page_size", 20),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, page)
}

// AdminCrawlRunDetail 抓取记录详情（含明细日志）。
//
// GET /api/v1/admin/crawl-runs/:id
func (h *Handler) AdminCrawlRunDetail(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	run, logs, err := h.svc.Crawl.RunDetail(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"run": run, "logs": logs})
}

// —— 用户与角色 ——

// AdminListUsers 用户列表。
//
// GET /api/v1/admin/users
func (h *Handler) AdminListUsers(c *gin.Context) {
	page, err := h.svc.User.List(c.Request.Context(), service.UserListFilter{
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Status:   strings.TrimSpace(c.Query("status")),
		Role:     strings.TrimSpace(c.Query("role")),
		OrderBy:  strings.TrimSpace(c.Query("order_by")),
		Page:     queryInt(c, "page", 1),
		PageSize: queryInt(c, "page_size", 20),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, page)
}

// AdminUserDetail 用户详情。
//
// GET /api/v1/admin/users/:id
func (h *Handler) AdminUserDetail(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.User.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminSetUserStatus 启用/禁用用户。
//
// POST /api/v1/admin/users/:id/status
func (h *Handler) AdminSetUserStatus(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	var req setStatusRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.User.SetStatus(c.Request.Context(), id, req.Status, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminSetUserRoles 设置用户角色。
//
// POST /api/v1/admin/users/:id/roles
func (h *Handler) AdminSetUserRoles(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	var req setRolesRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.User.SetRoles(c.Request.Context(), id, req.Roles, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminResetUserPassword 管理员重置用户密码。
//
// POST /api/v1/admin/users/:id/reset-password
func (h *Handler) AdminResetUserPassword(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	var req resetUserPasswordRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	if err := h.svc.User.ResetPassword(c.Request.Context(), id, req.NewPassword, h.actor(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"message": "密码已重置，该用户需重新登录"})
}

// AdminDeleteUser 删除用户。
//
// DELETE /api/v1/admin/users/:id
func (h *Handler) AdminDeleteUser(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	if err := h.svc.User.Delete(c.Request.Context(), id, h.actor(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.NoContent(c)
}

// AdminListRoles 角色列表（含权限策略与用户数）。
//
// GET /api/v1/admin/roles
func (h *Handler) AdminListRoles(c *gin.Context) {
	views, err := h.svc.User.Roles(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": views})
}

// —— 系统配置 ——

// AdminListSettings 配置列表。
//
// GET /api/v1/admin/settings
func (h *Handler) AdminListSettings(c *gin.Context) {
	views, err := h.svc.Setting.List(c.Request.Context(), strings.TrimSpace(c.Query("group")))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": views})
}

// AdminGetSetting 读取单个配置。
//
// GET /api/v1/admin/settings/:key
func (h *Handler) AdminGetSetting(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		response.Fail(c, apperr.ErrInvalidParams.WithMessage("配置键不能为空"))
		return
	}

	view, err := h.svc.Setting.Get(c.Request.Context(), key)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminUpsertSetting 新建或覆盖配置。
//
// PUT /api/v1/admin/settings/:key
func (h *Handler) AdminUpsertSetting(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		response.Fail(c, apperr.ErrInvalidParams.WithMessage("配置键不能为空"))
		return
	}

	var req upsertSettingRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Setting.Upsert(c.Request.Context(), key, req.Value, req.Description, req.Group, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminDeleteSetting 删除配置。
//
// DELETE /api/v1/admin/settings/:key
func (h *Handler) AdminDeleteSetting(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		response.Fail(c, apperr.ErrInvalidParams.WithMessage("配置键不能为空"))
		return
	}

	if err := h.svc.Setting.Delete(c.Request.Context(), key, h.actor(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.NoContent(c)
}

// AdminGetCrawlScope 读取爬虫收集范围。
//
// GET /api/v1/admin/crawl-scope
func (h *Handler) AdminGetCrawlScope(c *gin.Context) {
	scope, err := h.svc.Setting.CrawlScope(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, scope)
}

// AdminSaveCrawlScope 保存爬虫收集范围（工作类型与行业白名单）。
//
// PUT /api/v1/admin/crawl-scope
func (h *Handler) AdminSaveCrawlScope(c *gin.Context) {
	var req crawlScopeRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Setting.SaveCrawlScope(c.Request.Context(), crawler.Scope{
		JobTypes:        req.JobTypes,
		Industries:      req.Industries,
		EmploymentTypes: req.EmploymentTypes,
		Locations:       req.Locations,
		Keywords:        req.Keywords,
		ExcludeKeywords: req.ExcludeKeywords,
		MinQualityScore: req.MinQualityScore,
	}, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// —— 审计与授权策略 ——

// AdminListAuditLogs 审计日志列表。
//
// GET /api/v1/admin/audit-logs
func (h *Handler) AdminListAuditLogs(c *gin.Context) {
	filter := service.AuditListFilter{
		Action:     strings.TrimSpace(c.Query("action")),
		Resource:   strings.TrimSpace(c.Query("resource")),
		ResourceID: strings.TrimSpace(c.Query("resource_id")),
		Page:       queryInt(c, "page", 1),
		PageSize:   queryInt(c, "page_size", 20),
	}
	if v := queryString(c, "actor_id"); v != "" {
		filter.ActorID = &v
	}
	filter.Success = queryBool(c, "success")

	page, err := h.svc.Audit.List(c.Request.Context(), filter)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, page)
}

// AdminListPolicies 授权策略列表。
//
// GET /api/v1/admin/authz/policies
func (h *Handler) AdminListPolicies(c *gin.Context) {
	policies, err := h.svc.Authz.Policies()
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"policies": policies, "stats": h.svc.Authz.Stats()})
}

// AdminAddPolicy 新增授权策略。
//
// POST /api/v1/admin/authz/policies
func (h *Handler) AdminAddPolicy(c *gin.Context) {
	var req policyRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	if err := h.svc.Authz.AddPolicy(req.Role, req.Obj, req.Act); err != nil {
		response.Fail(c, err)
		return
	}
	h.svc.Audit.Record(c.Request.Context(), h.actor(c), "authz.add_policy", "policy", req.Role,
		map[string]any{"obj": req.Obj, "act": req.Act})
	response.Created(c, gin.H{"role": req.Role, "obj": req.Obj, "act": req.Act})
}

// AdminRemovePolicy 删除授权策略。
//
// 参数优先从查询串读取（role/obj/act），同时兼容 JSON 请求体：
// 部分 HTTP 客户端（例如 Windows PowerShell）不允许在 DELETE 上携带请求体，
// 提供查询参数形式可避免调用方为删除一个策略而绕路。
//
// DELETE /api/v1/admin/authz/policies?role=reviewer&obj=/api/v1/x&act=GET
func (h *Handler) AdminRemovePolicy(c *gin.Context) {
	req := policyRequest{
		Role: strings.TrimSpace(c.Query("role")),
		Obj:  strings.TrimSpace(c.Query("obj")),
		Act:  strings.TrimSpace(c.Query("act")),
	}
	if req.Role == "" || req.Obj == "" || req.Act == "" {
		var body policyRequest
		if err := c.ShouldBindJSON(&body); err == nil {
			if req.Role == "" {
				req.Role = body.Role
			}
			if req.Obj == "" {
				req.Obj = body.Obj
			}
			if req.Act == "" {
				req.Act = body.Act
			}
		}
	}
	if req.Role == "" || req.Obj == "" || req.Act == "" {
		response.Fail(c, apperr.ErrInvalidParams.WithMessage("role、obj、act 均为必填（可用查询参数或请求体传递）"))
		return
	}

	if err := h.svc.Authz.RemovePolicy(req.Role, req.Obj, req.Act); err != nil {
		response.Fail(c, err)
		return
	}
	h.svc.Audit.Record(c.Request.Context(), h.actor(c), "authz.remove_policy", "policy", req.Role,
		map[string]any{"obj": req.Obj, "act": req.Act})
	response.NoContent(c)
}

// —— 邮件设置 ——
//
// 这组接口让管理员在后台页面直接配置 SMTP，而不用改配置文件后重启服务。
// 安全约定：
//   - 响应中的密码永远是占位符（service.MailPasswordMask），明文不出服务端；
//   - 提交时密码留空或原样回传占位符 = 不修改已保存的密码；
//   - 权限仅限管理员（审核员没有对应策略，会被 Casbin 拒绝）。

type mailConfigRequest struct {
	Driver        string `json:"driver" binding:"required,oneof=console smtp"`
	FromName      string `json:"from_name"`
	FromAddress   string `json:"from_address"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	TLS           *bool  `json:"tls"`
	ClearPassword bool   `json:"clear_password"`
}

type mailTestRequest struct {
	To string `json:"to" binding:"required"`
}

// AdminGetMailConfig 读取当前生效的邮件配置。
//
// GET /api/v1/admin/mail
func (h *Handler) AdminGetMailConfig(c *gin.Context) {
	view, err := h.svc.Mail.Get(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminSaveMailConfig 保存在线修改的邮件配置，并立即热生效。
//
// PUT /api/v1/admin/mail
func (h *Handler) AdminSaveMailConfig(c *gin.Context) {
	var req mailConfigRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Mail.Save(c.Request.Context(), service.MailConfigInput{
		Driver:        req.Driver,
		FromName:      req.FromName,
		FromAddress:   req.FromAddress,
		Host:          req.Host,
		Port:          req.Port,
		Username:      req.Username,
		Password:      req.Password,
		TLS:           req.TLS,
		ClearPassword: req.ClearPassword,
	}, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminSendTestMail 使用当前配置发送测试邮件。
//
// POST /api/v1/admin/mail/test
func (h *Handler) AdminSendTestMail(c *gin.Context) {
	var req mailTestRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	if err := h.svc.Mail.SendTest(c.Request.Context(), req.To, h.actor(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"message": "测试邮件已发送至 " + req.To + "，请查收（含垃圾箱）"})
}

// AdminRevertMailConfig 删除后台设置，回落到配置文件中的邮件配置。
//
// DELETE /api/v1/admin/mail
func (h *Handler) AdminRevertMailConfig(c *gin.Context) {
	view, err := h.svc.Mail.Revert(c.Request.Context(), h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// —— 字典、统计与健康检查 ——

// MetaDictionaries 字典数据（工作类型、行业、状态枚举等）。
//
// GET /api/v1/meta/dictionaries
func (h *Handler) MetaDictionaries(c *gin.Context) {
	view, err := h.svc.Meta.Dictionaries(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminStats 管理端概览统计。
//
// GET /api/v1/admin/stats
func (h *Handler) AdminStats(c *gin.Context) {
	view, err := h.svc.Meta.Stats(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// SystemStatus 系统初始化状态（公开接口，无需登录）。
//
// GET /api/v1/system/status
//
// 前端启动时调用：判断是否需要渲染「初始化向导」（尚无可用管理员、
// 注册通道因未配置 SMTP 而走不通、初始凭据文件仍待删除）。
// 该接口不包含任何业务数据，因此放在公开只读分组里。
func (h *Handler) SystemStatus(c *gin.Context) {
	view, err := h.svc.Meta.SystemStatus(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// Health 健康检查（不鉴权，供探针使用）。
//
// GET /healthz
func (h *Handler) Health(c *gin.Context) {
	view := h.svc.Meta.Health(c.Request.Context())
	status := 200
	if view.Status != "ok" {
		status = 503
	}
	c.JSON(status, view)
}

// Ready 就绪检查。
//
// GET /readyz
func (h *Handler) Ready(c *gin.Context) {
	view := h.svc.Meta.Health(c.Request.Context())
	if view.Status != "ok" {
		c.JSON(503, view)
		return
	}
	c.JSON(200, view)
}
