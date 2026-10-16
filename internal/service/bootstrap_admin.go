package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/casbin"
	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/ent"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
)

// 本文件实现「系统首次启动自动创建一个可登录的管理员账号」。
//
// 要解决的问题是一个死循环：配置 SMTP 必须登录后台，登录必须先有账号，
// 而账号此前只能靠运维手工执行 cmd/seed 创建——全新部署因此卡住。
// 自举把这个循环打断：启动时若发现系统中没有任何「可用管理员」，
// 就创建一个并把凭据打印在启动日志（WARN 级）与凭据文件中。
//
// 注意自举**默认关闭**（config.Bootstrap.Enabled 默认 false）：有界面的部署
// 应当走安装向导（internal/service/setup.go + 前端 /setup），由运维自己设定密码
// 并现场验证 SMTP，而不是从启动日志里捡一个随机口令。本文件保留给无界面场景
// （纯容器、CI、只跑 API 的镜像），需显式置 auth.bootstrap.enabled=true。
//
// 三条不可逾越的边界：
//   - 幂等：只要存在一名可用管理员就什么都不做，绝不修改既有账号；
//   - 密码不落日志：来自配置（环境变量/配置文件）的密码只打印占位提示；
//   - 审计不含密码：审计 detail 只写邮箱、来源与是否强制改密。
const (
	// bootstrapRegisterSource 是自举创建账号的注册来源标记，便于事后区分。
	bootstrapRegisterSource = "bootstrap"
	// bootstrapPasswordLength 是自动生成密码的长度。
	bootstrapPasswordLength = 16
	// bootstrapAdminRole 是自举必须授予的角色。
	bootstrapAdminRole = casbin.RoleAdmin
)

// ErrAdminEmailTaken 表示目标邮箱已被既有账号占用。
//
// 用哨兵错误而不是匹配错误文案：调用方需要据此返回 409（可操作提示），
// 而插入失败时 ent 返回的是驱动层的约束错误（不同版本文案不同），
// 只有这里能可靠地翻译成「邮箱被占用」这个业务含义。
var ErrAdminEmailTaken = errors.New("管理员邮箱已被既有账号占用")

// BootstrapResult 描述自举结果。
type BootstrapResult struct {
	Created bool // 是否本次创建/提升了管理员
	Skipped bool // 是否因为已存在可用管理员而跳过
	Email   string
	// Password 仅在「本次新建账号且密码为随机生成」时返回明文，供启动日志打印一次。
	//
	// 两种情况会为空或不可用：
	//   - 密码来自 cfg.Password（运维自设，不回显）；
	//   - 走的是「提升既有账号」路径（密码未被修改，这里生成的随机值并未生效）。
	Password string
	UserID   string
	Reason   string // 跳过原因，便于排查
}

