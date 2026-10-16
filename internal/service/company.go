package service

import (
	"context"
	"log/slog"
	"net/url"
	"reflect"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/company"
	"github.com/jsong/jobs-robot/internal/ent/crawllog"
	"github.com/jsong/jobs-robot/internal/ent/crawlrun"
	"github.com/jsong/jobs-robot/internal/ent/jobposting"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// CompanyService 负责「招聘来源公司」的全生命周期管理。
//
// 公司在整条链路中承担两个角色：
//   - 岗位的来源主体（岗位通过 company_id 归属到公司）；
//   - 定时抓取的调度单元（careers_url 是采集入口，crawl_rules 是站点适配规则）。
//
// 因此本服务同时提供管理端 CRUD 与抓取调度所需的候选集查询。
type CompanyService struct{ s *Services }

// CompanyListFilter 是公司列表筛选条件。
type CompanyListFilter struct {
	Keyword      string // 匹配 name / short_name / website
	Industry     string
	Enabled      *bool
	CrawlEnabled *bool
	Verified     *bool
	OrderBy      string // priority | created_at | last_crawled_at | job_count（默认 priority desc, id desc）
	Page         int
	PageSize     int
}

// CompanyCreateInput 是创建公司入参。
type CompanyCreateInput struct {
	Name                 string
	ShortName            string
	Website              string
	CareersURL           string
	LogoURL              string
	Industry             string
	SubIndustry          string
	Scale                string
	Nature               string
	Location             string
	Description          string
	ContactEmail         string
	ContactPhone         string
	Enabled              *bool
	Verified             *bool
	Priority             *int
	CrawlEnabled         *bool
	CrawlIntervalSeconds *int
	CrawlRules           map[string]any
	ForceHeadless        *bool
	RespectRobots        *bool
	Remark               string
}

// CompanyUpdateInput 是更新公司入参；所有字段为指针，nil 表示不修改。
type CompanyUpdateInput struct {
	Name                 *string
	ShortName            *string
	Website              *string
	CareersURL           *string
	LogoURL              *string
	Industry             *string
	SubIndustry          *string
	Scale                *string
	Nature               *string
	Location             *string
	Description          *string
	ContactEmail         *string
	ContactPhone         *string
	Enabled              *bool
	Verified             *bool
	Priority             *int
	CrawlEnabled         *bool
	CrawlIntervalSeconds *int
	CrawlRules           map[string]any
	ForceHeadless        *bool
	RespectRobots        *bool
	Remark               *string
}

// 审计动作标识：与 schema 注释里的 "company.create" 约定保持一致。
const (
	actionCompanyCreate        = "company.create"
	actionCompanyUpdate        = "company.update"
	actionCompanyDelete        = "company.delete"
	actionCompanySetEnabled    = "company.set_enabled"
	actionCompanySetCareersURL = "company.set_careers_url"
	actionCompanyRefreshJobs   = "company.refresh_job_count"

	resourceCompany = "company"
)

// List 分页查询公司列表。
func (svc *CompanyService) List(ctx context.Context, f CompanyListFilter) (response.Page[CompanyView], error) {
	limit, offset, page, pageSize := paginate(f.Page, f.PageSize, 20, 100)

	q := svc.s.Store.Ent.Company.Query()
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		// 关键词只覆盖「能唯一标识一家公司」的字段，避免把简介等长文本纳入扫描。
		q = q.Where(company.Or(
			company.NameContainsFold(kw),
			company.ShortNameContainsFold(kw),
			company.WebsiteContainsFold(kw),
		))
	}
	if industry := strings.TrimSpace(f.Industry); industry != "" {
		q = q.Where(company.IndustryEQ(industry))
	}
	if f.Enabled != nil {
		q = q.Where(company.EnabledEQ(*f.Enabled))
	}
	if f.CrawlEnabled != nil {
		q = q.Where(company.CrawlEnabledEQ(*f.CrawlEnabled))
	}
	if f.Verified != nil {
		q = q.Where(company.VerifiedEQ(*f.Verified))
	}

	orders, err := companyOrder(f.OrderBy)
	if err != nil {
		return response.Page[CompanyView]{}, err
	}

	total, err := q.Count(ctx)
	if err != nil {
		return response.Page[CompanyView]{}, apperr.ErrInternal.WithCause(err)
	}

	rows, err := q.Order(orders...).Offset(offset).Limit(limit).All(ctx)
	if err != nil {
		return response.Page[CompanyView]{}, apperr.ErrInternal.WithCause(err)
	}

	views := make([]CompanyView, 0, len(rows))
	for _, row := range rows {
		views = append(views, NewCompanyView(row))
	}
	return response.NewPage(views, total, page, pageSize), nil
}

