package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jsong/jobs-robot/internal/crawler"
	"github.com/jsong/jobs-robot/internal/ent/auditlog"
	"github.com/jsong/jobs-robot/internal/ent/company"
	"github.com/jsong/jobs-robot/internal/ent/crawlrun"
	"github.com/jsong/jobs-robot/internal/ent/jobposting"
	"github.com/jsong/jobs-robot/internal/ent/jobrevision"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
)

// 抓取流水线写入的行必须能回答「是谁写的」：
//   - job_postings.created_by = 爬虫身份（仅新建时写，重抓不改）；
//   - job_revisions.operator_id = 爬虫身份（新建与内容变更两条路径）；
//   - companies.discovered_by = 爬虫身份（爬虫回填招聘入口时写）。
//
// 这里直接调用 CrawlService 的内部落库方法，跳过真实网络抓取：
// 要验证的是「归属字段写没写对」，不是爬虫抽取能力。
func TestCrawlWritesAttributeCrawlerIdentity(t *testing.T) {
	svc := testServices(t)
	ctx := context.Background()
	ids := ensureSystemIdentitiesForTest(t, svc)

	RegisterSystemActor(SystemKeyCrawler, Actor{UserID: ids[SystemKeyCrawler]})
	t.Cleanup(func() { RegisterSystemActor(SystemKeyCrawler, Actor{}) })

	suffix := time.Now().Format("150405.000000000")
	comp, err := svc.Store.Ent.Company.Create().
		SetName("测试公司-" + suffix).
		SetSource("manual").
		Save(ctx)
	if err != nil {
		t.Fatalf("创建测试公司失败: %v", err)
	}

	sourceURL := "https://example.test/jobs/" + suffix
	result := &crawler.Result{
		CompanyID:   comp.ID,
		CompanyName: comp.Name,
		CareersURL:  "https://example.test/careers/" + suffix,
		Jobs: []*crawler.ExtractedJob{
			{
				SourceURL: sourceURL,
				Title:     "测试岗位",
				Category:  "技术",
				City:      "北京",
			},
		},
		Logs: []crawler.LogEntry{{Stage: "discover", URL: "https://example.test/" + suffix}},
	}

	jobID := ""
	t.Cleanup(func() {
		if jobID != "" {
			_, _ = svc.Store.Ent.JobRevision.Delete().
				Where(jobrevision.JobIDEQ(jobID)).Exec(ctx)
			_ = svc.Store.Ent.JobPosting.DeleteOneID(jobID).Exec(ctx)
		}
		_, _ = svc.Store.Ent.AuditLog.Delete().
			Where(auditlog.Or(
				auditlog.ResourceIDEQ(comp.ID),
				auditlog.ResourceIDEQ(jobID),
			)).Exec(ctx)
		_ = svc.Store.Ent.Company.DeleteOneID(comp.ID).Exec(ctx)
	})

	counts := runCounts{}
	if err := svc.Crawl.persistJobs(ctx, comp, result, &counts, time.Now()); err != nil {
		t.Fatalf("岗位入库失败: %v", err)
	}
	if counts.created != 1 {
		t.Fatalf("期望新建 1 个岗位，实际 %+v", counts)
	}

	job, err := svc.Store.Ent.JobPosting.Query().
		Where(jobposting.SourceURLEQ(sourceURL)).Only(ctx)
	if err != nil {
		t.Fatalf("读取岗位失败: %v", err)
	}
	jobID = job.ID
	if job.CreatedBy == nil || *job.CreatedBy != ids[SystemKeyCrawler] {
		t.Fatalf("job.created_by = %v，期望爬虫身份 %s", job.CreatedBy, ids[SystemKeyCrawler])
	}

	revision, err := svc.Store.Ent.JobRevision.Query().
		Where(jobrevision.JobIDEQ(job.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("读取修订记录失败: %v", err)
	}
	if revision.OperatorID == nil || *revision.OperatorID != ids[SystemKeyCrawler] {
		t.Fatalf("job_revision.operator_id = %v，期望爬虫身份 %s", revision.OperatorID, ids[SystemKeyCrawler])
	}

	// 再次抓取（内容未变）不得改写 created_by。
	if err := svc.Crawl.persistJobs(ctx, comp, result, &counts, time.Now()); err != nil {
		t.Fatalf("第二次入库失败: %v", err)
	}
	again, err := svc.Store.Ent.JobPosting.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("重新读取岗位失败: %v", err)
	}
	if again.CreatedBy == nil || *again.CreatedBy != *job.CreatedBy {
		t.Fatalf("重抓不应改写 created_by：%v → %v", job.CreatedBy, again.CreatedBy)
	}

	// 爬虫回填招聘入口时写 discovered_by（公司本身的创建者语义不变）。
	svc.Crawl.updateCompanyAfterCrawl(ctx, comp, result, counts, crawlrun.StatusSuccess, "", time.Now())
	updated, err := svc.Store.Ent.Company.Query().Where(company.IDEQ(comp.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("读取公司失败: %v", err)
	}
	if updated.CareersURL != result.CareersURL {
		t.Errorf("careers_url = %q，期望 %q", updated.CareersURL, result.CareersURL)
	}
	if updated.DiscoveredBy == nil || *updated.DiscoveredBy != ids[SystemKeyCrawler] {
		t.Errorf("company.discovered_by = %v，期望爬虫身份 %s", updated.DiscoveredBy, ids[SystemKeyCrawler])
	}
}

// 后台手工创建的岗位归属操作管理员，而不是爬虫。
func TestAdminCreatedJobAttributesAdmin(t *testing.T) {
	svc := testServices(t)
	ctx := context.Background()

	admin := createUser(t, svc, "job-creator", nil)
	comp, err := svc.Store.Ent.Company.Create().
		SetName("手工岗位测试公司-" + time.Now().Format("150405.000000000")).
		Save(ctx)
	if err != nil {
		t.Fatalf("创建测试公司失败: %v", err)
	}
	jobID := ""
	t.Cleanup(func() {
		if jobID != "" {
			_, _ = svc.Store.Ent.JobRevision.Delete().Where(jobrevision.JobIDEQ(jobID)).Exec(ctx)
			_ = svc.Store.Ent.JobPosting.DeleteOneID(jobID).Exec(ctx)
		}
		_, _ = svc.Store.Ent.AuditLog.Delete().
			Where(auditlog.Or(
				auditlog.ResourceIDEQ(comp.ID),
				auditlog.ResourceIDEQ(jobID),
			)).Exec(ctx)
		_ = svc.Store.Ent.Company.DeleteOneID(comp.ID).Exec(ctx)
	})

	view, err := svc.Job.Create(ctx, JobCreateInput{
		CompanyID: comp.ID,
		Title:     "手工岗位",
		SourceURL: fmt.Sprintf("https://example.test/manual/%d", time.Now().UnixNano()),
	}, Actor{UserID: admin.ID, Email: admin.Email})
	if err != nil {
		t.Fatalf("手工创建岗位失败: %v", err)
	}
	jobID = view.ID

	job, err := svc.Store.Ent.JobPosting.Get(ctx, view.ID)
	if err != nil {
		t.Fatalf("读取岗位失败: %v", err)
	}
	if job.CreatedBy == nil || *job.CreatedBy != admin.ID {
		t.Fatalf("后台创建岗位的 created_by = %v，期望管理员 %s", job.CreatedBy, admin.ID)
	}
	// 顺手确认管理员账号本身没有被误标成系统身份。
	creator, err := svc.Store.Ent.User.Get(ctx, *job.CreatedBy)
	if err != nil {
		t.Fatalf("读取创建者失败: %v", err)
	}
	if creator.Kind != entuser.KindHuman {
		t.Fatalf("创建者 kind = %s，期望 human", creator.Kind)
	}
}
