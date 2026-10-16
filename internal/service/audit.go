package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/auditlog"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// AuditService 负责操作留痕（谁在什么时候改了什么）。
//
// 审计是「旁路能力」：写入失败绝不能让业务请求失败，
// 因此 Record 不返回 error，只在内部记日志。
type AuditService struct{ s *Services }

// AuditListFilter 是审计日志查询条件。
type AuditListFilter struct {
	ActorID    *string
	Action     string
	Resource   string
	ResourceID string
	Success    *bool
	StartAt    *time.Time
	EndAt      *time.Time
	Page       int
	PageSize   int
}

// auditWriteTimeout 是审计落库的独立超时。
//
// 审计写入与业务请求解耦（剥离取消信号）后必须自己兜住耗时，
// 否则数据库抖动时可能长期占用连接。
const auditWriteTimeout = 3 * time.Second

// Record 记录一条审计日志。
//
// 设计取舍：
//   - 用 context.WithoutCancel 派生独立 context：请求被客户端取消或超时时审计仍要落库，
//     否则「失败的越权尝试」这类最需要留痕的操作反而会丢记录；
//   - 内部 panic 与写库错误一律吞掉并记日志，审计失败不能影响业务结果；
//   - 所有字符串按 ent schema 的 MaxLen 截断，避免超长写入被数据库拒绝。
func (svc *AuditService) Record(ctx context.Context, actor Actor, action, resource, resourceID string, detail map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("审计日志写入 panic，已忽略以保护业务请求", "action", action, "panic", r)
		}
	}()

	if svc == nil || svc.s == nil || svc.s.Store == nil || svc.s.Store.Ent == nil {
		return
	}

	// action 在 schema 中是 NotEmpty，这里兜底避免把空动作写成校验错误。
	if strings.TrimSpace(action) == "" {
		action = "unknown"
	}
	if detail == nil {
		detail = map[string]any{}
	}

	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	writeCtx, cancel := context.WithTimeout(base, auditWriteTimeout)
	defer cancel()

	// actor_id 是可空外键：系统任务（UserID 为空串）必须写 NULL，
	// 否则会撞上 users 表的外键约束。
	var actorID *string
	if actor.UserID != "" {
		id := actor.UserID
		actorID = &id
	}

	_, err := svc.s.Store.Ent.AuditLog.Create().
		SetNillableActorID(actorID).
		SetActorEmail(truncateRunes(actor.Email, 255)).
		SetAction(truncateRunes(action, 64)).
		SetResource(truncateRunes(resource, 64)).
		SetResourceID(truncateRunes(resourceID, 64)).
		SetMethod(truncateRunes(actor.Method, 10)).
		SetPath(truncateRunes(actor.Path, 512)).
		SetStatusCode(200).
		SetSuccess(true).
		SetIP(truncateRunes(actor.IP, 64)).
		SetUserAgent(truncateRunes(actor.UserAgent, 512)).
		SetDetail(detail).
		Save(writeCtx)
	if err != nil {
		slog.Error("写入审计日志失败",
			"action", action,
			"resource", resource,
			"resource_id", resourceID,
			"actor_email", actor.Email,
			"error", err,
		)
	}
}

// List 分页查询审计日志，按 id 倒序（最新在前），并预加载操作人边。
//
// action 采用模糊匹配，便于按 "company." 这类前缀筛选一类操作；
// resource / resource_id 是精确定位用的标识，因此用等值匹配。
func (svc *AuditService) List(ctx context.Context, f AuditListFilter) (response.Page[AuditLogView], error) {
	limit, offset, page, pageSize := paginate(f.Page, f.PageSize, 20, 100)

	q := svc.s.Store.Ent.AuditLog.Query()
	if f.ActorID != nil {
		q = q.Where(auditlog.ActorIDEQ(*f.ActorID))
	}
	if action := strings.TrimSpace(f.Action); action != "" {
		q = q.Where(auditlog.ActionContainsFold(action))
	}
	if resource := strings.TrimSpace(f.Resource); resource != "" {
		q = q.Where(auditlog.ResourceEQ(resource))
	}
	if resourceID := strings.TrimSpace(f.ResourceID); resourceID != "" {
		q = q.Where(auditlog.ResourceIDEQ(resourceID))
	}
	if f.Success != nil {
		q = q.Where(auditlog.SuccessEQ(*f.Success))
	}
	if f.StartAt != nil {
		q = q.Where(auditlog.CreateTimeGTE(*f.StartAt))
	}
	if f.EndAt != nil {
		q = q.Where(auditlog.CreateTimeLTE(*f.EndAt))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return response.Page[AuditLogView]{}, apperr.ErrInternal.WithCause(err)
	}

	rows, err := q.Order(ent.Desc(auditlog.FieldID)).
		Offset(offset).
		Limit(limit).
		WithActor().
		All(ctx)
	if err != nil {
		return response.Page[AuditLogView]{}, apperr.ErrInternal.WithCause(err)
	}

	views := make([]AuditLogView, 0, len(rows))
	for _, row := range rows {
		views = append(views, NewAuditLogView(row))
	}
	return response.NewPage(views, total, page, pageSize), nil
}

// fieldChange 构造审计 detail 中的单个字段变更：{"old":..,"new":..}。
//
// 统一结构便于后台直接渲染「旧值 → 新值」，也不需要为每个字段约定格式。
func fieldChange(old, new any) map[string]any {
	return map[string]any{"old": old, "new": new}
}