// companyOrder 把外部排序字段翻译成 ent 排序条件。
//
// 每种排序都追加 id 兜底，保证同值记录的顺序稳定（分页时不会出现重复/漏项）。
func companyOrder(orderBy string) ([]company.OrderOption, error) {
	switch strings.ToLower(strings.TrimSpace(orderBy)) {
	case "", "priority":
		// 默认：优先级高者先抓，与 Candidates 的调度顺序一致。
		return []company.OrderOption{
			company.ByPriority(sql.OrderDesc()),
			company.ByID(sql.OrderDesc()),
		}, nil
	case "created_at":
		return []company.OrderOption{
			company.ByCreateTime(sql.OrderDesc()),
			company.ByID(sql.OrderDesc()),
		}, nil
	case "last_crawled_at":
		// 最近抓取过的排最前；从未抓取过（NULL）的排最后，避免空值把真实数据挤下去。
		return []company.OrderOption{
			company.ByLastCrawledAt(sql.OrderDesc(), sql.OrderNullsLast()),
			company.ByID(sql.OrderDesc()),
		}, nil
	case "job_count":
		return []company.OrderOption{
			company.ByJobCount(sql.OrderDesc()),
			company.ByID(sql.OrderDesc()),
		}, nil
	default:
		return nil, apperr.ErrInvalidParams.
			WithMessage("排序字段不支持，可选 priority/created_at/last_crawled_at/job_count").
			WithDetails(map[string]any{"field": "order_by", "value": orderBy})
	}
}

// Get 返回公司详情视图。
func (svc *CompanyService) Get(ctx context.Context, id string) (CompanyView, error) {
	entity, err := svc.GetEntity(ctx, id)
	if err != nil {
		return CompanyView{}, err
	}
	return NewCompanyView(entity), nil
}

// GetEntity 返回实体（含边），供其他服务复用。
//
// 这里不主动预加载 jobs / crawl_runs：一家公司可能挂着大量岗位与抓取记录，
// 无差别预加载会把详情接口拖慢；调用方确有需要时用 entity.QueryJobs() 等按需取。
// 方法名保留「含边」语义——实体本身携带 Edges 结构，只是默认不填充。
func (svc *CompanyService) GetEntity(ctx context.Context, id string) (*ent.Company, error) {
	entity, err := svc.s.Store.Ent.Company.Query().Where(company.ID(id)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, apperr.ErrCompanyNotFound.WithCause(err)
		}
		return nil, apperr.ErrInternal.WithCause(err)
	}
	return entity, nil
}

