package service

import (
	"context"
	"strings"
	"time"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/casbin"
	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/auditlog"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
	"github.com/jsong/jobs-robot/internal/ent/verificationcode"
	"github.com/jsong/jobs-robot/internal/mail"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
)

// UserService 负责管理端的用户与角色管理。
type UserService struct{ s *Services }

// UserListFilter 是用户列表筛选条件。
type UserListFilter struct {
	Keyword  string
	Status   string
	Role     string
	OrderBy  string // created_at | last_login_at | email
	Page     int
	PageSize int
}

// List 分页查询用户。
func (svc *UserService) List(ctx context.Context, f UserListFilter) (response.Page[UserView], error) {
	query := svc.s.Store.Ent.User.Query()

	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		query = query.Where(entuser.Or(
			entuser.EmailContainsFold(kw),
			entuser.NicknameContainsFold(kw),
		))
	}
	if f.Status != "" {
		query = query.Where(entuser.StatusEQ(entuser.Status(f.Status)))
	}
	if f.Role != "" {
		ids, err := svc.s.Authz.UsersForRole(f.Role)
		if err != nil {
			return response.Page[UserView]{}, err
		}
		if len(ids) == 0 {
			// 该角色下没有用户，直接返回空页，避免生成恒假条件。
			_, _, page, size := paginate(f.Page, f.PageSize, 20, 100)
			return response.NewPage([]UserView{}, 0, page, size), nil
		}
		query = query.Where(entuser.IDIn(ids...))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return response.Page[UserView]{}, apperr.ErrInternal.WithCause(err)
	}

	limit, offset, page, size := paginate(f.Page, f.PageSize, 20, 100)
	switch f.OrderBy {
	case "email":
		query = query.Order(ent.Asc(entuser.FieldEmail))
	case "last_login_at":
		query = query.Order(ent.Desc(entuser.FieldLastLoginAt))
	default:
		query = query.Order(ent.Desc(entuser.FieldID))
	}

	users, err := query.Limit(limit).Offset(offset).All(ctx)
	if err != nil {
		return response.Page[UserView]{}, apperr.ErrInternal.WithCause(err)
	}

	views := make([]UserView, 0, len(users))
	for _, u := range users {
		roles, err := svc.s.Authz.RolesForUser(u.ID)
		if err != nil {
			return response.Page[UserView]{}, err
		}
		views = append(views, NewUserView(u, roles))
	}
	return response.NewPage(views, total, page, size), nil
}

// Get 查询单个用户。
func (svc *UserService) Get(ctx context.Context, id string) (UserView, error) {
	user, err := svc.s.Store.Ent.User.Get(ctx, id)
	if ent.IsNotFound(err) {
		return UserView{}, apperr.ErrUserNotFound
	}
	if err != nil {
		return UserView{}, apperr.ErrInternal.WithCause(err)
	}
	roles, err := svc.s.Authz.RolesForUser(id)
	if err != nil {
		return UserView{}, err
	}
	return NewUserView(user, roles), nil
}

// SetStatus 启用/禁用用户。
func (svc *UserService) SetStatus(ctx context.Context, id string, status string, actor Actor) (UserView, error) {
	switch entuser.Status(status) {
	case entuser.StatusActive, entuser.StatusDisabled, entuser.StatusPending, entuser.StatusLocked:
	default:
		return UserView{}, apperr.ErrInvalidParams.WithMessage("不支持的用户状态：" + status)
	}
	if id == actor.UserID {
		return UserView{}, apperr.ErrInvalidParams.WithMessage("不能修改自己的账号状态")
	}
	if _, err := svc.mutableUser(ctx, id); err != nil {
		return UserView{}, err
	}

	updater := svc.s.Store.Ent.User.UpdateOneID(id).SetStatus(entuser.Status(status))
	// 解锁时同时清零失败计数与锁定时间，否则会被立即重新锁定。
	if entuser.Status(status) == entuser.StatusActive {
		updater = updater.SetFailedLoginCount(0).SetNillableLockedUntil(nil)
	}

	user, err := updater.Save(ctx)
	if ent.IsNotFound(err) {
		return UserView{}, apperr.ErrUserNotFound
	}
	if err != nil {
		return UserView{}, apperr.ErrInternal.WithCause(err)
	}

	// 禁用用户时立即吊销其全部登录会话与令牌。
	if entuser.Status(status) == entuser.StatusDisabled {
		_ = svc.s.Sessions.BumpPasswordVersion(ctx, id)
		_ = svc.s.Sessions.RevokeAllExcept(ctx, id, "")
	}

	roles, err := svc.s.Authz.RolesForUser(id)
	if err != nil {
		return UserView{}, err
	}
	svc.s.Audit.Record(ctx, actor, "user.set_status", "user", id,
		map[string]any{"status": status})
	return NewUserView(user, roles), nil
}