// EnsureBootstrapAdmin 保证系统中至少存在一名可登录的管理员。
//
// 判定顺序：
//  1. 遍历 Casbin 中拥有 admin 角色的用户，并逐个确认「用户行存在且未被禁用」；
//  2. 只要存在 1 个可用管理员就跳过（Skipped=true），不触碰任何既有账号；
//  3. 否则用配置的邮箱创建管理员；若该邮箱已被占用（曾降级/管理员被清空），
//     走「提升既有账号」路径，避免撞唯一约束。
//
// 密码来源：cfg.Password 非空时直接使用（返回值不回显，避免运维自设密码进日志），
// 否则用 auth.GeneratePassword 生成随机强密码并在返回值中带出一次。
//
// 注意：这是启动路径上的「尽力而为 + 必须可读」操作。数据库层面的临时故障
// （例如查询超时）不会阻止服务启动，只会告警并跳过；只有真正的创建失败
// （唯一约束、写入错误）才返回错误。
func (svc *Services) EnsureBootstrapAdmin(ctx context.Context, cfg config.Bootstrap) (*BootstrapResult, error) {
	if !cfg.Enabled {
		return &BootstrapResult{Skipped: true, Reason: "auth.bootstrap.enabled=false，已停用初始管理员自举"}, nil
	}

	availableIDs, err := svc.availableAdminIDs(ctx)
	if err != nil {
		// 数据库抖动不应把 API 拦在启动之外：跳过并告警，下次启动会再试。
		slog.Warn("查询可用管理员失败，本次跳过初始管理员自举", "error", err)
		return &BootstrapResult{Skipped: true, Reason: "查询可用管理员失败：" + err.Error()}, nil
	}

	decision := DecideBootstrapAdmin(availableIDs)
	if !decision.NeedBootstrap {
		adminID := ""
		if len(availableIDs) > 0 {
			adminID = availableIDs[0]
		}
		slog.Info("已存在可用管理员，跳过初始账号自举",
			"admin_id", adminID,
			"available_admins", len(availableIDs),
			"reason", decision.Reason,
		)
		return &BootstrapResult{
			Skipped: true,
			UserID:  adminID,
			Reason:  decision.Reason,
		}, nil
	}

	email := normalizeEmail(cfg.Email)
	if email == "" {
		return nil, errors.New("初始管理员邮箱为空：请配置 auth.bootstrap.email 或设置 JOBSROBOT_AUTH_BOOTSTRAP_EMAIL")
	}

	password := cfg.Password
	generated := false
	if strings.TrimSpace(password) == "" {
		password, err = auth.GeneratePassword(bootstrapPasswordLength)
		if err != nil {
			return nil, fmt.Errorf("生成初始管理员随机密码失败: %w", err)
		}
		generated = true
	}
	if err := auth.ValidatePasswordStrength(password, svc.Cfg.Auth.PasswordMinLength); err != nil {
		return nil, fmt.Errorf("初始管理员密码不符合要求: %w", err)
	}
	created, err := EnsureAdminAccount(ctx, svc.Store.Ent, svc.Authz, EnsureAdminOptions{
		Email:          email,
		Password:       password,
		Nickname:       cfg.Nickname,
		BcryptCost:     svc.Cfg.Auth.BcryptCost,
		RegisterSource: bootstrapRegisterSource,
	})
	if err != nil {
		return nil, err
	}

	user, err := svc.Store.Ent.User.Query().Where(entuser.EmailEQ(email)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("初始管理员已写入，但读取账号失败: %w", err)
	}

	// Reason 让运维一眼看出这次到底是「新建」还是「提升既有账号」——
	// 后者的密码没有被重置，可能与运维预期不同。
	reason := "已创建初始管理员账号"
	if !created {
		reason = "该邮箱已存在（此前不是管理员），已提升为管理员并标记需强制改密；其原有密码未被修改"
	} else if !generated {
		reason = "已按配置的密码创建初始管理员账号（密码来自 auth.bootstrap.password，未写入日志）"
	}

	// 凭据文件写失败只告警：拿不到文件也必须能登录（密码已打印在日志里）。
	//
	// 只在「本次新建了账号」时写：提升既有账号的路径不会修改密码，
	// 把一个并未生效的随机密码写进凭据文件只会让运维照着登录却失败。
	credentialFile := strings.TrimSpace(cfg.CredentialFile)
	if created && credentialFile != "" {
		if err := writeBootstrapCredentialFile(credentialFile, email, password, time.Now()); err != nil {
			slog.Warn("写入初始管理员凭据文件失败，请直接从启动日志中获取密码",
				"credential_file", credentialFile, "error", err)
		}
	}
	if !created && credentialFile != "" {
		slog.Warn("该邮箱已是既有账号，未修改其密码，因此不写凭据文件；请使用该账号原有密码登录",
			"email", email, "credential_file", credentialFile)
	}

	// 审计只记录可公开事实，绝不写入口令。
	detail := map[string]any{
		"email":                email,
		"register_source":      user.RegisterSource,
		"must_change_password": true,
		"password_generated":   generated && created,
		"promoted_existing":    !created,
	}
	if credentialFile != "" {
		detail["credential_file"] = credentialFile
	}
	svc.Audit.Record(ctx, SystemActor(), "user.bootstrap_admin", "user", user.ID, detail)

	// 用 WARN 级打印：默认日志级别是 info，而这条是运维必须看到的一次性凭据输出。
	// 密码只在「本次新建账号且随机生成」时打印——来自配置的密码、以及
	// 提升既有账号时那个并未生效的随机密码，都不写进日志。
	message := "初始管理员账号已创建，请登录后台配置邮件（SMTP）服务，并立即修改密码"
	loggedPassword := password
	switch {
	case !created:
		message = "既有账号已被提升为管理员，请登录后台配置邮件（SMTP）服务，并立即修改密码"
		loggedPassword = "(既有账号，密码未修改)"
	case !generated:
		loggedPassword = "(来自配置，未打印)"
	}
	slog.Warn(message,
		"email", email,
		"password", loggedPassword,
		"user_id", user.ID,
		"credential_file", credentialFile,
		"must_change_password", true,
		"created", created,
		"note", reason,
	)

	// 只有「本次新建账号且随机生成密码」才把明文带出去：
	// 来源为配置的密码不回显，提升既有账号时生成的随机密码并未生效。
	resultPassword := ""
	if created && generated {
		resultPassword = password
	}

	return &BootstrapResult{
		Created:  true,
		Email:    email,
		Password: resultPassword,
		UserID:   user.ID,
		Reason:   reason,
	}, nil
}