// Create 创建公司。
//
// 允许「只有官网、暂无招聘页」的先建后回填模式：
// CareersURL 为空不是错误，发现流程会在抓到《加入我们》入口后调用 SetCareersURL 补全。
func (svc *CompanyService) Create(ctx context.Context, in CompanyCreateInput, actor Actor) (CompanyView, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return CompanyView{}, apperr.ErrInvalidParams.
			WithMessage("公司名称不能为空").
			WithDetails(map[string]any{"field": "name"})
	}
	if err := validateHTTPURL("website", in.Website); err != nil {
		return CompanyView{}, err
	}
	if err := validateHTTPURL("careers_url", in.CareersURL); err != nil {
		return CompanyView{}, err
	}
	if err := validateCrawlRules(in.CrawlRules); err != nil {
		return CompanyView{}, err
	}
	if in.CrawlIntervalSeconds != nil && *in.CrawlIntervalSeconds <= 0 {
		return CompanyView{}, apperr.ErrInvalidParams.
			WithMessage("抓取间隔必须大于 0 秒").
			WithDetails(map[string]any{"field": "crawl_interval_seconds"})
	}

	builder := svc.s.Store.Ent.Company.Create().
		SetName(name).
		SetShortName(strings.TrimSpace(in.ShortName)).
		SetWebsite(strings.TrimSpace(in.Website)).
		SetCareersURL(strings.TrimSpace(in.CareersURL)).
		SetLogoURL(strings.TrimSpace(in.LogoURL)).
		SetIndustry(strings.TrimSpace(in.Industry)).
		SetSubIndustry(strings.TrimSpace(in.SubIndustry)).
		SetScale(strings.TrimSpace(in.Scale)).
		SetNature(strings.TrimSpace(in.Nature)).
		SetLocation(strings.TrimSpace(in.Location)).
		SetDescription(strings.TrimSpace(in.Description)).
		SetContactEmail(strings.TrimSpace(in.ContactEmail)).
		SetContactPhone(strings.TrimSpace(in.ContactPhone)).
		SetRemark(strings.TrimSpace(in.Remark)).
		// 未显式传入的开关一律交给数据库默认值（enabled/crawl_enabled=true、respect_robots=true 等），
		// SetNillable* 在指针为 nil 时是空操作。
		SetNillableEnabled(in.Enabled).
		SetNillableVerified(in.Verified).
		SetNillablePriority(in.Priority).
		SetNillableCrawlEnabled(in.CrawlEnabled).
		SetNillableCrawlIntervalSeconds(in.CrawlIntervalSeconds).
		SetNillableForceHeadless(in.ForceHeadless).
		SetNillableRespectRobots(in.RespectRobots)
	if in.CrawlRules != nil {
		builder = builder.SetCrawlRules(in.CrawlRules)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return CompanyView{}, companyWriteErr(err, name)
	}

	svc.s.Audit.Record(ctx, actor, actionCompanyCreate, resourceCompany, created.ID, map[string]any{
		"name":          created.Name,
		"website":       created.Website,
		"careers_url":   created.CareersURL,
		"industry":      created.Industry,
		"enabled":       created.Enabled,
		"crawl_enabled": created.CrawlEnabled,
		"priority":      created.Priority,
	})
	return NewCompanyView(created), nil
}

