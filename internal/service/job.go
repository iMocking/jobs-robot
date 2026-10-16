package service

import (
	"context"
	"net/url"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/company"
	"github.com/jsong/jobs-robot/internal/ent/jobposting"
	"github.com/jsong/jobs-robot/internal/ent/jobrevision"
	"github.com/jsong/jobs-robot/internal/ent/predicate"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// JobService 负责岗位的查询、人工编辑、审核与状态流转。
type JobService struct{ s *Services }

// JobListFilter 是岗位列表筛选条件。
type JobListFilter struct {
	Keyword         string
	Category        string
	Industry        string
	EmploymentType  string
	Experience      string
	City            string
	CompanyID       string
	Statuses        []string
	MinQualityScore *int
	IsDirectHire    *bool
	// SourceURL 精确匹配来源地址，用于回答「这个官网链接是否已经采集过」。
	SourceURL     string
	PublishedFrom *time.Time
	PublishedTo   *time.Time
	// SalaryMin / SalaryMax 以「月薪（元）」为单位的区间筛选，语义为**区间相交**：
	// 只要岗位薪资区间与查询区间有交集即命中（前端筛选栏是「25K-40K」这种区间选择）。
	SalaryMin *int
	SalaryMax *int
	OrderBy   string // last_updated_at | published_at | quality_score | created_at
	Page      int
	PageSize  int
}

// JobEditInput 是岗位人工编辑入参；指针为 nil 表示不修改该字段。
//
// 带 json 标签是刻意的：HTTP 层通过类型别名直接复用它做请求绑定，
// 避免为 30 多个字段写一遍机械的 DTO 转换代码（字段一旦不一致会立刻编译失败）。
type JobEditInput struct {
	Title            *string    `json:"title"`
	Category         *string    `json:"category"`
	Industry         *string    `json:"industry"`
	EmploymentType   *string    `json:"employment_type"`
	Department       *string    `json:"department"`
	Headcount        *int       `json:"headcount"`
	Experience       *string    `json:"experience"`
	Education        *string    `json:"education"`
	WorkMode         *string    `json:"work_mode"`
	SalaryText       *string    `json:"salary_text"`
	SalaryMin        *int       `json:"salary_min"`
	SalaryMax        *int       `json:"salary_max"`
	SalaryMonths     *int       `json:"salary_months"`
	Country          *string    `json:"country"`
	Province         *string    `json:"province"`
	City             *string    `json:"city"`
	District         *string    `json:"district"`
	Address          *string    `json:"address"`
	LocationText     *string    `json:"location_text"`
	Summary          *string    `json:"summary"`
	Description      *string    `json:"description"`
	Responsibilities *string    `json:"responsibilities"`
	Requirements     *string    `json:"requirements"`
	RequirementsList *[]string  `json:"requirements_list"`
	Benefits         *string    `json:"benefits"`
	Tags             *[]string  `json:"tags"`
	ContactPerson    *string    `json:"contact_person"`
	ContactEmail     *string    `json:"contact_email"`
	ContactPhone     *string    `json:"contact_phone"`
	ContactWechat    *string    `json:"contact_wechat"`
	ApplyURL         *string    `json:"apply_url"`
	ApplyMethod      *string    `json:"apply_method"`
	PublishedAt      *time.Time `json:"published_at"`
	DeadlineAt       *time.Time `json:"deadline_at"`
}

// JobCreateInput 是手工新增岗位入参（管理员直接录入企业官网岗位）。
type JobCreateInput struct {
	CompanyID   string
	SourceURL   string
	Title       string
	ApplyURL    string
	AutoApprove bool
	JobEditInput
}

// 审核结论。
const (
	ReviewApprove = "approve"
	ReviewReject  = "reject"
)

// ReviewInput 是审核入参。
type ReviewInput struct {
	Decision     string
	RejectReason string
}

// List 查询岗位列表；publicOnly 为真时只返回已发布岗位（前台接口使用）。
func (svc *JobService) List(ctx context.Context, f JobListFilter, publicOnly bool) (response.Page[JobListView], error) {
	query := svc.s.Store.Ent.JobPosting.Query().WithCompany()

	if publicOnly {
		query = query.Where(jobposting.StatusEQ(jobposting.StatusApproved))
	} else if len(f.Statuses) > 0 {
		statuses := make([]jobposting.Status, 0, len(f.Statuses))
		for _, s := range f.Statuses {
			statuses = append(statuses, jobposting.Status(s))
		}
		query = query.Where(jobposting.StatusIn(statuses...))
	}

	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		query = query.Where(jobposting.Or(
			jobposting.TitleContainsFold(kw),
			jobposting.SummaryContainsFold(kw),
			jobposting.RequirementsContainsFold(kw),
			jobposting.SourceCompanyNameContainsFold(kw),
		))
	}
	if f.Category != "" {
		query = query.Where(jobposting.CategoryEQ(f.Category))
	}
	if f.Industry != "" {
		query = query.Where(jobposting.IndustryEQ(f.Industry))
	}
	if f.EmploymentType != "" {
		query = query.Where(jobposting.EmploymentTypeEQ(f.EmploymentType))
	}
	if f.Experience != "" {
		// 经验要求用包含匹配：来源页面写法差异很大（「3-5年」「3年以上」「经验不限」）。
		query = query.Where(jobposting.ExperienceContainsFold(f.Experience))
	}
	if f.SalaryMin != nil {
		// 薪资下限筛选：岗位薪资上限达到该值即命中；
		// 只填了下限的岗位（salary_max 为空）用下限兜底比较。
		query = query.Where(jobposting.Or(
			jobposting.SalaryMaxGTE(*f.SalaryMin),
			jobposting.And(jobposting.SalaryMaxIsNil(), jobposting.SalaryMinGTE(*f.SalaryMin)),
		))
	}
	if f.SalaryMax != nil {
		// 薪资上限筛选：岗位薪资下限不超过该值即命中。
		query = query.Where(jobposting.Or(
			jobposting.SalaryMinLTE(*f.SalaryMax),
			jobposting.And(jobposting.SalaryMinIsNil(), jobposting.SalaryMaxLTE(*f.SalaryMax)),
		))
	}
	if f.City != "" {
		// 城市用包含匹配，兼容「北京」「北京市」等写法差异。
		query = query.Where(jobposting.CityContainsFold(f.City))
	}
	if f.CompanyID != "" {
		query = query.Where(jobposting.CompanyIDEQ(f.CompanyID))
	}
	if f.SourceURL != "" {
		query = query.Where(jobposting.SourceURLEQ(f.SourceURL))
	}
	if f.MinQualityScore != nil {
		query = query.Where(jobposting.QualityScoreGTE(*f.MinQualityScore))
	}
	if f.IsDirectHire != nil {
		query = query.Where(jobposting.IsDirectHireEQ(*f.IsDirectHire))
	}
	if f.PublishedFrom != nil {
		query = query.Where(jobposting.PublishedAtGTE(*f.PublishedFrom))
	}
	if f.PublishedTo != nil {
		query = query.Where(jobposting.PublishedAtLTE(*f.PublishedTo))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return response.Page[JobListView]{}, apperr.ErrInternal.WithCause(err)
	}

	limit, offset, page, size := paginate(f.Page, f.PageSize, 20, 100)
	switch f.OrderBy {
	case "published_at":
		// 未标注发布时间的岗位排到最后，而不是 Postgres 默认的 NULLS FIRST。
		query = query.Order(sql.OrderByField(
			jobposting.FieldPublishedAt, sql.OrderDesc(), sql.OrderNullsLast(),
		).ToFunc())
	case "quality_score":
		query = query.Order(ent.Desc(jobposting.FieldQualityScore))
	case "created_at":
		query = query.Order(ent.Desc(jobposting.FieldID))
	default:
		query = query.Order(ent.Desc(jobposting.FieldLastUpdatedAt))
	}
	query = query.Order(ent.Desc(jobposting.FieldID))

	jobs, err := query.Limit(limit).Offset(offset).All(ctx)
	if err != nil {
		return response.Page[JobListView]{}, apperr.ErrInternal.WithCause(err)
	}

	views := make([]JobListView, 0, len(jobs))
	for _, j := range jobs {
		views = append(views, NewJobListView(j))
	}
	return response.NewPage(views, total, page, size), nil
}

