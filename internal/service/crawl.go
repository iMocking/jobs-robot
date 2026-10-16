package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jsong/jobs-robot/internal/crawler"
	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/crawllog"
	"github.com/jsong/jobs-robot/internal/ent/crawlrun"
	"github.com/jsong/jobs-robot/internal/ent/jobposting"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// CrawlService 负责抓取调度与抓取结果的落库。
//
// 职责边界：真正的页面抓取与字段抽取在 internal/crawler；
// 这里只关心「什么时候抓、抓完怎么入库、入库后进什么审核状态、如何判定岗位下线」。
type CrawlService struct{ s *Services }

// CrawlRunListFilter 是抓取记录列表筛选条件。
type CrawlRunListFilter struct {
	CompanyID string
	Status    string
	Trigger   string
	Page      int
	PageSize  int
}

// releaseLockScript 保证只释放自己持有的锁。
//
// 直接 DEL 有风险：若本次执行超过锁的 TTL，锁已归属他人，盲删会破坏互斥。
var releaseLockScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
end
return 0
`)

// CrawlCompany 抓取单个公司。
//
// 通过 Redis 分布式锁保证同一公司同一时刻只有一个实例在抓取，
// 避免多副本部署或定时任务与手动触发叠加导致对目标站点重复施压。
func (svc *CrawlService) CrawlCompany(ctx context.Context, companyID string, trigger string, actor Actor) (*CrawlRunView, error) {
	if !svc.s.Cfg.Crawler.Enabled {
		return nil, apperr.ErrCrawlerDisabled
	}
	if svc.s.Crawler == nil {
		return nil, apperr.ErrCrawlerDisabled.WithMessage("爬虫未成功初始化，请检查配置与日志")
	}

	comp, err := svc.s.Company.GetEntity(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if !comp.Enabled {
		return nil, apperr.ErrInvalidParams.WithMessage("该公司已停用，无法抓取")
	}
	if !comp.CrawlEnabled {
		return nil, apperr.ErrInvalidParams.WithMessage("该公司未开启抓取")
	}

	// 分布式锁
	lockKey := svc.s.Store.Key("crawl", "lock", "company", companyID)
	lockToken := companyID + ":" + time.Now().Format("20060102150405.000000")
	locked, err := svc.s.Store.Redis.SetNX(ctx, lockKey, lockToken, svc.s.Cfg.Crawler.LockTTL).Result()
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	if !locked {
		return nil, apperr.ErrRunInProgress
	}
	defer func() {
		// 释放锁使用独立上下文，避免请求被取消后锁一直残留到 TTL 到期。
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := releaseLockScript.Run(releaseCtx, svc.s.Store.Redis, []string{lockKey}, lockToken).Err(); err != nil {
			slog.Warn("释放抓取锁失败", "company_id", companyID, "error", err)
		}
	}()

	// 建立运行记录
	now := time.Now()
	run, err := svc.s.Store.Ent.CrawlRun.Create().
		SetCompanyID(companyID).
		SetTrigger(crawlrun.Trigger(normalizeTrigger(trigger))).
		SetStatus(crawlrun.StatusRunning).
		SetStartedAt(now).
		Save(ctx)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	// 组装抓取输入
	// 每次都从配置读取最新的收集范围并同步给爬虫，
	// 这样管理员在后台调整「工作类型/行业白名单」后无需重启即可生效。
	scope, err := svc.s.Setting.CrawlScope(ctx)
	if err != nil {
		svc.finishRun(ctx, run.ID, crawlrun.StatusFailed, "获取收集范围配置失败："+err.Error(), nil, now, runCounts{})
		return nil, err
	}
	svc.s.Crawler.SetScope(scope)
	input := crawler.CompanyInput{
		CompanyID:     comp.ID,
		CompanyName:   comp.Name,
		Website:       comp.Website,
		CareersURL:    comp.CareersURL,
		Industry:      comp.Industry,
		Rules:         siteRulesFromMap(comp.CrawlRules),
		ForceHeadless: comp.ForceHeadless,
		RespectRobots: comp.RespectRobots && svc.s.Cfg.Crawler.RespectRobots,
		MaxPages:      svc.s.Cfg.Crawler.MaxPagesPerSite,
	}

	// 执行抓取（超时保护：单公司最长 10 分钟）
	crawlCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	result, crawlErr := svc.s.Crawler.CrawlCompany(crawlCtx, input)

	// 统计与落库
	counts := runCounts{}
	if result != nil {
		counts.pagesFetched = result.PagesFetched
		// jobs_found 记录「招聘页上发现了多少岗位」，与落库数量区分开，
		// 否则运营从运行记录里看不出「本来有几个、被过滤了几个」。
		counts.jobsFound = result.JobLinksFound
		counts.usedHeadless = result.UsedHeadless
		counts.careersURL = result.CareersURL
		for _, entry := range result.Logs {
			if entry.Stage == "filter" {
				counts.filtered++
			}
		}
	}

	status := crawlrun.StatusSuccess
	var runError string
	if crawlErr != nil {
		runError = crawlErr.Error()
		switch {
		case apperr.Is(crawlErr, apperr.CodeRobotsDenied):
			status = crawlrun.StatusSkipped
		case apperr.Is(crawlErr, apperr.CodeCareersNotFound):
			status = crawlrun.StatusPartial
		default:
			status = crawlrun.StatusFailed
		}
	}

	// 只有在拿到招聘页结果时才入库。
	if result != nil && (crawlErr == nil || status == crawlrun.StatusPartial) {
		if err := svc.persistJobs(crawlCtx, comp, result, &counts, now); err != nil {
			runError = firstNonEmpty(runError, err.Error())
			status = crawlrun.StatusPartial
		}
		expired, err := svc.expireStaleJobs(crawlCtx, comp.ID, now)
		if err != nil {
			slog.Warn("判定岗位下线失败", "company_id", comp.ID, "error", err)
		} else {
			counts.expired = expired
		}
	}

	view := svc.finishRun(ctx, run.ID, status, runError, result, now, counts)
	if view == nil {
		return nil, apperr.ErrInternal.WithMessage("写入抓取结果失败")
	}

	// 回写公司抓取状态
	svc.updateCompanyAfterCrawl(ctx, comp, result, counts, status, runError, now)

	svc.s.Audit.Record(ctx, actor, "crawl.run", "company", comp.ID, map[string]any{
		"run_id":        view.ID,
		"status":        string(status),
		"jobs_created":  view.JobsCreated,
		"jobs_updated":  view.JobsUpdated,
		"jobs_filtered": view.JobsFiltered,
		"trigger":       normalizeTrigger(trigger),
	})

	return view, nil
}

// DiscoverCareers 只定位并回填《加入我们》入口，不抓取岗位。
//
// 运营在录入新公司时先用它确认「入口找对了」，再决定是否开启抓取。
func (svc *CrawlService) DiscoverCareers(ctx context.Context, companyID string, actor Actor) (CompanyView, error) {
	if svc.s.Crawler == nil {
		return CompanyView{}, apperr.ErrCrawlerDisabled
	}
	comp, err := svc.s.Company.GetEntity(ctx, companyID)
	if err != nil {
		return CompanyView{}, err
	}

	input := crawler.CompanyInput{
		CompanyID:     comp.ID,
		CompanyName:   comp.Name,
		Website:       comp.Website,
		CareersURL:    comp.CareersURL,
		Industry:      comp.Industry,
		Rules:         siteRulesFromMap(comp.CrawlRules),
		ForceHeadless: comp.ForceHeadless,
		RespectRobots: comp.RespectRobots && svc.s.Cfg.Crawler.RespectRobots,
		MaxPages:      1, // 只探测入口，不展开抓取
	}

	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	result, err := svc.s.Crawler.CrawlCompany(probeCtx, input)
	if err != nil && (result == nil || result.CareersURL == "") {
		return CompanyView{}, err
	}
	if result == nil || result.CareersURL == "" {
		return CompanyView{}, apperr.ErrCareersNotFound
	}

	discovery := ""
	if len(result.Logs) > 0 {
		discovery = result.Logs[0].URL
	}
	return svc.s.Company.SetCareersURL(ctx, comp.ID, result.CareersURL, discovery, actor)
}

// CrawlAll 抓取全部符合条件的公司（供定时任务调用）。
//
// 并发度由 crawler.workers 控制；单个公司失败不影响其他公司。
func (svc *CrawlService) CrawlAll(ctx context.Context, trigger string) ([]CrawlRunView, error) {
	if !svc.s.Cfg.Crawler.Enabled {
		return nil, apperr.ErrCrawlerDisabled
	}

	candidates, err := svc.s.Company.Candidates(ctx, 0)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		slog.Info("没有需要抓取的公司")
		return []CrawlRunView{}, nil
	}

	workers := svc.s.Cfg.Crawler.Workers
	if workers <= 0 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []CrawlRunView
	)

	for _, comp := range candidates {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(companyID string, name string) {
			defer wg.Done()
			defer func() { <-sem }()

			view, err := svc.CrawlCompany(ctx, companyID, trigger, SystemActor())
			if err != nil {
				// 被锁占用属于正常竞争，不记为失败。
				if apperr.Is(err, apperr.CodeRunInProgress) {
					slog.Info("该公司正在抓取中，跳过", "company_id", companyID, "company", name)
					return
				}
				slog.Warn("抓取公司失败", "company_id", companyID, "company", name, "error", err)
				svc.s.Audit.Record(ctx, SystemActor(), "crawl.failed", "company", companyID,
					map[string]any{"error": err.Error()})
				return
			}
			mu.Lock()
			results = append(results, *view)
			mu.Unlock()
		}(comp.ID, comp.Name)
	}

	wg.Wait()
	slog.Info("本轮全量抓取结束", "companies", len(candidates), "runs", len(results))
	return results, nil
}

// ExpireStaleJobs 全量判定：长期未在任何来源页面出现的岗位标记为已过期。
func (svc *CrawlService) ExpireStaleJobs(ctx context.Context) (int, error) {
	ttl := svc.s.Cfg.Crawler.JobExpireAfter
	if ttl <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-ttl)
	n, err := svc.s.Store.Ent.JobPosting.Update().
		Where(
			jobposting.StatusIn(jobposting.StatusApproved, jobposting.StatusPendingReview),
			jobposting.LastSeenAtLT(cutoff),
		).
		SetStatus(jobposting.StatusExpired).
		SetExpiredAt(time.Now()).
		Save(ctx)
	if err != nil {
		return 0, apperr.ErrInternal.WithCause(err)
	}
	if n > 0 {
		slog.Info("已标记过期岗位", "count", n, "cutoff", cutoff.Format(time.RFC3339))
	}
	return n, nil
}

// ListRuns 分页查询抓取记录。
func (svc *CrawlService) ListRuns(ctx context.Context, f CrawlRunListFilter) (response.Page[CrawlRunView], error) {
	query := svc.s.Store.Ent.CrawlRun.Query().WithCompany()

	if f.CompanyID != "" {
		query = query.Where(crawlrun.CompanyIDEQ(f.CompanyID))
	}
	if f.Status != "" {
		query = query.Where(crawlrun.StatusEQ(crawlrun.Status(f.Status)))
	}
	if f.Trigger != "" {
		query = query.Where(crawlrun.TriggerEQ(crawlrun.Trigger(f.Trigger)))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return response.Page[CrawlRunView]{}, apperr.ErrInternal.WithCause(err)
	}

	limit, offset, page, size := paginate(f.Page, f.PageSize, 20, 100)
	rows, err := query.Order(ent.Desc(crawlrun.FieldID)).Limit(limit).Offset(offset).All(ctx)
	if err != nil {
		return response.Page[CrawlRunView]{}, apperr.ErrInternal.WithCause(err)
	}

	views := make([]CrawlRunView, 0, len(rows))
	for _, r := range rows {
		views = append(views, NewCrawlRunView(r))
	}
	return response.NewPage(views, total, page, size), nil
}

// RunDetail 返回单次抓取记录及其明细日志。
func (svc *CrawlService) RunDetail(ctx context.Context, id string) (CrawlRunView, []CrawlLogView, error) {
	run, err := svc.s.Store.Ent.CrawlRun.Query().
		Where(crawlrun.IDEQ(id)).WithCompany().Only(ctx)
	if ent.IsNotFound(err) {
		return CrawlRunView{}, nil, apperr.ErrRunNotFound
	}
	if err != nil {
		return CrawlRunView{}, nil, apperr.ErrInternal.WithCause(err)
	}

	logs, err := svc.s.Store.Ent.CrawlLog.Query().
		Where(crawllog.RunIDEQ(id)).
		Order(ent.Asc(crawllog.FieldID)).
		Limit(500).
		All(ctx)
	if err != nil {
		return CrawlRunView{}, nil, apperr.ErrInternal.WithCause(err)
	}

	views := make([]CrawlLogView, 0, len(logs))
	for _, l := range logs {
		views = append(views, NewCrawlLogView(l))
	}
	return NewCrawlRunView(run), views, nil
}

// —— 内部实现 ——

// runCounts 汇总一次抓取的各项计数。
type runCounts struct {
	pagesFetched int
	jobsFound    int
	created      int
	updated      int
	unchanged    int
	filtered     int
	expired      int
	usedHeadless bool
	careersURL   string
}

// persistJobs 把抓取结果写入数据库，并维护内容变更留痕。
func (svc *CrawlService) persistJobs(ctx context.Context, comp *ent.Company, result *crawler.Result, counts *runCounts, now time.Time) error {
	for _, ej := range result.Jobs {
		outcome, err := svc.upsertJob(ctx, comp, ej, now)
		if err != nil {
			slog.Warn("岗位入库失败", "url", ej.SourceURL, "error", err)
			continue
		}
		switch outcome {
		case outcomeCreated:
			counts.created++
		case outcomeUpdated:
			counts.updated++
		default:
			counts.unchanged++
		}
	}
	return nil
}

type upsertOutcome string

const (
	outcomeCreated   upsertOutcome = "created"
	outcomeUpdated   upsertOutcome = "updated"
	outcomeUnchanged upsertOutcome = "unchanged"
)

// upsertJob 以 source_url 为唯一键做「新增 / 更新 / 无变化」判定。
//
// 变更判定的依据是内容指纹（见 revision.go），保证与人工编辑路径口径一致。
func (svc *CrawlService) upsertJob(ctx context.Context, comp *ent.Company, ej *crawler.ExtractedJob, now time.Time) (upsertOutcome, error) {
	fields := extractedContentFields(ej)
	newHash := fields.hash()
	score := crawler.ScoreJob(ej)

	existing, err := svc.s.Store.Ent.JobPosting.Query().
		Where(jobposting.SourceURLEQ(ej.SourceURL)).
		Only(ctx)

	if ent.IsNotFound(err) {
		builder := svc.s.Store.Ent.JobPosting.Create().
			SetCompanyID(comp.ID).
			SetSourceURL(ej.SourceURL).
			SetSourceSite(hostOf(ej.SourceURL)).
			SetSourceCompanyName(firstNonEmpty(ej.SourceCompanyName, comp.Name)).
			SetSourceJobID(truncateRunes(ej.SourceJobID, 128)).
			SetIsDirectHire(true).
			SetJobCode(truncateRunes(ej.JobCode, 64)).
			SetIndustry(firstNonEmpty(ej.Industry, comp.Industry)).
			SetDepartment(truncateRunes(ej.Department, 128)).
			SetWorkMode(truncateRunes(ej.WorkMode, 32)).
			SetSummary(truncateRunes(ej.Summary, 512)).
			SetProvince(truncateRunes(ej.Province, 64)).
			SetLocationText(truncateRunes(ej.LocationText, 255)).
			SetContactPerson(truncateRunes(ej.ContactPerson, 64)).
			SetContactWechat(truncateRunes(ej.ContactWechat, 64)).
			SetApplyMethod(truncateRunes(ej.ApplyMethod, 64)).
			SetStatus(jobposting.StatusPendingReview).
			SetRevision(1).
			SetContentHash(newHash).
			SetQualityScore(score).
			SetParseConfidence(ej.Confidence).
			SetExtractor(truncateRunes(ej.Extractor, 32)).
			SetCrawledAt(now).
			SetLastSeenAt(now).
			SetLastUpdatedAt(now)
		if ej.Raw != nil {
			builder = builder.SetRawJSON(ej.Raw)
		}
		builder = applyContentFieldsToCreate(builder, fields)
		if ej.Country != "" {
			builder = builder.SetCountry(truncateRunes(ej.Country, 64))
		}
		if ej.Headcount != nil {
			builder = builder.SetHeadcount(*ej.Headcount)
		}
		if ej.SalaryMin != nil {
			builder = builder.SetSalaryMin(*ej.SalaryMin)
		}
		if ej.SalaryMax != nil {
			builder = builder.SetSalaryMax(*ej.SalaryMax)
		}
		if ej.SalaryMonths != nil {
			builder = builder.SetSalaryMonths(*ej.SalaryMonths)
		}
		if ej.DeadlineAt != nil {
			builder = builder.SetDeadlineAt(*ej.DeadlineAt)
		}
		if ej.SourceUpdatedAt != nil {
			builder = builder.SetSourceUpdatedAt(*ej.SourceUpdatedAt)
		}

		created, err := builder.Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				// 并发插入同一岗位：视为已存在，下一轮会走更新分支。
				return outcomeUnchanged, nil
			}
			return "", apperr.ErrInternal.WithCause(err)
		}
		// 新岗位直接生成一条初始修订，保证「最后更新时间」有据可查。
		if err := recordJobRevision(ctx, svc.s, created.ID, 1, newHash, nil,
			fields.snapshot(), created.SourceURL, "crawl", nil); err != nil {
			slog.Warn("写入初始修订记录失败", "job_id", created.ID, "error", err)
		}
		return outcomeCreated, nil
	}
	if err != nil {
		return "", apperr.ErrInternal.WithCause(err)
	}

	// 内容未变化：只刷新「最近仍在架」时间，避免无意义地改动 last_updated_at。
	if existing.ContentHash == newHash {
		if _, err := svc.s.Store.Ent.JobPosting.UpdateOneID(existing.ID).
			SetLastSeenAt(now).
			SetCrawledAt(now).
			Save(ctx); err != nil {
			return "", apperr.ErrInternal.WithCause(err)
		}
		return outcomeUnchanged, nil
	}

	before := jobContentFields(existing).snapshot()
	changed := changedFieldNames(before, fields.snapshot())
	revision := existing.Revision + 1

	// 状态决策：实质性变更退回待审核；已下线/过期的岗位重新出现也回到待审核。
	status := existing.Status
	switch {
	case existing.Status == jobposting.StatusExpired,
		existing.Status == jobposting.StatusOffline,
		existing.Status == jobposting.StatusRejected:
		status = jobposting.StatusPendingReview
	case existing.Status == jobposting.StatusApproved && needsReReview(changed):
		status = jobposting.StatusPendingReview
	}

	updater := svc.s.Store.Ent.JobPosting.UpdateOneID(existing.ID).
		SetRevision(revision).
		SetContentHash(newHash).
		SetQualityScore(score).
		SetCrawledAt(now).
		SetLastSeenAt(now).
		SetLastUpdatedAt(now).
		SetStatus(status).
		SetIndustry(firstNonEmpty(ej.Industry, comp.Industry)).
		SetSourceCompanyName(firstNonEmpty(ej.SourceCompanyName, comp.Name)).
		SetParseConfidence(ej.Confidence).
		SetExtractor(truncateRunes(ej.Extractor, 32))
	if status == jobposting.StatusPendingReview && existing.Status != jobposting.StatusPendingReview {
		updater = updater.SetRejectReason("")
	}
	if ej.Raw != nil {
		updater = updater.SetRawJSON(ej.Raw)
	}
	updater = applyContentFieldsToUpdate(updater, fields)
	if ej.SalaryMin != nil {
		updater = updater.SetSalaryMin(*ej.SalaryMin)
	}
	if ej.SalaryMax != nil {
		updater = updater.SetSalaryMax(*ej.SalaryMax)
	}
	if ej.SalaryMonths != nil {
		updater = updater.SetSalaryMonths(*ej.SalaryMonths)
	}
	if ej.Headcount != nil {
		updater = updater.SetHeadcount(*ej.Headcount)
	}
	if ej.DeadlineAt != nil {
		updater = updater.SetDeadlineAt(*ej.DeadlineAt)
	}
	if ej.SourceUpdatedAt != nil {
		updater = updater.SetSourceUpdatedAt(*ej.SourceUpdatedAt)
	}

	if _, err := updater.Save(ctx); err != nil {
		return "", apperr.ErrInternal.WithCause(err)
	}
	if err := recordJobRevision(ctx, svc.s, existing.ID, revision, newHash, changed,
		fields.snapshot(), ej.SourceURL, "crawl", nil); err != nil {
		slog.Warn("写入修订记录失败", "job_id", existing.ID, "error", err)
	}
	return outcomeUpdated, nil
}

// expireStaleJobs 把该公司长期未再出现的岗位标记为已过期，返回受影响的岗位数。
func (svc *CrawlService) expireStaleJobs(ctx context.Context, companyID string, now time.Time) (int, error) {
	ttl := svc.s.Cfg.Crawler.JobExpireAfter
	if ttl <= 0 {
		return 0, nil
	}
	cutoff := now.Add(-ttl)
	n, err := svc.s.Store.Ent.JobPosting.Update().
		Where(
			jobposting.CompanyIDEQ(companyID),
			jobposting.StatusIn(jobposting.StatusApproved, jobposting.StatusPendingReview),
			jobposting.LastSeenAtLT(cutoff),
		).
		SetStatus(jobposting.StatusExpired).
		SetExpiredAt(now).
		Save(ctx)
	if err != nil {
		return 0, apperr.ErrInternal.WithCause(err)
	}
	return n, nil
}

// finishRun 结束运行记录并写入明细日志。
func (svc *CrawlService) finishRun(
	ctx context.Context,
	runID string,
	status crawlrun.Status,
	runError string,
	result *crawler.Result,
	startedAt time.Time,
	counts runCounts,
) *CrawlRunView {
	finishedAt := time.Now()
	// 运行记录的收尾必须落库，因此用独立上下文，避免请求取消导致记录永远停在 running。
	finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	updater := svc.s.Store.Ent.CrawlRun.UpdateOneID(runID).
		SetStatus(status).
		SetFinishedAt(finishedAt).
		SetDurationMs(finishedAt.Sub(startedAt).Milliseconds()).
		SetPagesFetched(counts.pagesFetched).
		SetJobsFound(counts.jobsFound).
		SetJobsCreated(counts.created).
		SetJobsUpdated(counts.updated).
		SetJobsUnchanged(counts.unchanged).
		SetJobsFiltered(counts.filtered).
		SetJobsExpired(counts.expired).
		SetUsedHeadless(counts.usedHeadless).
		SetErrorMessage(truncateRunes(runError, 1024))
	if counts.careersURL != "" {
		updater = updater.SetCareersURL(truncateRunes(counts.careersURL, 512))
	}

	run, err := updater.Save(finalCtx)
	if err != nil {
		slog.Error("写入抓取结果失败", "run_id", runID, "error", err)
		return nil
	}

	if result != nil && len(result.Logs) > 0 {
		builders := make([]*ent.CrawlLogCreate, 0, len(result.Logs))
		for _, entry := range result.Logs {
			builder := svc.s.Store.Ent.CrawlLog.Create().
				SetRunID(runID).
				SetLevel(crawllog.Level(normalizeLogLevel(entry.Level))).
				SetStage(truncateRunes(entry.Stage, 32)).
				SetURL(truncateRunes(entry.URL, 1024)).
				SetMessage(truncateRunes(entry.Message, 1024))
			if entry.Context != nil {
				builder = builder.SetContext(entry.Context)
			}
			builders = append(builders, builder)
		}
		if _, err := svc.s.Store.Ent.CrawlLog.CreateBulk(builders...).Save(finalCtx); err != nil {
			slog.Warn("写入抓取明细日志失败", "run_id", runID, "error", err)
		}
	}

	view := NewCrawlRunView(run)
	return &view
}

// updateCompanyAfterCrawl 回写公司抓取状态与岗位计数。
func (svc *CrawlService) updateCompanyAfterCrawl(
	ctx context.Context,
	comp *ent.Company,
	result *crawler.Result,
	counts runCounts,
	status crawlrun.Status,
	runError string,
	now time.Time,
) {
	updater := svc.s.Store.Ent.Company.UpdateOneID(comp.ID).
		SetLastCrawledAt(now).
		SetLastCrawlStatus(string(status)).
		SetLastCrawlError(truncateRunes(runError, 1024))

	// 发现到新的招聘入口时回填，便于运营核对与后续直接使用。
	if result != nil && result.CareersURL != "" && result.CareersURL != comp.CareersURL {
		updater = updater.SetCareersURL(truncateRunes(result.CareersURL, 512))
		if len(result.Logs) > 0 {
			updater = updater.SetDiscoveryURL(truncateRunes(result.Logs[0].URL, 512))
		}
	}

	if _, err := updater.Save(ctx); err != nil {
		slog.Warn("回写公司抓取状态失败", "company_id", comp.ID, "error", err)
	}
	if err := svc.s.Company.RefreshJobCount(ctx, comp.ID); err != nil {
		slog.Warn("刷新公司岗位数失败", "company_id", comp.ID, "error", err)
	}
}

// siteRulesFromMap 把公司配置中的 crawl_rules（JSON）转换为站点抓取规则。
//
// 采用宽松解析：类型不符的键会被忽略，避免因为一个笔误导致整家公司无法抓取。
func siteRulesFromMap(raw map[string]any) *crawler.SiteRules {
	if len(raw) == 0 {
		return &crawler.SiteRules{}
	}
	rules := &crawler.SiteRules{
		CareersURL:          mapString(raw, "careers_url"),
		CareersLinkSelector: mapString(raw, "careers_link_selector"),
		CareersLinkTexts:    ruleStringSlice(raw, "careers_link_texts"),
		ListSelector:        mapString(raw, "list_selector"),
		ItemSelector:        mapString(raw, "item_selector"),
		LinkSelector:        mapString(raw, "link_selector"),
		DetailFields:        ruleStringMap(raw, "detail_fields"),
		ForceHeadless:       mapBool(raw, "force_headless"),
		MaxPages:            mapInt(raw, "max_pages"),
	}
	return rules
}

// 以下三个是从 map[string]any（JSONB 反序列化结果）中安全取值的通用工具，
// 邮件配置等其他「数据库中的 JSON 配置」也复用它们。
func mapString(raw map[string]any, key string) string {
	if v, ok := raw[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func mapBool(raw map[string]any, key string) bool {
	if v, ok := raw[key].(bool); ok {
		return v
	}
	return false
}

func mapInt(raw map[string]any, key string) int {
	switch v := raw[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func ruleStringSlice(raw map[string]any, key string) []string {
	list, ok := raw[key].([]any)
	if !ok {
		if direct, ok := raw[key].([]string); ok {
			return direct
		}
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func ruleStringMap(raw map[string]any, key string) map[string]string {
	nested, ok := raw[key].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(nested))
	for k, v := range nested {
		if s, ok := v.(string); ok {
			out[k] = strings.TrimSpace(s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// normalizeTrigger 归一化触发来源，未知值一律按定时任务处理。
func normalizeTrigger(trigger string) string {
	switch crawlrun.Trigger(strings.ToLower(strings.TrimSpace(trigger))) {
	case crawlrun.TriggerSchedule, crawlrun.TriggerManual, crawlrun.TriggerRetry:
		return strings.ToLower(strings.TrimSpace(trigger))
	default:
		return string(crawlrun.TriggerSchedule)
	}
}

// normalizeLogLevel 归一化日志级别。
func normalizeLogLevel(level string) string {
	switch crawllog.Level(strings.ToLower(strings.TrimSpace(level))) {
	case crawllog.LevelDebug, crawllog.LevelInfo, crawllog.LevelWarn, crawllog.LevelError:
		return strings.ToLower(strings.TrimSpace(level))
	default:
		return string(crawllog.LevelInfo)
	}
}

// applyContentFieldsToCreate 把规范内容字段写入创建构造器。
func applyContentFieldsToCreate(builder *ent.JobPostingCreate, f contentFields) *ent.JobPostingCreate {
	builder = builder.
		SetTitle(truncateRunes(f.Title, 255)).
		SetCategory(truncateRunes(f.Category, 64)).
		SetEmploymentType(truncateRunes(f.EmploymentType, 32)).
		SetCity(truncateRunes(f.City, 64)).
		SetDistrict(truncateRunes(f.District, 64)).
		SetAddress(truncateRunes(f.Address, 512)).
		SetSalaryText(truncateRunes(f.SalaryText, 128)).
		SetExperience(truncateRunes(f.Experience, 64)).
		SetEducation(truncateRunes(f.Education, 64)).
		SetDescription(f.Description).
		SetResponsibilities(f.Responsibilities).
		SetRequirements(f.Requirements).
		SetBenefits(f.Benefits).
		SetContactEmail(truncateRunes(f.ContactEmail, 255)).
		SetContactPhone(truncateRunes(f.ContactPhone, 64)).
		SetApplyURL(truncateRunes(f.ApplyURL, 1024))
	if f.RequirementsList != nil {
		builder = builder.SetRequirementsList(f.RequirementsList)
	}
	if f.PublishedAt != nil {
		builder = builder.SetPublishedAt(*f.PublishedAt)
	}
	return builder
}

// applyContentFieldsToUpdate 把规范内容字段写入更新构造器。
func applyContentFieldsToUpdate(updater *ent.JobPostingUpdateOne, f contentFields) *ent.JobPostingUpdateOne {
	updater = updater.
		SetTitle(truncateRunes(f.Title, 255)).
		SetCategory(truncateRunes(f.Category, 64)).
		SetEmploymentType(truncateRunes(f.EmploymentType, 32)).
		SetCity(truncateRunes(f.City, 64)).
		SetDistrict(truncateRunes(f.District, 64)).
		SetAddress(truncateRunes(f.Address, 512)).
		SetSalaryText(truncateRunes(f.SalaryText, 128)).
		SetExperience(truncateRunes(f.Experience, 64)).
		SetEducation(truncateRunes(f.Education, 64)).
		SetDescription(f.Description).
		SetResponsibilities(f.Responsibilities).
		SetRequirements(f.Requirements).
		SetBenefits(f.Benefits).
		SetContactEmail(truncateRunes(f.ContactEmail, 255)).
		SetContactPhone(truncateRunes(f.ContactPhone, 64)).
		SetApplyURL(truncateRunes(f.ApplyURL, 1024))
	// 切片字段必须显式清空，否则旧的任职要求条目会残留在结构化数组里。
	if f.RequirementsList != nil {
		updater = updater.SetRequirementsList(f.RequirementsList)
	} else {
		updater = updater.SetRequirementsList([]string{})
	}
	if f.PublishedAt != nil {
		updater = updater.SetPublishedAt(*f.PublishedAt)
	}
	return updater
}