// Update 更新公司；只写入发生变化的字段。
func (svc *CompanyService) Update(ctx context.Context, id string, in CompanyUpdateInput, actor Actor) (CompanyView, error) {
	current, err := svc.GetEntity(ctx, id)
	if err != nil {
		return CompanyView{}, err
	}

	// 先做整体校验，避免「改了一半才发现 URL 非法」的半成品写入。
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return CompanyView{}, apperr.ErrInvalidParams.
			WithMessage("公司名称不能为空").
			WithDetails(map[string]any{"field": "name"})
	}
	if in.Website != nil {
		if err := validateHTTPURL("website", *in.Website); err != nil {
			return CompanyView{}, err
		}
	}
	if in.CareersURL != nil {
		if err := validateHTTPURL("careers_url", *in.CareersURL); err != nil {
			return CompanyView{}, err
		}
	}
	if err := validateCrawlRules(in.CrawlRules); err != nil {
		return CompanyView{}, err
	}
	if in.CrawlIntervalSeconds != nil && *in.CrawlIntervalSeconds <= 0 {
		return CompanyView{}, apperr.ErrInvalidParams.
			WithMessage("抓取间隔必须大于 0 秒").
			WithDetails(map[string]any{"field": "crawl_interval_seconds"})
	}

	update := svc.s.Store.Ent.Company.UpdateOneID(id)
	detail := map[string]any{}

	applyString := func(field string, in *string, cur string, set func(string) *ent.CompanyUpdateOne) {
		if in == nil {
			return
		}
		next := strings.TrimSpace(*in)
		if next == cur {
			return
		}
		update = set(next)
		detail[field] = fieldChange(cur, next)
	}
	applyBool := func(field string, in *bool, cur bool, set func(bool) *ent.CompanyUpdateOne) {
		if in == nil || *in == cur {
			return
		}
		update = set(*in)
		detail[field] = fieldChange(cur, *in)
	}
	applyInt := func(field string, in *int, cur int, set func(int) *ent.CompanyUpdateOne) {
		if in == nil || *in == cur {
			return
		}
		update = set(*in)
		detail[field] = fieldChange(cur, *in)
	}

	applyString("name", in.Name, current.Name, update.SetName)
	applyString("short_name", in.ShortName, current.ShortName, update.SetShortName)
	applyString("website", in.Website, current.Website, update.SetWebsite)
	applyString("careers_url", in.CareersURL, current.CareersURL, update.SetCareersURL)
	applyString("logo_url", in.LogoURL, current.LogoURL, update.SetLogoURL)
	applyString("industry", in.Industry, current.Industry, update.SetIndustry)
	applyString("sub_industry", in.SubIndustry, current.SubIndustry, update.SetSubIndustry)
	applyString("scale", in.Scale, current.Scale, update.SetScale)
	applyString("nature", in.Nature, current.Nature, update.SetNature)
	applyString("location", in.Location, current.Location, update.SetLocation)
	applyString("description", in.Description, current.Description, update.SetDescription)
	applyString("contact_email", in.ContactEmail, current.ContactEmail, update.SetContactEmail)
	applyString("contact_phone", in.ContactPhone, current.ContactPhone, update.SetContactPhone)
	applyString("remark", in.Remark, current.Remark, update.SetRemark)
	applyBool("enabled", in.Enabled, current.Enabled, update.SetEnabled)
	applyBool("verified", in.Verified, current.Verified, update.SetVerified)
	applyInt("priority", in.Priority, current.Priority, update.SetPriority)
	applyBool("crawl_enabled", in.CrawlEnabled, current.CrawlEnabled, update.SetCrawlEnabled)
	applyInt("crawl_interval_seconds", in.CrawlIntervalSeconds, current.CrawlIntervalSeconds, update.SetCrawlIntervalSeconds)
	applyBool("force_headless", in.ForceHeadless, current.ForceHeadless, update.SetForceHeadless)
	applyBool("respect_robots", in.RespectRobots, current.RespectRobots, update.SetRespectRobots)

	if in.CrawlRules != nil && !reflect.DeepEqual(current.CrawlRules, in.CrawlRules) {
		update = update.SetCrawlRules(in.CrawlRules)
		detail["crawl_rules"] = fieldChange(current.CrawlRules, in.CrawlRules)
	}

	if len(detail) == 0 {
		// 无实质变更时不写库：既避免 updated_at 无意义抖动，也不制造审计噪音。
		return NewCompanyView(current), nil
	}

	updated, err := update.Save(ctx)
	if err != nil {
		return CompanyView{}, companyWriteErr(err, derefString(in.Name))
	}

	svc.s.Audit.Record(ctx, actor, actionCompanyUpdate, resourceCompany, id, detail)
	return NewCompanyView(updated), nil
}