// PublicList 是前台岗位列表（仅已发布）。
func (svc *JobService) PublicList(ctx context.Context, f JobListFilter) (response.Page[JobListView], error) {
	return svc.List(ctx, f, true)
}

// AdminList 是管理端岗位列表（可查看全部状态）。
func (svc *JobService) AdminList(ctx context.Context, f JobListFilter) (response.Page[JobListView], error) {
	return svc.List(ctx, f, false)
}

// Detail 查询岗位详情；publicOnly 为真时非已发布状态一律按「不存在」处理。
func (svc *JobService) Detail(ctx context.Context, id string, publicOnly bool) (JobDetailView, error) {
	job, err := svc.s.Store.Ent.JobPosting.Query().
		Where(jobposting.IDEQ(id)).
		WithCompany().
		Only(ctx)
	if ent.IsNotFound(err) {
		return JobDetailView{}, apperr.ErrJobNotFound
	}
	if err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}
	if publicOnly && job.Status != jobposting.StatusApproved {
		return JobDetailView{}, apperr.ErrJobNotFound
	}

	revisions, err := svc.s.Store.Ent.JobRevision.Query().
		Where(jobrevision.JobIDEQ(id)).
		Order(ent.Desc(jobrevision.FieldRevision)).
		Limit(20).
		All(ctx)
	if err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	if publicOnly {
		// 浏览量统计失败不影响详情返回。
		_ = svc.s.Store.Ent.JobPosting.UpdateOneID(id).
			AddViewCount(1).
			Exec(ctx)
	}
	return NewJobDetailView(job, revisions), nil
}