// availableAdminIDs 返回「确实直接拥有 admin 角色，且账号存在未被禁用」的候选管理员。
//
// 这是 *Services 上的方法而不是某个子服务的方法：「最后一名管理员保护」
// （UserService.assertNotLastAdmin）与「初始管理员自举」都要用它，
// 判定规则必须只有一份，否则两个逻辑会得出互相矛盾的结论。
//
// 双重确认的原因：UsersForRole 走的是 Casbin 的角色图，而角色图是传递的
// （admin ⊃ reviewer ⊃ user）。一旦查询语义带出继承关系上的用户，仅凭
// UsersForRole 就会把审核员误判成管理员，从而让自举错误地跳过，
// 系统便重新落入「没有可登录管理员」的死循环。
func (svc *Services) availableAdminIDs(ctx context.Context) ([]string, error) {
	return svc.availableAdminIDsWith(ctx, svc.Store.Ent)
}

// availableAdminIDsWith 是 availableAdminIDs 的可注入客户端版本。
//
// 为什么要能换客户端：安装向导在咨询锁内必须用**同一个事务**复查管理员数量，
// 否则读到的是事务开始前的快照，锁就失去了意义（见 setup.go 的 createSetupAdmin）。
// 判定规则本身仍然只有这一份实现。
func (svc *Services) availableAdminIDsWith(ctx context.Context, client *ent.Client) ([]string, error) {
	ids, err := svc.Authz.UsersForRole(bootstrapAdminRole)
	if err != nil {
		return nil, err
	}
	return selectAvailableAdmins(ids, func(id string) (bool, error) {
		roles, err := svc.Authz.RolesForUser(id)
		if err != nil {
			return false, err
		}
		if !containsString(roles, bootstrapAdminRole) {
			return false, nil
		}
		return client.User.Query().
			Where(entuser.IDEQ(id), entuser.StatusNEQ(entuser.StatusDisabled)).
			Exist(ctx)
	})
}

// BootstrapDecision 是「是否需要自举初始管理员」的判定结果。
type BootstrapDecision struct {
	NeedBootstrap bool
	Reason        string
}

// DecideBootstrapAdmin 判断是否需要创建初始管理员。
//
// 这是从 EnsureBootstrapAdmin 中抽出的纯函数：只依赖「候选管理员 ID 列表」，
// 不接触数据库或 Casbin，因此可以完整单测「跳过 / 创建」两条分支。
func DecideBootstrapAdmin(availableAdminIDs []string) BootstrapDecision {
	if len(availableAdminIDs) > 0 {
		return BootstrapDecision{
			NeedBootstrap: false,
			Reason:        fmt.Sprintf("已存在 %d 名可用管理员，无需自举", len(availableAdminIDs)),
		}
	}
	return BootstrapDecision{
		NeedBootstrap: true,
		Reason:        "系统中没有任何可用管理员（不存在、已被禁用或仅有继承自其他角色的权限）",
	}
}