// Delete 删除公司。
//
// 明确不做岗位级联删除：岗位一旦审核发布就是对外可见的资产，
// 连带删除会造成「招聘信息凭空消失」且不可追溯；因此有岗位时直接拒绝。
//
// 抓取运行记录（crawl_runs/crawl_logs）是公司私有的内部诊断数据，
// 外键为 NoAction（既不能置空也不能孤立存在），因此随公司一并清理。
func (svc *CompanyService) Delete(ctx context.Context, id string, actor Actor) error {
	current, err := svc.GetEntity(ctx, id)
	if err != nil {
		return err
	}

	jobCount, err := svc.s.Store.Ent.JobPosting.Query().
		Where(jobposting.HasCompanyWith(company.ID(id))).
		Count(ctx)
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	if jobCount > 0 {
		return apperr.ErrInvalidParams.
			WithMessage("该公司下仍有岗位，请先删除或转移岗位后再删除公司").
			WithDetails(map[string]any{"company_id": id, "job_count": jobCount})
	}

	runCount, err := svc.s.Store.Ent.CrawlRun.Query().
		Where(crawlrun.HasCompanyWith(company.ID(id))).
		Count(ctx)
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}

	tx, err := svc.s.Store.Ent.Tx(ctx)
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	// 无论计数结果如何都执行清理：计数与事务之间可能有新的抓取记录写入，
	// 漏删会被外键拦下并变成 500，因此这里以求稳为主（空删代价极低）。
	// 顺序由外键决定：先删明细日志，再删运行记录。
	if _, err := tx.CrawlLog.Delete().
		Where(crawllog.HasRunWith(crawlrun.HasCompanyWith(company.ID(id)))).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		return apperr.ErrInternal.WithCause(err)
	}
	if _, err := tx.CrawlRun.Delete().
		Where(crawlrun.HasCompanyWith(company.ID(id))).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		return apperr.ErrInternal.WithCause(err)
	}
	if err := tx.Company.DeleteOneID(id).Exec(ctx); err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return apperr.ErrCompanyNotFound.WithCause(err)
		}
		return apperr.ErrInternal.WithCause(err)
	}
	if err := tx.Commit(); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}

	svc.s.Audit.Record(ctx, actor, actionCompanyDelete, resourceCompany, id, map[string]any{
		"name":               current.Name,
		"website":            current.Website,
		"careers_url":        current.CareersURL,
		"deleted_crawl_runs": runCount,
	})
	return nil
}

// SetEnabled 启用/停用公司。
//
// 停用会同时影响抓取调度与前台可见性，因此单独提供方法并强制留痕。
func (svc *CompanyService) SetEnabled(ctx context.Context, id string, enabled bool, actor Actor) (CompanyView, error) {
	current, err := svc.GetEntity(ctx, id)
	if err != nil {
		return CompanyView{}, err
	}
	if current.Enabled == enabled {
		// 幂等：状态未变化时不写库、不记审计。
		return NewCompanyView(current), nil
	}

	updated, err := svc.s.Store.Ent.Company.UpdateOneID(id).SetEnabled(enabled).Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return CompanyView{}, apperr.ErrCompanyNotFound.WithCause(err)
		}
		return CompanyView{}, apperr.ErrInternal.WithCause(err)
	}

	svc.s.Audit.Record(ctx, actor, actionCompanySetEnabled, resourceCompany, id, map[string]any{
		"enabled": fieldChange(current.Enabled, enabled),
	})
	return NewCompanyView(updated), nil
}