// PublicDetail 是前台岗位详情。
func (svc *JobService) PublicDetail(ctx context.Context, id string) (JobDetailView, error) {
	return svc.Detail(ctx, id, true)
}

// AdminDetail 是管理端岗位详情。
func (svc *JobService) AdminDetail(ctx context.Context, id string) (JobDetailView, error) {
	return svc.Detail(ctx, id, false)
}

// Related 返回相关职位，用于详情页底部的「相关职位」推荐。
//
// 召回策略：同公司 或 同职位类别 的已发布岗位，按信息完整度排序。
// 之所以不用「发布时间倒序」：发布时间可能为空，Postgres 默认 NULLS FIRST
// 会把信息不全的岗位排在最前面，反而降低推荐质量。
func (svc *JobService) Related(ctx context.Context, id string, limit int) ([]JobListView, error) {
	job, err := svc.s.Store.Ent.JobPosting.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, apperr.ErrJobNotFound
	}
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	if limit <= 0 || limit > 20 {
		limit = 6
	}

	preds := []predicate.JobPosting{jobposting.CompanyIDEQ(job.CompanyID)}
	if job.Category != "" {
		preds = append(preds, jobposting.CategoryEQ(job.Category))
	}

	rows, err := svc.s.Store.Ent.JobPosting.Query().
		WithCompany().
		Where(
			jobposting.StatusEQ(jobposting.StatusApproved),
			jobposting.IDNEQ(id),
			jobposting.Or(preds...),
		).
		Order(ent.Desc(jobposting.FieldQualityScore), ent.Desc(jobposting.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	views := make([]JobListView, 0, len(rows))
	for _, row := range rows {
		views = append(views, NewJobListView(row))
	}
	return views, nil
}

// Update 人工编辑岗位，并留痕。
func (svc *JobService) Update(ctx context.Context, id string, in JobEditInput, actor Actor) (JobDetailView, error) {
	job, err := svc.s.Store.Ent.JobPosting.Query().
		Where(jobposting.IDEQ(id)).WithCompany().Only(ctx)
	if ent.IsNotFound(err) {
		return JobDetailView{}, apperr.ErrJobNotFound
	}
	if err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	before := jobSnapshot(job)
	updater := svc.applyEdits(svc.s.Store.Ent.JobPosting.UpdateOneID(id), in)
	if _, err := updater.Save(ctx); err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	updated, err := svc.s.Store.Ent.JobPosting.Query().
		Where(jobposting.IDEQ(id)).WithCompany().Only(ctx)
	if err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	changed := changedFieldNames(before, jobSnapshot(updated))
	if len(changed) > 0 {
		revision := updated.Revision + 1
		hash := jobContentHash(updated)

		updater := svc.s.Store.Ent.JobPosting.UpdateOneID(id).
			SetRevision(revision).
			SetContentHash(hash).
			SetLastUpdatedAt(time.Now())

		// 与抓取路径口径一致：已发布岗位发生实质性变更（薪资、任职要求、地址等）
		// 时退回待审核，避免绕过审核直接改动已经对外发布的内容。
		backToReview := updated.Status == jobposting.StatusApproved && needsReReview(changed)
		if backToReview {
			updater = updater.SetStatus(jobposting.StatusPendingReview).SetRejectReason("")
		}
		if _, err := updater.Save(ctx); err != nil {
			return JobDetailView{}, apperr.ErrInternal.WithCause(err)
		}

		operator := actor.UserID
		if err := recordJobRevision(ctx, svc.s, id, revision, hash, changed,
			jobSnapshot(updated), updated.SourceURL, "manual", &operator); err != nil {
			return JobDetailView{}, err
		}
		updated.Revision = revision
		updated.ContentHash = hash
		if backToReview {
			updated.Status = jobposting.StatusPendingReview
			updated.RejectReason = ""
		}
	}

	svc.s.Audit.Record(ctx, actor, "job.update", "job", id,
		map[string]any{"changed_fields": changed})
	return NewJobDetailView(updated, nil), nil
}

// Create 手工新增岗位（用于管理员发现优质直招岗位时直接录入）。
func (svc *JobService) Create(ctx context.Context, in JobCreateInput, actor Actor) (JobDetailView, error) {
	if in.CompanyID == "" {
		return JobDetailView{}, apperr.ErrInvalidParams.WithMessage("必须指定所属公司")
	}
	if strings.TrimSpace(in.Title) == "" {
		return JobDetailView{}, apperr.ErrInvalidParams.WithMessage("岗位名称不能为空")
	}
	if strings.TrimSpace(in.SourceURL) == "" {
		return JobDetailView{}, apperr.ErrInvalidParams.WithMessage("必须提供岗位来源地址（官网链接）")
	}

	exists, err := svc.s.Store.Ent.JobPosting.Query().
		Where(jobposting.SourceURLEQ(in.SourceURL)).Exist(ctx)
	if err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}
	if exists {
		return JobDetailView{}, apperr.ErrJobExists
	}

	comp, err := svc.s.Store.Ent.Company.Query().Where(company.IDEQ(in.CompanyID)).Only(ctx)
	if ent.IsNotFound(err) {
		return JobDetailView{}, apperr.ErrCompanyNotFound
	}
	if err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	status := jobposting.StatusPendingReview
	if in.AutoApprove {
		status = jobposting.StatusApproved
	}
	now := time.Now()

	builder := svc.s.Store.Ent.JobPosting.Create().
		SetCompanyID(comp.ID).
		SetSourceURL(in.SourceURL).
		SetSourceSite(hostOf(in.SourceURL)).
		SetSourceCompanyName(comp.Name).
		SetTitle(strings.TrimSpace(in.Title)).
		SetIndustry(comp.Industry).
		SetIsDirectHire(true).
		SetStatus(status).
		SetCrawledAt(now).
		SetLastSeenAt(now).
		SetLastUpdatedAt(now).
		SetExtractor("manual").
		SetParseConfidence(1)

	builder = svc.applyEditsToCreate(builder, in.JobEditInput)
	if in.AutoApprove {
		builder = builder.SetReviewedAt(now)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return JobDetailView{}, apperr.ErrJobExists
		}
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	hash := jobContentHash(created)
	created, err = svc.s.Store.Ent.JobPosting.UpdateOneID(created.ID).
		SetContentHash(hash).
		SetQualityScore(scoreOfJob(created)).
		Save(ctx)
	if err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	if err := svc.s.Company.RefreshJobCount(ctx, comp.ID); err != nil {
		return JobDetailView{}, err
	}

	created.Edges.Company = comp
	svc.s.Audit.Record(ctx, actor, "job.create", "job", created.ID,
		map[string]any{"title": created.Title, "company": comp.Name})
	return NewJobDetailView(created, nil), nil
}