// SetRoles 覆盖式设置用户角色。
func (svc *UserService) SetRoles(ctx context.Context, id string, roles []string, actor Actor) (UserView, error) {
	if len(roles) == 0 {
		return UserView{}, apperr.ErrInvalidParams.WithMessage("至少需要保留一个角色")
	}
	for _, r := range roles {
		if !casbin.IsBuiltinRole(r) {
			return UserView{}, apperr.ErrInvalidParams.WithMessage("不支持的角色：" + r)
		}
	}
	if _, err := svc.mutableUser(ctx, id); err != nil {
		return UserView{}, err
	}

	current, err := svc.s.Authz.RolesForUser(id)
	if err != nil {
		return UserView{}, err
	}
	// 不允许把最后一个管理员降级，否则系统将永久失去管理入口。
	if containsString(current, casbin.RoleAdmin) && !containsString(roles, casbin.RoleAdmin) {
		if err := svc.assertNotLastAdmin(ctx, id); err != nil {
			return UserView{}, err
		}
	}

	if err := svc.s.Authz.SetRoles(id, roles); err != nil {
		return UserView{}, err
	}

	user, err := svc.s.Store.Ent.User.Get(ctx, id)
	if ent.IsNotFound(err) {
		return UserView{}, apperr.ErrUserNotFound
	}
	if err != nil {
		return UserView{}, apperr.ErrInternal.WithCause(err)
	}

	// 角色变化后使既有令牌失效，避免旧令牌继续携带旧角色。
	_ = svc.s.Sessions.BumpPasswordVersion(ctx, id)

	svc.s.Audit.Record(ctx, actor, "user.set_roles", "user", id,
		map[string]any{"old": current, "new": roles})

	// 变更通知：发往被变更账号邮箱与操作人邮箱（邮件失败不阻断主流程，
	// sendSystemMail 会在审计里记录 success=false 供排查）。
	svc.notifyRoleChanged(ctx, actor, user, current, roles)

	return NewUserView(user, roles), nil
}

// roleLabel 返回内置角色的中文展示名（与前端 ROLE_LABEL 保持一致）。
func roleLabel(role string) string {
	switch role {
	case casbin.RoleAdmin:
		return "管理员"
	case casbin.RoleReviewer:
		return "审核员"
	case casbin.RoleUser:
		return "正式会员"
	case casbin.RoleSystem:
		return "系统"
	default:
		return role
	}
}

// joinRoleLabels 把角色列表拼接成「A、B」形式，用于通知邮件正文。
func joinRoleLabels(roles []string) string {
	labels := make([]string, 0, len(roles))
	for _, r := range roles {
		labels = append(labels, roleLabel(r))
	}
	return strings.Join(labels, "、")
}

// notifyRoleChanged 在角色变更成功后通知被变更用户与操作人本人。
func (svc *UserService) notifyRoleChanged(ctx context.Context, actor Actor, user *ent.User, oldRoles, newRoles []string) {
	oldLabel := joinRoleLabels(oldRoles)
	newLabel := joinRoleLabels(newRoles)
	at := time.Now()

	// 被变更账号：邮件主题/正文以「您的账号」口吻，面向本人。
	toUser := mail.RoleChangedMessage(
		user.Email,
		user.Nickname,
		oldLabel,
		newLabel,
		actor.Email,
		at,
	)
	if user.Email != "" && user.Email != actor.Email {
		_ = svc.s.sendSystemMail(ctx, mailPurposeNotifyRoleChanged, user.Email, user.ID, toUser)
	}

	// 操作人：留档确认邮件，正文一致。
	if actor.Email != "" {
		toOperator := mail.RoleChangedMessage(
			actor.Email,
			user.Nickname,
			oldLabel,
			newLabel,
			actor.Email,
			at,
		)
		_ = svc.s.sendSystemMail(ctx, mailPurposeNotifyRoleChanged, actor.Email, user.ID, toOperator)
	}
}

// ResetPassword 由管理员重置指定用户密码，并强制该用户重新登录。
func (svc *UserService) ResetPassword(ctx context.Context, id string, newPassword string, actor Actor) error {
	if err := auth.ValidatePasswordStrength(newPassword, svc.s.Cfg.Auth.PasswordMinLength); err != nil {
		return err
	}
	user, err := svc.mutableUser(ctx, id)
	if err != nil {
		return err
	}

	hash, err := auth.HashPassword(newPassword, svc.s.Cfg.Auth.BcryptCost)
	if err != nil {
		return err
	}
	if _, err := svc.s.Store.Ent.User.UpdateOneID(id).
		SetPasswordHash(hash).
		SetPasswordChangedAt(time.Now()).
		SetFailedLoginCount(0).
		SetNillableLockedUntil(nil).
		// 管理员重置后密码由用户自己掌握，不再需要「必须修改密码」提示。
		SetMustChangePassword(false).
		Save(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}

	_ = svc.s.Sessions.BumpPasswordVersion(ctx, id)
	_ = svc.s.Sessions.RevokeAllExcept(ctx, id, "")

	svc.s.Audit.Record(ctx, actor, "user.reset_password", "user", id,
		map[string]any{"email": user.Email})
	return nil
}

