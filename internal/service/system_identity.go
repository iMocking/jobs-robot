package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jsong/jobs-robot/internal/casbin"
	"github.com/jsong/jobs-robot/internal/ent"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// 系统身份的稳定标识（写入 users.system_key）。
const (
	SystemKeyCrawler = "crawler"
	SystemKeyMail    = "mail"
)

// 系统身份的显示名称（写入 users.nickname）。
const (
	SystemNameCrawler = "Jobs AI Robot"
	SystemNameMail    = "Jobs Mail Robot"
)

// systemRegisterSource 是系统身份专用注册来源，便于事后从用户列表区分。
const systemRegisterSource = "system"

// SystemIdentity 描述一个系统内置机器身份。
type SystemIdentity struct {
	Key  string // users.system_key
	Name string // users.nickname
}

// SystemIdentities 是系统内置的全部机器身份，按展示顺序返回。
var SystemIdentities = []SystemIdentity{
	{Key: SystemKeyCrawler, Name: SystemNameCrawler},
	{Key: SystemKeyMail, Name: SystemNameMail},
}

// EnsureSystemIdentities 幂等创建/修复系统身份，并返回 key → 用户 ULID 的映射。
//
// 这些账号与真人同表，但没有邮箱与密码、kind=system、角色固定为 system：
//   - 邮箱为空：永远无法通过邮箱登录 / 收验证码 / 找回密码；
//   - 密码哈希为空：即使绕过邮箱检查也没有可校验的凭据；
//   - 角色 system 不继承任何角色、没有任何路径策略，因此不能访问后台接口。
//
// 「修复」是刻意的：任何字段被误改（例如被手工改成 disabled 或被塞了邮箱），
// 下一次启动都会恢复成机器身份应有的样子，而不是留下一个半坏的身份。
func (svc *Services) EnsureSystemIdentities(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(SystemIdentities))
	for _, identity := range SystemIdentities {
		user, created, err := svc.ensureSystemIdentity(ctx, identity)
		if err != nil {
			return nil, err
		}
		if err := svc.Authz.SetRoles(user.ID, []string{casbin.RoleSystem}); err != nil {
			return nil, fmt.Errorf("设置系统身份 %s 的角色失败: %w", identity.Key, err)
		}

		// detail 只记录可公开的定位信息，绝不含密码（本来也没有密码）。
		svc.Audit.Record(ctx, SystemActor(), "system.identity_ensure", "user", user.ID,
			map[string]any{
				"key":     identity.Key,
				"user_id": user.ID,
				"created": created,
			})
		out[identity.Key] = user.ID
	}
	return out, nil
}

// ensureSystemIdentity 创建或修复单个系统身份，返回用户与「是否本次创建」。
func (svc *Services) ensureSystemIdentity(ctx context.Context, identity SystemIdentity) (*ent.User, bool, error) {
	existing, err := svc.Store.Ent.User.Query().
		Where(entuser.SystemKeyEQ(identity.Key)).
		Only(ctx)
	switch {
	case err == nil:
		user, err := svc.repairSystemIdentity(ctx, existing, identity)
		if err != nil {
			return nil, false, err
		}
		return user, false, nil
	case ent.IsNotFound(err):
		// 继续往下走创建分支。
	default:
		return nil, false, apperr.ErrInternal.WithCause(err)
	}

	now := time.Now()
	user, err := svc.Store.Ent.User.Create().
		SetKind(entuser.KindSystem).
		SetSystemKey(identity.Key).
		SetNickname(identity.Name).
		SetEmail("").
		SetPasswordHash("").
		SetStatus(entuser.StatusActive).
		SetEmailVerified(true).
		SetEmailVerifiedAt(now).
		SetRegisterSource(systemRegisterSource).
		Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("创建系统身份 %s 失败: %w", identity.Key, err)
	}
	return user, true, nil
}

// repairSystemIdentity 把既有行恢复到系统身份应有的状态。
func (svc *Services) repairSystemIdentity(ctx context.Context, current *ent.User, identity SystemIdentity) (*ent.User, error) {
	updater := svc.Store.Ent.User.UpdateOneID(current.ID).
		SetKind(entuser.KindSystem).
		SetSystemKey(identity.Key).
		SetNickname(identity.Name).
		SetEmail("").
		SetPasswordHash("").
		SetStatus(entuser.StatusActive).
		SetEmailVerified(true).
		SetRegisterSource(systemRegisterSource).
		// 系统身份永远不会登录，也不应该被提示改密。
		SetMustChangePassword(false)
	if current.EmailVerifiedAt == nil {
		updater = updater.SetEmailVerifiedAt(time.Now())
	}

	user, err := updater.Save(ctx)
	if ent.IsNotFound(err) {
		return nil, apperr.ErrUserNotFound
	}
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	return user, nil
}