// Review 审核岗位。
func (svc *JobService) Review(ctx context.Context, id string, in ReviewInput, actor Actor) (JobDetailView, error) {
	job, err := svc.s.Store.Ent.JobPosting.Query().
		Where(jobposting.IDEQ(id)).WithCompany().Only(ctx)
	if ent.IsNotFound(err) {
		return JobDetailView{}, apperr.ErrJobNotFound
	}
	if err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	// 已发布的岗位不能重复审核（如需调整请先下线）。
	if job.Status == jobposting.StatusApproved {
		return JobDetailView{}, apperr.ErrJobAlreadyHandled
	}
	if job.Status != jobposting.StatusPendingReview && job.Status != jobposting.StatusRejected {
		return JobDetailView{}, apperr.ErrJobStateInvalid.WithMessage("当前状态不允许审核：" + string(job.Status))
	}

	now := time.Now()
	updater := svc.s.Store.Ent.JobPosting.UpdateOneID(id).
		SetReviewerID(actor.UserID).
		SetReviewedAt(now)

	switch strings.ToLower(strings.TrimSpace(in.Decision)) {
	case ReviewApprove, "approved", "pass":
		updater = updater.SetStatus(jobposting.StatusApproved).SetRejectReason("")
	case ReviewReject, "rejected", "fail":
		reason := strings.TrimSpace(in.RejectReason)
		if reason == "" {
			return JobDetailView{}, apperr.ErrInvalidParams.WithMessage("驳回必须填写原因")
		}
		updater = updater.SetStatus(jobposting.StatusRejected).SetRejectReason(truncateRunes(reason, 512))
	default:
		return JobDetailView{}, apperr.ErrInvalidParams.WithMessage("审核结论只能是 approve 或 reject")
	}

	if _, err := updater.Save(ctx); err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	if err := svc.s.Company.RefreshJobCount(ctx, job.CompanyID); err != nil {
		return JobDetailView{}, err
	}

	svc.s.Audit.Record(ctx, actor, "job."+strings.ToLower(in.Decision), "job", id, map[string]any{
		"title":         job.Title,
		"decision":      in.Decision,
		"reject_reason": in.RejectReason,
	})

	return svc.Detail(ctx, id, false)
}