// selectAvailableAdmins 用 isAvailable 过滤候选管理员，保持原有顺序。
//
// 回调返回 error 时立即向上传递而不是当作「不可用」：把数据库故障误判成
// 「没有管理员」会导致系统反复自举、创建出多余的管理员账号。
func selectAvailableAdmins(candidateIDs []string, isAvailable func(id string) (bool, error)) ([]string, error) {
	out := make([]string, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		ok, err := isAvailable(id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, id)
		}
	}
	return out, nil
}

// EnsureAdminOptions 是 EnsureAdminAccount 的入参。
type EnsureAdminOptions struct {
	// Email 必须是已归一化（小写去空白）的邮箱。
	Email string
	// Password 是明文密码，仅在需要新建账号时使用。
	Password string
	Nickname string
	// BcryptCost 为 0 时使用 bcrypt 默认成本。
	BcryptCost     int
	RegisterSource string
	// MustChangePassword 控制新建账号是否被标记为「必须修改密码」。
	//
	// 为什么用指针：自举路径（启动自动建号）必须为 true —— 密码是系统随机生成的，
	// 运维没见过它；而安装向导路径必须是 false —— 密码就是运维自己在页面上设的，
	// 再逼他登录后立刻改一遍纯属打扰。
	// nil 表示沿用默认的 true（保持自举既有行为不变）。
	MustChangePassword *bool
	// Roles 是新建账号要授予的角色；为空表示授予默认的 admin。
	//
	// 安装向导用显式角色列表（SetRoles）而不是逐个 AssignRole：新建账号的
	// 权限集合应当是「确定的一份」，只加不减会留下历史遗留角色。
	Roles []string
}

// rolesOrDefault 返回本次创建要授予的角色集合。
func (o EnsureAdminOptions) rolesOrDefault() []string {
	if len(o.Roles) == 0 {
		return []string{bootstrapAdminRole}
	}
	return o.Roles
}

// mustChangePassword 解析 MustChangePassword，nil 时按默认 true 处理。
func (o EnsureAdminOptions) mustChangePassword() bool {
	if o.MustChangePassword == nil {
		return true
	}
	return *o.MustChangePassword
}

// EnsureAdminAccount 幂等地保证指定邮箱的账号存在、状态可用并拥有 admin 角色。
//
// 返回的 created 表示是否**新建**了账号（false 表示走的是「提升既有账号」路径，
// 此时密码未被修改）。
//
// 两种情况：
//   - 邮箱不存在：创建新账号（active + 已验证，是否强制改密由配置决定）；
//   - 邮箱已存在：只补 admin 角色与强制改密标记，**不改密码**。
//     这是为了避开 users.email 的唯一约束——管理员被清空或降级的场景下，
//     直接插入会撞约束并让启动失败。
func EnsureAdminAccount(ctx context.Context, client *ent.Client, authz *casbin.Authorizer, opts EnsureAdminOptions) (bool, error) {
	existing, err := client.User.Query().Where(entuser.EmailEQ(opts.Email)).Only(ctx)
	switch {
	case err == nil:
		if err := promoteBootstrapAdmin(ctx, client, authz, existing); err != nil {
			return false, err
		}
		return false, nil
	case !ent.IsNotFound(err):
		return false, fmt.Errorf("查询邮箱 %s 是否已存在失败: %w", opts.Email, err)
	}

	if _, err := CreateAdminAccount(ctx, client, authz, opts); err != nil {
		return false, err
	}
	return true, nil
}

