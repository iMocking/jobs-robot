package service

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/jsong/jobs-robot/internal/crawler"
	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// 本文件负责岗位内容的「规范化 — 指纹 — 比对 — 留痕」。
//
// 为什么需要它：抓取来的岗位经常被企业方悄悄修改（薪资调整、地点变更、要求放宽）。
// 只保存最终值会让「最后更新时间」变成一个无法解释的字段；有了修订记录，
// 就能回答「什么时候、因为哪次抓取、具体改了什么」。
//
// 关键设计：contentFields 是唯一的内容规范形式，
// 抓取结果与数据库实体都先转换到它，再计算指纹与比对，
// 从而保证两条写入路径（爬虫入库 / 人工编辑）产生完全一致的变更判定。

// contentFields 是参与内容指纹与变更比对的标准字段集合。
//
// 刻意排除爬取噪声字段（content_hash、crawled_at、parse_confidence、raw_json 等），
// 否则每次抓取都会因为元数据变化而被误判为「内容更新」。
type contentFields struct {
	Title            string
	Category         string
	EmploymentType   string
	City             string
	District         string
	Address          string
	SalaryText       string
	Experience       string
	Education        string
	Description      string
	Responsibilities string
	Requirements     string
	RequirementsList []string
	Benefits         string
	ContactEmail     string
	ContactPhone     string
	ApplyURL         string
	PublishedAt      *time.Time
}

// hash 计算内容指纹。
func (c contentFields) hash() string {
	return hashContent(
		c.Title,
		c.Category,
		c.EmploymentType,
		c.City,
		c.District,
		c.Address,
		c.SalaryText,
		c.Experience,
		c.Education,
		c.Description,
		c.Responsibilities,
		c.Requirements,
		joinStrings(c.RequirementsList),
		c.Benefits,
		c.ContactEmail,
		c.ContactPhone,
		c.ApplyURL,
		timeKey(c.PublishedAt),
	)
}

// snapshot 生成用于变更比对与修订留痕的快照。
func (c contentFields) snapshot() map[string]any {
	return map[string]any{
		"title":             c.Title,
		"category":          c.Category,
		"employment_type":   c.EmploymentType,
		"city":              c.City,
		"district":          c.District,
		"address":           c.Address,
		"salary_text":       c.SalaryText,
		"experience":        c.Experience,
		"education":         c.Education,
		"description":       c.Description,
		"responsibilities":  c.Responsibilities,
		"requirements":      c.Requirements,
		"requirements_list": c.RequirementsList,
		"benefits":          c.Benefits,
		"contact_email":     c.ContactEmail,
		"contact_phone":     c.ContactPhone,
		"apply_url":         c.ApplyURL,
		"published_at":      timeKey(c.PublishedAt),
	}
}

// jobContentFields 从数据库实体提取规范字段。
func jobContentFields(j *ent.JobPosting) contentFields {
	if j == nil {
		return contentFields{}
	}
	return contentFields{
		Title:            j.Title,
		Category:         j.Category,
		EmploymentType:   j.EmploymentType,
		City:             j.City,
		District:         j.District,
		Address:          j.Address,
		SalaryText:       j.SalaryText,
		Experience:       j.Experience,
		Education:        j.Education,
		Description:      j.Description,
		Responsibilities: j.Responsibilities,
		Requirements:     j.Requirements,
		RequirementsList: j.RequirementsList,
		Benefits:         j.Benefits,
		ContactEmail:     j.ContactEmail,
		ContactPhone:     j.ContactPhone,
		ApplyURL:         j.ApplyURL,
		PublishedAt:      j.PublishedAt,
	}
}

// extractedContentFields 从抓取结果提取规范字段。
func extractedContentFields(e *crawler.ExtractedJob) contentFields {
	if e == nil {
		return contentFields{}
	}
	return contentFields{
		Title:            e.Title,
		Category:         e.Category,
		EmploymentType:   e.EmploymentType,
		City:             e.City,
		District:         e.District,
		Address:          e.Address,
		SalaryText:       e.SalaryText,
		Experience:       e.Experience,
		Education:        e.Education,
		Description:      e.Description,
		Responsibilities: e.Responsibilities,
		Requirements:     e.Requirements,
		RequirementsList: e.RequirementsList,
		Benefits:         e.Benefits,
		ContactEmail:     e.ContactEmail,
		ContactPhone:     e.ContactPhone,
		ApplyURL:         e.ApplyURL,
		PublishedAt:      e.PublishedAt,
	}
}

// jobSnapshot 返回岗位的变更比对快照。
func jobSnapshot(j *ent.JobPosting) map[string]any {
	return jobContentFields(j).snapshot()
}

// jobContentHash 计算岗位内容指纹。
func jobContentHash(j *ent.JobPosting) string {
	return jobContentFields(j).hash()
}

// changedFieldNames 比较两个快照，返回发生变化的字段名（按字典序，保证结果稳定）。
func changedFieldNames(before, after map[string]any) []string {
	var changed []string
	for key, newValue := range after {
		oldValue, ok := before[key]
		if !ok || !reflect.DeepEqual(oldValue, newValue) {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	return changed
}

// substantiveFields 是「实质性变更」字段集合。
//
// 已发布岗位发生这些字段的变更时，会被退回待审核状态：
// 薪资、任职要求、工作地址这类信息一旦变化，之前的人工审核结论就不再成立。
// 而诸如补充一句简介、加个标签这类变更不打扰审核流程，只留痕。
var substantiveFields = map[string]struct{}{
	"title":             {},
	"requirements":      {},
	"requirements_list": {},
	"responsibilities":  {},
	"salary_text":       {},
	"employment_type":   {},
	"address":           {},
	"city":              {},
}

// needsReReview 判断变更是否属于实质性变更。
func needsReReview(changed []string) bool {
	for _, field := range changed {
		if _, ok := substantiveFields[field]; ok {
			return true
		}
	}
	return false
}

// recordJobRevision 写入一条变更记录。
//
// 做成自由函数而不是某个服务的方法：抓取入库（CrawlService）与人工编辑（JobService）
// 都要写修订记录，挂在任一服务上都会让另一方产生无谓的依赖。
func recordJobRevision(
	ctx context.Context,
	s *Services,
	jobID string, revision int,
	contentHash string,
	changed []string,
	snapshot map[string]any,
	sourceURL, trigger string,
	operatorID *string,
) error {
	builder := s.Store.Ent.JobRevision.Create().
		SetJobID(jobID).
		SetRevision(revision).
		SetContentHash(contentHash).
		SetChangedFields(changed).
		SetSnapshot(snapshot).
		SetSourceURL(sourceURL).
		SetTrigger(trigger)
	if operatorID != nil {
		builder = builder.SetOperatorID(*operatorID)
	}
	if _, err := builder.Save(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// joinStrings 以换行拼接切片，用于内容指纹。
func joinStrings(list []string) string {
	out := ""
	for _, item := range list {
		out += item + "\n"
	}
	return out
}
