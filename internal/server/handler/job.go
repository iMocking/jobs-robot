package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
	"github.com/jsong/jobs-robot/internal/service"
)

// jobEditRequest 直接复用 service 的编辑入参。
//
// 用类型别名而非另建 DTO：字段集合一旦不一致就会编译失败，
// 从而避免出现「接口加了字段但 service 没接」这类静默问题。
type jobEditRequest = service.JobEditInput

// jobCreateRequest 是手工新增岗位入参。
type jobCreateRequest struct {
	CompanyID   string `json:"company_id" binding:"required"`
	SourceURL   string `json:"source_url" binding:"required"`
	Title       string `json:"title" binding:"required"`
	AutoApprove bool   `json:"auto_approve"`
	jobEditRequest
}

// reviewRequest 是审核入参。
type reviewRequest struct {
	Decision     string `json:"decision" binding:"required,oneof=approve reject"`
	RejectReason string `json:"reject_reason"`
}

// jobStatusRequest 是状态切换入参。
type jobStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// listJobFilter 从查询参数构造岗位筛选条件，支持前台与后台共用。
func listJobFilter(c *gin.Context) service.JobListFilter {
	filter := service.JobListFilter{
		Keyword:         strings.TrimSpace(c.Query("keyword")),
		Category:        strings.TrimSpace(c.Query("category")),
		Industry:        strings.TrimSpace(c.Query("industry")),
		EmploymentType:  strings.TrimSpace(c.Query("employment_type")),
		Experience:      strings.TrimSpace(c.Query("experience")),
		City:            strings.TrimSpace(c.Query("city")),
		CompanyID:       queryString(c, "company_id"),
		SourceURL:       queryString(c, "source_url"),
		MinQualityScore: nil,
		IsDirectHire:    queryBool(c, "is_direct_hire"),
		OrderBy:         strings.TrimSpace(c.Query("order_by")),
	}
	// 薪资区间筛选（单位：元/月），语义为区间相交。
	if v := queryInt(c, "salary_min", 0); v > 0 {
		filter.SalaryMin = &v
	}
	if v := queryInt(c, "salary_max", 0); v > 0 {
		filter.SalaryMax = &v
	}
	if v := c.Query("min_quality_score"); v != "" {
		score := queryInt(c, "min_quality_score", 0)
		filter.MinQualityScore = &score
	}
	if statuses := strings.TrimSpace(c.Query("status")); statuses != "" {
		for _, s := range strings.Split(statuses, ",") {
			if s = strings.TrimSpace(s); s != "" {
				filter.Statuses = append(filter.Statuses, s)
			}
		}
	}
	filter.Page, filter.PageSize = pagination(c)
	return filter
}