// SetStatus 切换岗位状态（上线/下线/标记过期）。
func (svc *JobService) SetStatus(ctx context.Context, id string, status string, actor Actor) (JobDetailView, error) {
	target := jobposting.Status(status)
	switch target {
	case jobposting.StatusApproved, jobposting.StatusOffline, jobposting.StatusExpired, jobposting.StatusPendingReview:
	default:
		return JobDetailView{}, apperr.ErrInvalidParams.WithMessage("不支持的岗位状态：" + status)
	}

	job, err := svc.s.Store.Ent.JobPosting.Get(ctx, id)
	if ent.IsNotFound(err) {
		return JobDetailView{}, apperr.ErrJobNotFound
	}
	if err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	// 重新上线视为一次人工确认，记录审核人与时间。
	updater := svc.s.Store.Ent.JobPosting.UpdateOneID(id).SetStatus(target)
	if target == jobposting.StatusApproved {
		updater = updater.SetReviewerID(actor.UserID).SetReviewedAt(time.Now()).SetRejectReason("")
	}
	if target == jobposting.StatusExpired {
		updater = updater.SetExpiredAt(time.Now())
	}
	if _, err := updater.Save(ctx); err != nil {
		return JobDetailView{}, apperr.ErrInternal.WithCause(err)
	}

	if err := svc.s.Company.RefreshJobCount(ctx, job.CompanyID); err != nil {
		return JobDetailView{}, err
	}

	svc.s.Audit.Record(ctx, actor, "job.set_status", "job", id,
		map[string]any{"old": string(job.Status), "new": status})
	return svc.Detail(ctx, id, false)
}