// CreateAdminAccount **只新建**管理员账号，邮箱已被占用时直接返回错误。
//
// 与 EnsureAdminAccount 的差别就是这条：不需要「提升既有账号」的兜底。
// 安装向导用它，因为「邮箱已存在」在全新安装场景下是异常状态，
// 静默走上提升路径会让运维以为自己设的密码生效了，实际上并没有。
func CreateAdminAccount(ctx context.Context, client *ent.Client, authz *casbin.Authorizer, opts EnsureAdminOptions) (*ent.User, error) {
	hash, err := auth.HashPassword(opts.Password, opts.BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("生成初始管理员密码哈希失败: %w", err)
	}
	now := time.Now()
	registerSource := strings.TrimSpace(opts.RegisterSource)
	if registerSource == "" {
		registerSource = bootstrapRegisterSource
	}

	user, err := client.User.Create().
		SetEmail(opts.Email).
		SetPasswordHash(hash).
		SetNickname(truncateRunes(strings.TrimSpace(opts.Nickname), 64)).
		SetStatus(entuser.StatusActive).
		SetEmailVerified(true).
		SetEmailVerifiedAt(now).
		SetPasswordChangedAt(now).
		SetRegisterSource(registerSource).
		SetMustChangePassword(opts.mustChangePassword()).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			// 用 %w 包住哨兵错误：调用方 errors.Is 判定后返回 409 或继续走提升路径。
			return nil, fmt.Errorf("%w：%s（请改用其它邮箱或清理冲突数据）: %w",
				ErrAdminEmailTaken, opts.Email, err)
		}
		return nil, fmt.Errorf("创建初始管理员失败: %w", err)
	}

	roles := opts.rolesOrDefault()
	if err := authz.SetRoles(user.ID, roles); err != nil {
		return nil, fmt.Errorf("授予初始管理员角色失败（账号 %s 已创建，请手动补授 %v 角色）: %w", user.ID, roles, err)
	}
	return user, nil
}

// promoteBootstrapAdmin 把已存在的账号提升为管理员。
//
// 刻意不动密码：该账号可能正是运维正在使用的账号，重置密码会把
// 「自动自举」变成一次意外的凭据变更。仅补角色与强制改密标记。
func promoteBootstrapAdmin(ctx context.Context, client *ent.Client, authz *casbin.Authorizer, user *ent.User) error {
	if err := authz.AssignRole(user.ID, bootstrapAdminRole); err != nil {
		return fmt.Errorf("为既有账号 %s 补授 admin 角色失败: %w", user.ID, err)
	}
	_, err := client.User.UpdateOneID(user.ID).
		SetMustChangePassword(true).
		// 被禁用的账号即使有 admin 角色也无法登录，这里一并放开。
		SetStatus(entuser.StatusActive).
		SetEmailVerified(true).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("标记既有账号 %s 需强制改密失败: %w", user.ID, err)
	}
	return nil
}

// writeBootstrapCredentialFile 把初始凭据写入文件，权限 0600。
//
// 为什么是 0600：这是可直接登录后台的明文口令，同机其它用户不应读到。
// 目录不存在时自动创建（默认路径 var/ 在多数部署里并不存在）。
func writeBootstrapCredentialFile(path, email, password string, at time.Time) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		// 目录权限同样收紧：0700 保证只有属主能进入与列举。
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("创建凭据文件目录 %s 失败: %w", dir, err)
		}
	}

	content := fmt.Sprintf(`JobsRobot 初始管理员凭据（自动生成）
生成时间：%s
登录邮箱：%s
初始密码：%s

请按以下顺序处理，处理完成后删除本文件：
  1. 使用上述邮箱与密码登录后台；
  2. 先在「系统设置 - 邮件设置」中配置 SMTP 并发送测试邮件，确保注册与找回密码可用；
  3. 再在「个人中心」修改密码（初始密码被标记为必须修改）；
  4. 删除本文件：%s
`, at.Format("2006-01-02 15:04:05"), email, password, path)

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("写入凭据文件 %s 失败: %w", path, err)
	}
	return nil
}