// SetCareersURL 回填/更新招聘入口（由发现流程调用）。
//
// discoveryURL 记录「在哪个页面上找到了这个入口」，便于排查误判；
// 首次回填还会把 source 从 manual 改为 discovered，用于区分人工录入与自动发现。
func (svc *CompanyService) SetCareersURL(ctx context.Context, id string, careersURL, discoveryURL string, actor Actor) (CompanyView, error) {
	current, err := svc.GetEntity(ctx, id)
	if err != nil {
		return CompanyView{}, err
	}

	careersURL = strings.TrimSpace(careersURL)
	discoveryURL = strings.TrimSpace(discoveryURL)
	if err := validateHTTPURL("careers_url", careersURL); err != nil {
		return CompanyView{}, err
	}
	if err := validateHTTPURL("discovery_url", discoveryURL); err != nil {
		return CompanyView{}, err
	}

	update := svc.s.Store.Ent.Company.UpdateOneID(id)
	detail := map[string]any{}

	if careersURL != current.CareersURL {
		update = update.SetCareersURL(careersURL)
		detail["careers_url"] = fieldChange(current.CareersURL, careersURL)
	}
	if discoveryURL != "" && discoveryURL != current.DiscoveryURL {
		update = update.SetDiscoveryURL(discoveryURL)
		detail["discovery_url"] = fieldChange(current.DiscoveryURL, discoveryURL)
	}
	if current.CareersURL == "" && careersURL != "" && current.Source != "discovered" {
		update = update.SetSource("discovered")
		detail["source"] = fieldChange(current.Source, "discovered")
	}

	if len(detail) == 0 {
		return NewCompanyView(current), nil
	}

	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return CompanyView{}, apperr.ErrCompanyNotFound.WithCause(err)
		}
		return CompanyView{}, apperr.ErrInternal.WithCause(err)
	}

	svc.s.Audit.Record(ctx, actor, actionCompanySetCareersURL, resourceCompany, id, detail)
	return NewCompanyView(updated), nil
}

// RefreshJobCount 重新统计该公司「已发布(approved)」岗位数并写回 job_count。
//
// job_count 是冗余统计字段（列表页按岗位数排序时避免每行一次 count 子查询），
// 由岗位审核/下线流程在状态变化后调用本方法刷新。
func (svc *CompanyService) RefreshJobCount(ctx context.Context, id string) error {
	count, err := svc.s.Store.Ent.JobPosting.Query().
		Where(
			jobposting.HasCompanyWith(company.ID(id)),
			jobposting.StatusEQ(jobposting.StatusApproved),
		).
		Count(ctx)
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}

	current, err := svc.GetEntity(ctx, id)
	if err != nil {
		return err
	}
	if current.JobCount == count {
		return nil
	}

	if _, err := svc.s.Store.Ent.Company.UpdateOneID(id).SetJobCount(count).Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return apperr.ErrCompanyNotFound.WithCause(err)
		}
		return apperr.ErrInternal.WithCause(err)
	}

	// 统计值是派生缓存，仅在真的变化时留痕：审核高峰期不要把审计表刷成计数流水。
	// 本方法没有 actor 参数（可由任意业务流程触发），因此记为系统操作。
	svc.s.Audit.Record(ctx, SystemActor(), actionCompanyRefreshJobs, resourceCompany, id, map[string]any{
		"job_count": fieldChange(current.JobCount, count),
	})
	return nil
}

// Candidates 返回参与定时抓取的公司。
//
// 过滤条件是 enabled 且 crawl_enabled；排序为优先级降序、
// 最近一次抓取时间升序（从未抓取过的 NULL 排最前），
// 让「高优先级 + 长期没抓过」的公司优先被调度。
func (svc *CompanyService) Candidates(ctx context.Context, limit int) ([]*ent.Company, error) {
	q := svc.s.Store.Ent.Company.Query().
		Where(
			company.EnabledEQ(true),
			company.CrawlEnabledEQ(true),
		).
		Order(
			company.ByPriority(sql.OrderDesc()),
			company.ByLastCrawledAt(sql.OrderAsc(), sql.OrderNullsFirst()),
			company.ByID(sql.OrderAsc()),
		)
	if limit > 0 {
		q = q.Limit(limit)
	}

	rows, err := q.All(ctx)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	return rows, nil
}

// —— 校验小工具 ——

// validateHTTPURL 校验字段是否为 http/https 绝对地址。
//
// 空串表示「暂未填写」而放行：公司允许先录入官网，招聘入口稍后由发现流程回填。
func validateHTTPURL(field, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return invalidURLErr(field, raw)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		// 只允许 http/https：file://、javascript: 等协议对抓取无意义且存在安全风险。
		return invalidURLErr(field, raw)
	}
}