// Delete 删除岗位。
func (svc *JobService) Delete(ctx context.Context, id string, actor Actor) error {
	job, err := svc.s.Store.Ent.JobPosting.Get(ctx, id)
	if ent.IsNotFound(err) {
		return apperr.ErrJobNotFound
	}
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	// 修订记录依赖岗位，先删子表再删主记录。
	if _, err := svc.s.Store.Ent.JobRevision.Delete().
		Where(jobrevision.JobIDEQ(id)).Exec(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	if err := svc.s.Store.Ent.JobPosting.DeleteOneID(id).Exec(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	if err := svc.s.Company.RefreshJobCount(ctx, job.CompanyID); err != nil {
		return err
	}
	svc.s.Audit.Record(ctx, actor, "job.delete", "job", id, map[string]any{"title": job.Title})
	return nil
}

// Revisions 返回岗位变更历史。
func (svc *JobService) Revisions(ctx context.Context, id string) ([]RevisionView, error) {
	rows, err := svc.s.Store.Ent.JobRevision.Query().
		Where(jobrevision.JobIDEQ(id)).
		Order(ent.Desc(jobrevision.FieldRevision)).
		All(ctx)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	views := make([]RevisionView, 0, len(rows))
	for _, r := range rows {
		views = append(views, NewRevisionView(r))
	}
	return views, nil
}

// PendingCount 返回待审核岗位数。
func (svc *JobService) PendingCount(ctx context.Context) (int, error) {
	n, err := svc.s.Store.Ent.JobPosting.Query().
		Where(jobposting.StatusEQ(jobposting.StatusPendingReview)).
		Count(ctx)
	if err != nil {
		return 0, apperr.ErrInternal.WithCause(err)
	}
	return n, nil
}

// —— 内部辅助 ——

// applyEdits 把编辑入参应用到更新构造器上。
func (svc *JobService) applyEdits(updater *ent.JobPostingUpdateOne, in JobEditInput) *ent.JobPostingUpdateOne {
	if in.Title != nil {
		updater = updater.SetTitle(truncateRunes(strings.TrimSpace(*in.Title), 255))
	}
	if in.Category != nil {
		updater = updater.SetCategory(truncateRunes(*in.Category, 64))
	}
	if in.Industry != nil {
		updater = updater.SetIndustry(truncateRunes(*in.Industry, 64))
	}
	if in.EmploymentType != nil {
		updater = updater.SetEmploymentType(truncateRunes(*in.EmploymentType, 32))
	}
	if in.Department != nil {
		updater = updater.SetDepartment(truncateRunes(*in.Department, 128))
	}
	if in.Headcount != nil {
		updater = updater.SetHeadcount(*in.Headcount)
	}
	if in.Experience != nil {
		updater = updater.SetExperience(truncateRunes(*in.Experience, 64))
	}
	if in.Education != nil {
		updater = updater.SetEducation(truncateRunes(*in.Education, 64))
	}
	if in.WorkMode != nil {
		updater = updater.SetWorkMode(truncateRunes(*in.WorkMode, 32))
	}
	if in.SalaryText != nil {
		updater = updater.SetSalaryText(truncateRunes(*in.SalaryText, 128))
	}
	if in.SalaryMin != nil {
		updater = updater.SetSalaryMin(*in.SalaryMin)
	}
	if in.SalaryMax != nil {
		updater = updater.SetSalaryMax(*in.SalaryMax)
	}
	if in.SalaryMonths != nil {
		updater = updater.SetSalaryMonths(*in.SalaryMonths)
	}
	if in.Country != nil {
		updater = updater.SetCountry(truncateRunes(*in.Country, 64))
	}
	if in.Province != nil {
		updater = updater.SetProvince(truncateRunes(*in.Province, 64))
	}
	if in.City != nil {
		updater = updater.SetCity(truncateRunes(*in.City, 64))
	}
	if in.District != nil {
		updater = updater.SetDistrict(truncateRunes(*in.District, 64))
	}
	if in.Address != nil {
		updater = updater.SetAddress(truncateRunes(*in.Address, 512))
	}
	if in.LocationText != nil {
		updater = updater.SetLocationText(truncateRunes(*in.LocationText, 255))
	}
	if in.Summary != nil {
		updater = updater.SetSummary(truncateRunes(*in.Summary, 512))
	}
	if in.Description != nil {
		updater = updater.SetDescription(*in.Description)
	}
	if in.Responsibilities != nil {
		updater = updater.SetResponsibilities(*in.Responsibilities)
	}
	if in.Requirements != nil {
		updater = updater.SetRequirements(*in.Requirements)
	}
	if in.RequirementsList != nil {
		updater = updater.SetRequirementsList(*in.RequirementsList)
	}
	if in.Benefits != nil {
		updater = updater.SetBenefits(*in.Benefits)
	}
	if in.Tags != nil {
		updater = updater.SetTags(*in.Tags)
	}
	if in.ContactPerson != nil {
		updater = updater.SetContactPerson(truncateRunes(*in.ContactPerson, 64))
	}
	if in.ContactEmail != nil {
		updater = updater.SetContactEmail(truncateRunes(*in.ContactEmail, 255))
	}
	if in.ContactPhone != nil {
		updater = updater.SetContactPhone(truncateRunes(*in.ContactPhone, 64))
	}
	if in.ContactWechat != nil {
		updater = updater.SetContactWechat(truncateRunes(*in.ContactWechat, 64))
	}
	if in.ApplyURL != nil {
		updater = updater.SetApplyURL(truncateRunes(*in.ApplyURL, 1024))
	}
	if in.ApplyMethod != nil {
		updater = updater.SetApplyMethod(truncateRunes(*in.ApplyMethod, 64))
	}
	if in.PublishedAt != nil {
		updater = updater.SetPublishedAt(*in.PublishedAt)
	}
	if in.DeadlineAt != nil {
		updater = updater.SetDeadlineAt(*in.DeadlineAt)
	}
	return updater
}

// applyEditsToCreate 把编辑入参应用到创建构造器上。
//
// 说明：ent 的 Create 与 UpdateOne 是两套不同的构造器，无法共用同一段赋值代码，
// 因此这里保持显式展开——虽然啰嗦，但类型安全且一眼可查。
func (svc *JobService) applyEditsToCreate(builder *ent.JobPostingCreate, in JobEditInput) *ent.JobPostingCreate {
	if in.Category != nil {
		builder = builder.SetCategory(truncateRunes(*in.Category, 64))
	}
	if in.EmploymentType != nil {
		builder = builder.SetEmploymentType(truncateRunes(*in.EmploymentType, 32))
	}
	if in.Department != nil {
		builder = builder.SetDepartment(truncateRunes(*in.Department, 128))
	}
	if in.City != nil {
		builder = builder.SetCity(truncateRunes(*in.City, 64))
	}
	if in.District != nil {
		builder = builder.SetDistrict(truncateRunes(*in.District, 64))
	}
	if in.Address != nil {
		builder = builder.SetAddress(truncateRunes(*in.Address, 512))
	}
	if in.SalaryText != nil {
		builder = builder.SetSalaryText(truncateRunes(*in.SalaryText, 128))
	}
	if in.SalaryMin != nil {
		builder = builder.SetSalaryMin(*in.SalaryMin)
	}
	if in.SalaryMax != nil {
		builder = builder.SetSalaryMax(*in.SalaryMax)
	}
	if in.Experience != nil {
		builder = builder.SetExperience(truncateRunes(*in.Experience, 64))
	}
	if in.Education != nil {
		builder = builder.SetEducation(truncateRunes(*in.Education, 64))
	}
	if in.Summary != nil {
		builder = builder.SetSummary(truncateRunes(*in.Summary, 512))
	}
	if in.Description != nil {
		builder = builder.SetDescription(*in.Description)
	}
	if in.Responsibilities != nil {
		builder = builder.SetResponsibilities(*in.Responsibilities)
	}
	if in.Requirements != nil {
		builder = builder.SetRequirements(*in.Requirements)
	}
	if in.RequirementsList != nil {
		builder = builder.SetRequirementsList(*in.RequirementsList)
	}
	if in.Benefits != nil {
		builder = builder.SetBenefits(*in.Benefits)
	}
	if in.Tags != nil {
		builder = builder.SetTags(*in.Tags)
	}
	if in.ContactPerson != nil {
		builder = builder.SetContactPerson(truncateRunes(*in.ContactPerson, 64))
	}
	if in.ContactEmail != nil {
		builder = builder.SetContactEmail(truncateRunes(*in.ContactEmail, 255))
	}
	if in.ContactPhone != nil {
		builder = builder.SetContactPhone(truncateRunes(*in.ContactPhone, 64))
	}
	if in.PublishedAt != nil {
		builder = builder.SetPublishedAt(*in.PublishedAt)
	}
	if in.DeadlineAt != nil {
		builder = builder.SetDeadlineAt(*in.DeadlineAt)
	}
	if in.Headcount != nil {
		builder = builder.SetHeadcount(*in.Headcount)
	}
	if in.WorkMode != nil {
		builder = builder.SetWorkMode(truncateRunes(*in.WorkMode, 32))
	}
	if in.ApplyMethod != nil {
		builder = builder.SetApplyMethod(truncateRunes(*in.ApplyMethod, 64))
	}
	return builder
}

// scoreOfJob 计算岗位信息完整度评分（与爬虫侧的打分口径保持一致）。
func scoreOfJob(j *ent.JobPosting) int {
	if j == nil {
		return 0
	}
	score := 0
	if strings.TrimSpace(j.Title) != "" {
		score += 10
	}
	if len([]rune(j.Description)) >= 20 {
		score += 10
	}
	switch n := len([]rune(j.Requirements)); {
	case n >= 50:
		score += 15
	case n >= 10:
		score += 8
	}
	if len(j.RequirementsList) >= 3 {
		score += 5
	}
	if len([]rune(j.Responsibilities)) >= 20 {
		score += 10
	}
	if strings.TrimSpace(j.Address) != "" {
		score += 6
	}
	if strings.TrimSpace(j.City) != "" {
		score += 6
	}
	if j.PublishedAt != nil && !j.PublishedAt.IsZero() {
		score += 10
	}
	if j.SalaryMin != nil || j.SalaryMax != nil {
		score += 5
	} else if strings.TrimSpace(j.SalaryText) != "" {
		score += 3
	}
	if strings.TrimSpace(j.ContactEmail) != "" {
		score += 5
	}
	if strings.TrimSpace(j.ContactPhone) != "" {
		score += 3
	}
	if strings.TrimSpace(j.ApplyURL) != "" {
		score += 4
	}
	if strings.TrimSpace(j.EmploymentType) != "" {
		score += 3
	}
	if strings.TrimSpace(j.Category) != "" {
		score += 3
	}
	if strings.TrimSpace(j.Experience) != "" {
		score += 3
	}
	if strings.TrimSpace(j.Education) != "" {
		score += 2
	}
	if len(j.Tags) > 0 || strings.TrimSpace(j.Benefits) != "" {
		score += 2
	}
	if score > 100 {
		score = 100
	}
	return score
}

// hostOf 从 URL 中提取主机名，解析失败时返回空串。
func hostOf(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}