// Delete 删除用户（同时清理其角色与审计关联）。
func (svc *UserService) Delete(ctx context.Context, id string, actor Actor) error {
	if id == actor.UserID {
		return apperr.ErrInvalidParams.WithMessage("不能删除自己的账号")
	}
	if _, err := svc.mutableUser(ctx, id); err != nil {
		return err
	}

	roles, err := svc.s.Authz.RolesForUser(id)
	if err != nil {
		return err
	}
	if containsString(roles, casbin.RoleAdmin) {
		if err := svc.assertNotLastAdmin(ctx, id); err != nil {
			return err
		}
	}

	// 先解除审计记录与验证码的外键引用，保留历史记录本身。
	if _, err := svc.s.Store.Ent.AuditLog.Update().
		Where(auditlog.ActorIDEQ(id)).
		SetNillableActorID(nil).
		Save(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	if _, err := svc.s.Store.Ent.VerificationCode.Update().
		Where(verificationcode.UserIDEQ(id)).
		SetNillableUserID(nil).
		Save(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}

	if err := svc.s.Store.Ent.User.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return apperr.ErrUserNotFound
		}
		return apperr.ErrInternal.WithCause(err)
	}

	_ = svc.s.Authz.SetRoles(id, nil)
	_ = svc.s.Sessions.BumpPasswordVersion(ctx, id)
	_ = svc.s.Sessions.RevokeAllExcept(ctx, id, "")

	svc.s.Audit.Record(ctx, actor, "user.delete", "user", id, map[string]any{"roles": roles})
	return nil
}

// Roles 返回角色列表（元数据 + 权限策略 + 用户数）。
func (svc *UserService) Roles(ctx context.Context) ([]RoleView, error) {
	entities, err := svc.s.Store.Ent.Role.Query().Order(ent.Desc("level")).All(ctx)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	policies, err := svc.s.Authz.Policies()
	if err != nil {
		return nil, err
	}

	views := make([]RoleView, 0, len(entities))
	for _, r := range entities {
		view := NewRoleView(r)
		for _, p := range policies {
			if len(p) >= 3 && p[0] == r.Name {
				view.Permissions = append(view.Permissions, p)
			}
		}
		if ids, err := svc.s.Authz.UsersForRole(r.Name); err == nil {
			view.UserCount = len(ids)
		}
		views = append(views, view)
	}
	return views, nil
}

// assertNotLastAdmin 确保系统中至少保留一名管理员。
//
// 复用 selectAvailableAdmins：判定规则必须与「初始管理员自举」完全一致，
// 否则会出现「保护逻辑认为没有管理员可用、自举逻辑认为有」这类互相矛盾的结论。
func (svc *UserService) assertNotLastAdmin(ctx context.Context, excludeUserID string) error {
	ids, err := svc.s.Authz.UsersForRole(casbin.RoleAdmin)
	if err != nil {
		return err
	}

	candidates := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != excludeUserID {
			candidates = append(candidates, id)
		}
	}

	remaining, err := selectAvailableAdmins(candidates, func(id string) (bool, error) {
		// 双重确认「确实直接拥有 admin 角色」。
		//
		// 不能只信 UsersForRole 的返回：Casbin 的角色图是传递的
		// （admin ⊃ reviewer ⊃ user），一旦其查询语义包含继承关系带出来的用户，
		// 一个普通审核员就会被误判成「还有别的管理员」，从而放开最后管理员的保护。
		roles, err := svc.s.Authz.RolesForUser(id)
		if err != nil {
			return false, err
		}
		if !containsString(roles, casbin.RoleAdmin) {
			return false, nil
		}

		// 只统计仍然存在且未被禁用的管理员。
		exists, err := svc.s.Store.Ent.User.Query().
			Where(entuser.IDEQ(id), entuser.StatusNEQ(entuser.StatusDisabled)).
			Exist(ctx)
		if err != nil {
			return false, apperr.ErrInternal.WithCause(err)
		}
		return exists, nil
	})
	if err != nil {
		return err
	}
	if len(remaining) == 0 {
		return apperr.ErrInvalidParams.WithMessage("系统必须保留至少一名可用管理员")
	}
	return nil
}

// mutableUser 读取用户并拒绝操作系统机器身份。
//
// 系统身份由 bootstrap 幂等维护，后台任何写操作都必须拒绝：
// 禁用会让抓取/邮件失去审计归属，删除会让历史 actor_id 变成悬空引用，
// 改角色会破坏「system 不继承任何角色」的边界，重置密码则毫无意义（本就无密码）。
func (svc *UserService) mutableUser(ctx context.Context, id string) (*ent.User, error) {
	user, err := svc.s.Store.Ent.User.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, apperr.ErrUserNotFound
	}
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	if user.Kind == entuser.KindSystem {
		return nil, apperr.ErrInvalidParams.WithMessage("系统身份不可修改")
	}
	return user, nil
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