// ListJobs 前台岗位列表（仅已发布）。
//
// GET /api/v1/jobs
func (h *Handler) ListJobs(c *gin.Context) {
	page, err := h.svc.Job.PublicList(c.Request.Context(), listJobFilter(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, page)
}

// JobDetail 前台岗位详情。
//
// GET /api/v1/jobs/:id
func (h *Handler) JobDetail(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Job.PublicDetail(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// JobRelatedJobs 相关职位（详情页底部推荐）。
//
// GET /api/v1/jobs/:id/related?limit=6
func (h *Handler) JobRelatedJobs(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	items, err := h.svc.Job.Related(c.Request.Context(), id, queryInt(c, "limit", 6))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}

// ListCompanies 前台公司列表（仅启用且已核实的直招公司）。
//
// GET /api/v1/companies
func (h *Handler) ListCompanies(c *gin.Context) {
	enabled := true
	page, err := h.svc.Company.List(c.Request.Context(), service.CompanyListFilter{
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Industry: strings.TrimSpace(c.Query("industry")),
		Enabled:  &enabled,
		Page:     queryInt(c, "page", 1),
		PageSize: queryInt(c, "page_size", 20),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	// 前台只暴露对外安全的简要字段，不返回抓取配置等内部信息。
	briefs := make([]service.CompanyBriefView, 0, len(page.Items))
	for _, item := range page.Items {
		briefs = append(briefs, service.CompanyBriefView{
			ID:        item.ID,
			Name:      item.Name,
			ShortName: item.ShortName,
			LogoURL:   item.LogoURL,
			Industry:  item.Industry,
			Location:  item.Location,
			Verified:  item.Verified,
		})
	}
	response.OK(c, response.NewPage(briefs, page.Total, page.Page, page.PageSize))
}

// CompanyDetail 前台公司详情（含该公司已发布岗位入口信息）。
//
// GET /api/v1/companies/:id
func (h *Handler) CompanyDetail(c *gin.Context) {
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
	if !view.Enabled {
		response.Fail(c, apperr.ErrNotFound.WithMessage("公司不存在或已下线"))
		return
	}

	// 前台视图：仅保留对外字段。
	response.OK(c, service.CompanyBriefView{
		ID:        view.ID,
		Name:      view.Name,
		ShortName: view.ShortName,
		LogoURL:   view.LogoURL,
		Industry:  view.Industry,
		Location:  view.Location,
		Verified:  view.Verified,
	})
}

// —— 管理端 ——

// AdminListJobs 管理端岗位列表。
//
// GET /api/v1/admin/jobs
//
// 不传 status 时返回全部状态的岗位：管理端接口遵循「无过滤条件即不过滤」的直觉契约。
// 审核员工作台应显式传 status=pending_review，而不是依赖隐式默认值——
// 否则「列出某公司的全部岗位」这类调用会静默漏掉已发布岗位。
func (h *Handler) AdminListJobs(c *gin.Context) {
	filter := listJobFilter(c)
	page, err := h.svc.Job.AdminList(c.Request.Context(), filter)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, page)
}

// AdminJobDetail 管理端岗位详情。
//
// GET /api/v1/admin/jobs/:id
func (h *Handler) AdminJobDetail(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Job.AdminDetail(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminCreateJob 手工新增岗位。
//
// POST /api/v1/admin/jobs
func (h *Handler) AdminCreateJob(c *gin.Context) {
	var req jobCreateRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Job.Create(c.Request.Context(), service.JobCreateInput{
		CompanyID:    req.CompanyID,
		SourceURL:    req.SourceURL,
		Title:        req.Title,
		AutoApprove:  req.AutoApprove,
		JobEditInput: req.jobEditRequest,
	}, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, view)
}

// AdminUpdateJob 人工编辑岗位。
//
// PATCH /api/v1/admin/jobs/:id
func (h *Handler) AdminUpdateJob(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	var req jobEditRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Job.Update(c.Request.Context(), id, req, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminReviewJob 审核岗位。
//
// POST /api/v1/admin/jobs/:id/review
func (h *Handler) AdminReviewJob(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	var req reviewRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Job.Review(c.Request.Context(), id, service.ReviewInput{
		Decision:     req.Decision,
		RejectReason: req.RejectReason,
	}, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminSetJobStatus 切换岗位状态（上线/下线/标记过期）。
//
// POST /api/v1/admin/jobs/:id/status
func (h *Handler) AdminSetJobStatus(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	var req jobStatusRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Job.SetStatus(c.Request.Context(), id, req.Status, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminJobRevisions 岗位变更历史。
//
// GET /api/v1/admin/jobs/:id/revisions
func (h *Handler) AdminJobRevisions(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	views, err := h.svc.Job.Revisions(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": views})
}

// AdminDeleteJob 删除岗位。
//
// DELETE /api/v1/admin/jobs/:id
func (h *Handler) AdminDeleteJob(c *gin.Context) {
	id, err := h.pathID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}

	if err := h.svc.Job.Delete(c.Request.Context(), id, h.actor(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.NoContent(c)
}