// invalidURLErr 统一 URL 校验失败的错误（沿用公司相关错误码）。
func invalidURLErr(field, raw string) error {
	message := "官网地址无效，必须是 http/https 绝对地址"
	switch field {
	case "careers_url":
		message = "招聘页地址无效，必须是 http/https 绝对地址"
	case "discovery_url":
		message = "发现来源地址无效，必须是 http/https 绝对地址"
	}
	return apperr.ErrCareersURLInvalid.
		WithMessage(message).
		WithDetails(map[string]any{"field": field, "value": raw})
}

// crawlRuleStringKeys 是 crawl_rules 中「字符串型」的键。
//
// 键名必须与 crawler.SiteRules 的 json tag 严格一致，否则规则写了也不会生效。
var crawlRuleStringKeys = map[string]struct{}{
	"careers_link_selector": {},
	"list_selector":         {},
	"item_selector":         {},
	"link_selector":         {},
}

// validateCrawlRules 对 crawl_rules 做宽松校验。
//
// 策略：
//   - 未知键原样保留（爬虫侧忽略即可），只在日志里告警，方便发现拼错的键名；
//   - 已知键做类型校验（并额外校验其中的 URL），类型不符直接拒绝保存，
//     因为静默忽略一个写错的规则会让「明明配了选择器却抓不到数据」变得极难排查；
//   - 数字类型接受 float64/int/json.Number/数字字符串，兼容 JSON 反序列化后的形态。
func validateCrawlRules(rules map[string]any) error {
	for key, value := range rules {
		switch key {
		case "careers_url":
			raw, ok := value.(string)
			if !ok {
				return crawlRuleTypeErr(key, "字符串", value)
			}
			if err := validateHTTPURL(key, raw); err != nil {
				return err
			}
		case "careers_link_texts":
			if _, ok := toStringSlice(value); !ok {
				return crawlRuleTypeErr(key, "字符串数组", value)
			}
		case "detail_fields":
			if _, ok := toStringMap(value); !ok {
				return crawlRuleTypeErr(key, "字符串到字符串的对象", value)
			}
		case "force_headless":
			if _, ok := value.(bool); !ok {
				return crawlRuleTypeErr(key, "布尔值", value)
			}
		case "max_pages":
			pages, ok := toIntValue(value)
			if !ok {
				return crawlRuleTypeErr(key, "整数", value)
			}
			if pages < 0 {
				return apperr.ErrInvalidParams.
					WithMessage("crawl_rules.max_pages 不能为负数").
					WithDetails(map[string]any{"field": "crawl_rules.max_pages", "value": value})
			}
		default:
			if _, ok := crawlRuleStringKeys[key]; ok {
				if _, ok := value.(string); !ok {
					return crawlRuleTypeErr(key, "字符串", value)
				}
				continue
			}
			slog.Warn("公司 crawl_rules 含未知键，已原样保存（爬虫会忽略）",
				"key", key, "value", value)
		}
	}
	return nil
}

// crawlRuleTypeErr 构造 crawl_rules 类型错误，details 里指明具体是哪个键不对。
func crawlRuleTypeErr(key, expected string, actual any) error {
	return apperr.ErrInvalidParams.
		WithMessage("crawl_rules." + key + " 类型不正确，期望" + expected).
		WithDetails(map[string]any{
			"field":    "crawl_rules." + key,
			"expected": expected,
			"actual":   actual,
		})
}

// companyWriteErr 统一映射公司写库错误：唯一约束 → 公司已存在。
//
// ent 的唯一索引冲突不会在编译期暴露，只能在写库时按 ConstraintError 识别。
func companyWriteErr(err error, name string) error {
	switch {
	case ent.IsConstraintError(err):
		return apperr.ErrCompanyExists.
			WithMessage("公司名称已存在：" + name).
			WithCause(err)
	case ent.IsValidationError(err):
		// 例如字段超长、枚举非法：属于入参问题而不是服务端故障。
		return apperr.ErrInvalidParams.WithCause(err)
	default:
		return apperr.ErrInternal.WithCause(err)
	}
}
