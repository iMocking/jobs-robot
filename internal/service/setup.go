package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/ent"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
	"github.com/jsong/jobs-robot/internal/mail"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// 本文件实现「首次安装向导」的服务端逻辑。
//
// 与 internal/service/bootstrap_admin.go 的关系：
//   - bootstrap_admin.go 是**服务端**在启动时自动建号（凭据随机生成、写日志与文件），
//     默认关闭，保留给无界面/容器部署；
//   - 本文件是**运维**在浏览器里走的一次性安装流程（自己设密码、现场验证 SMTP），
//     是默认路径，对应前端 /setup 与 POST /api/v1/system/setup。
//
// 三条硬性边界：
//   - 只在「系统尚无可用管理员」时可用，一旦存在管理员即永久失效（409）；
//   - 口令绝不进入日志、审计 detail 或响应体；
//   - 不写任何凭据文件——密码是运维自己设的，落盘只会多一处明文泄漏面。

const (
	// setupRegisterSource 标记由安装向导创建的账号，便于事后与注册/自举区分。
	setupRegisterSource = "setup"
	// setupAdvisoryLockKey 是「判定 + 创建管理员」这段临界区的 PostgreSQL 咨询锁 key。
	//
	// 为什么需要它：三重门控里的「系统尚无可用管理员」是一次**读**，
	// 两个并发的安装请求可以同时读到「没有管理员」，然后各建一个账号——
	// 结果就是刚装好的系统里有两个管理员，其中一个还是请求方以为没建成的。
	// 普通行锁挡不住这种「先查后插」，因为被查的行当时并不存在（无行可锁）。
	//
	// pg_advisory_xact_lock 是会话级/事务级的命名互斥量，与业务表无关，
	// 恰好表达「同一个 key 同一时刻只有一个事务能走完这段逻辑」，
	// 并且随事务提交/回滚自动释放，不需要任何过期与清理逻辑。
	//
	// 取值：任意常量即可，只要全库唯一。这里用 "jobsetup" 的 Unicode 码点拼出
	// 一个固定的 64 位数值，避免与将来其它咨询锁撞 key。
	setupAdvisoryLockKey int64 = 0x6a6f627365747570 // "jobsetup"
)

// SetupService 负责首次安装向导：建管理员、落邮件配置、发测试邮件。
type SetupService struct{ s *Services }

// SetupAdminInput 是安装向导提交的入参（已由 handler 做过必填校验）。
type SetupAdminInput struct {
	AdminEmail    string
	AdminPassword string
	Nickname      string
	// Mail 为 nil 表示运维选择「先跳过邮件服务」：只建管理员，不落邮件配置。
	// 此时 registration_open 仍为 false，前端应提示「稍后到后台补配」。
	Mail *config.Mail
	// RequestActor 只带 HTTP 侧的请求元信息（IP / User-Agent / 方法 / 路径），
	// 用于让审计能回答「这次初始化是从哪个 IP 发起的」。
	// 其中的 UserID / Email 会被创建出来的管理员身份覆盖（见 SetupAdmin）。
	RequestActor Actor
}

// SetupResult 是安装向导的成功返回体。
//
// 只回显**可公开**的信息：管理员身份、角色与当前邮件通道状态。
// 绝不回显口令、口令哈希或 SMTP 密码。
type SetupResult struct {
	Admin UserView         `json:"admin"`
	Mail  SystemMailStatus `json:"mail"`
	// RegistrationOpen 与 /system/status 同义：装了可用 SMTP 才会变成 true。
	RegistrationOpen bool `json:"registration_open"`
	// MailConfigured 表示本次请求是否真的写入了邮件配置（前端据此区分
	// 「已配置」与「跳过了，稍后补配」两种结局）。
	MailConfigured bool `json:"mail_configured"`
}

// DecideSetupAvailable 判断安装向导当前是否可用。
//
// 这是从 SetupAdmin 中抽出的纯函数：只依赖「向导是否开启」与「可用管理员数量」，
// 不接触数据库，因此可以完整单测两条拒绝分支（未启用 / 已初始化）与放行分支。
//
// 注意第二个参数是 len(availableAdminIDs) 而不是 len(userIDs)：判定「已初始化」
// 必须与 Bootstrap 用同一套「可用管理员」口径（直接拥有 admin 角色且未被禁用），
// 否则会出现「向导说没初始化、status 说已初始化」的自相矛盾。
func DecideSetupAvailable(enabled bool, availableAdminCount int) SetupDecision {
	switch {
	case !enabled:
		return SetupDecision{
			Available: false,
			Reason:    "安装向导已在配置中关闭（auth.setup.enabled=false）",
		}
	case availableAdminCount > 0:
		return SetupDecision{
			Available: false,
			Reason: fmt.Sprintf(
				"系统已完成初始化（已存在 %d 名可用管理员），安装向导永久失效；如需新增管理员请登录后台操作",
				availableAdminCount),
		}
	default:
		return SetupDecision{
			Available: true,
			Reason:    "系统尚未初始化，安装向导可用",
		}
	}
}

// SetupDecision 是安装向导可用性的判定结果。
type SetupDecision struct {
	Available bool
	// Reason 是可读原因：可用时用于日志，不可用时原样作为 409 的提示文案。
	Reason string
}

// ErrSetupClosed 把「向导不可用」的原因包装成 409 业务错误。
//
// 用 409（冲突）而不是 403：这不是权限问题，而是**系统状态**已经不允许
// 再走安装流程了；409 也便于前端把「已初始化」与真正的鉴权失败区分开。
func ErrSetupClosed(reason string) *apperr.Error {
	return apperr.ErrConflict.WithMessage("无法使用安装向导：" + reason)
}

// assertSetupAvailable 执行三重门控的 (a) 与 (b)，返回可读错误。
//
// 三重门控：
//
//	(a) cfg.Auth.Setup.Enabled 为真 —— 配置层开关；
//	(b) 系统当前不存在可用管理员 —— 复用 availableAdminIDs（与自举同一套判定）；
//	(c) 不满足则返回 409 + 可读信息。判定与创建之间由咨询锁串行化。
func (svc *SetupService) assertSetupAvailable(ctx context.Context) (*SetupDecision, error) {
	// 先看配置：关闭时连数据库都不查，避免无意义的查询。
	if !svc.s.Cfg.Auth.Setup.Enabled {
		decision := DecideSetupAvailable(false, 0)
		return nil, ErrSetupClosed(decision.Reason)
	}

	admins, err := svc.s.availableAdminIDs(ctx)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	decision := DecideSetupAvailable(true, len(admins))
	if !decision.Available {
		return nil, ErrSetupClosed(decision.Reason)
	}
	return &decision, nil
}

// availableAdminCountForTx 确认「是否存在可用管理员」，用于咨询锁内的权威复查。
//
// 为什么要传 txClient：这个复查必须在**同一个事务**里读到「锁刚拿到时」的状态。
// 若用 svc.s.Store.Ent（另一条连接），读到的可能是本事务开始之前的快照，
// 那样锁就白拿了——第一个请求建号提交后，第二个请求仍可能读到旧快照而放行。
//
// 这里刻意复用 svc.s.availableAdminIDsWith 而不是自己写 SQL：
// 「可用管理员」的判定规则（直接拥有 admin 角色 + 用户行存在 + 未被禁用）
// 必须全项目只有一份，否则各处的结论会互相矛盾。
func (svc *SetupService) availableAdminCountForTx(ctx context.Context, txClient *ent.Client) (int, error) {
	admins, err := svc.s.availableAdminIDsWith(ctx, txClient)
	if err != nil {
		return 0, err
	}
	return len(admins), nil
}

// SetupAdmin 执行安装向导：创建管理员（可选）落邮件配置，并热生效。
//
// 执行顺序（每一步失败都在注释里说明其后果）：
//  1. 校验邮箱、密码强度、邮件配置 —— **全部在任何写入之前完成**，
//     避免「管理员建好了但邮件配置非法」这种半成品状态；
//  2. 开门控检查（配置开关 + 是否已有管理员）；
//  3. 事务 + 咨询锁：锁内复查管理员数量 → 创建管理员 → 提交；
//  4. 授予 admin 角色（仍在锁的临界区语义内：锁在函数返回时随事务结束释放，
//     因此并发请求必须等到角色写完之后才能进来，不会看到「有用户但没角色」的中间态）；
//  5. 落邮件配置并热替换投递器（失败不回滚账号：账号已经可用，
//     运维可以登录后台补配，比把一个刚创建好的管理员删掉更合理）；
//  6. 审计两条（user.setup_admin / mail.update），detail 一律不含密码。
func (svc *SetupService) SetupAdmin(ctx context.Context, in SetupAdminInput) (*SetupResult, error) {
	if _, err := svc.assertSetupAvailable(ctx); err != nil {
		return nil, err
	}

	email := normalizeEmail(in.AdminEmail)
	if !looksLikeEmail(email) {
		return nil, apperr.ErrInvalidParams.WithMessage("管理员邮箱格式不正确")
	}
	// 密码强度必须复用 auth.ValidatePasswordStrength：安装向导与注册、
	// 找回密码、后台重置密码共用同一套规则，否则会出现「向导能设的密码
	// 在别处过不了（或反之）」的割裂。
	if err := auth.ValidatePasswordStrength(in.AdminPassword, svc.s.Cfg.Auth.PasswordMinLength); err != nil {
		return nil, err
	}
	if in.Mail != nil {
		if err := in.Mail.Validate(); err != nil {
			return nil, apperr.ErrSettingInvalid.WithMessage(err.Error())
		}
	}

	user, err := svc.createSetupAdmin(ctx, email, in.AdminPassword, in.Nickname)
	if err != nil {
		return nil, err
	}

	actor := SetupActor(user.ID, user.Email)
	// 把请求元信息（IP/UA/路径）带进审计：出问题时能回答「谁从哪发起的」。
	if in.RequestActor.IP != "" {
		actor.IP = in.RequestActor.IP
	}
	actor.UserAgent = in.RequestActor.UserAgent
	actor.Method = in.RequestActor.Method
	actor.Path = in.RequestActor.Path

	// 审计先记「建号」：即使后面的邮件配置写入失败，管理员账号也是客观存在的，
	// 漏记会让审计日志与实际状态不一致。detail 只有可公开事实。
	svc.s.Audit.Record(ctx, actor, "user.setup_admin", "user", user.ID, map[string]any{
		"email":                user.Email,
		"register_source":      user.RegisterSource,
		"must_change_password": user.MustChangePassword,
		"email_verified":       user.EmailVerified,
		"roles":                []string{bootstrapAdminRole},
		"mail_configured":      in.Mail != nil,
	})

	mailConfigured := false
	if in.Mail != nil {
		// Save 内部负责校验、加密落库、热替换投递器与写 mail.update 审计。
		if _, err := svc.s.Mail.Save(ctx, MailConfigInput{
			Driver:      in.Mail.Driver,
			FromName:    in.Mail.FromName,
			FromAddress: in.Mail.FromAddress,
			Host:        in.Mail.SMTP.Host,
			Port:        in.Mail.SMTP.Port,
			Username:    in.Mail.SMTP.Username,
			// 向导提交的是完整配置，密码就是运维刚填的那个；
			// 这里的空值表示「该 SMTP 不需要认证」，而不是「沿用旧密码」
			// （系统刚装好，本来也没有旧密码可言）。
			Password: in.Mail.SMTP.Password,
			TLS:      &in.Mail.SMTP.TLS,
		}, actor); err != nil {
			return nil, apperr.ErrInternal.WithCause(
				fmt.Errorf("管理员账号已创建（%s），但邮件配置保存失败，请登录后台「系统设置 - 邮件设置」重新配置: %w",
					user.Email, err))
		}
		mailConfigured = true
	}

	status, err := svc.s.Meta.SystemStatus(ctx)
	if err != nil {
		return nil, err
	}

	return &SetupResult{
		Admin: newSetupAdminView(user),
		Mail:  status.Mail,
		// 用 SystemStatus 的结论而不是自己重算：registration_open 的语义
		// （要求邮箱验证却没有可用 SMTP 时注册实际走不通）只应有一处实现。
		RegistrationOpen: status.RegistrationOpen,
		MailConfigured:   mailConfigured,
	}, nil
}

// createSetupAdmin 在咨询锁保护下创建管理员账号。
//
// 这里用 database/sql 的事务（而不是 svc.s.Store.Ent.Tx）是为了拿到 ExecContext：
// 咨询锁必须与「复查 + 插入」落在**同一条连接**上才有效，
// 而 ent 的 *ent.Tx 没有暴露执行原生 SQL 的方法。
// 拿到 *sql.Tx 后用 ent.Driver(entsql.NewDriver(...)) 包一个 *ent.Client，
// 就能在同一个事务里继续使用 ent 的查询构造器（见 CreateAdminAccount）。
//
// **锁的边界必须覆盖角色授予**（这一点很容易写错，值得展开）：
// 门控判定的是「不存在**可用**管理员」，而可用 = 直接拥有 admin 角色 + 账号未被禁用。
// 用户行与 casbin_rule 是两张表、两条连接，做不到一个事务写完。若把角色授予放在
// tx.Commit() 之后，就会出现一个窗口：用户行已提交、admin 角色还没写。
// 此时并发的第二个安装请求拿到锁后复查，会得出「没有可用管理员」并**再建一个账号**——
// 正是咨询锁要防的那件事。
//
// 因此把「建号 + 授予角色」整体放进临界区，最后才 Commit：
//   - 角色授予失败 → 事务回滚，绝不会留下「有用户行但没有 admin 角色」的半成品；
//   - Commit 成功 → 咨询锁在整个这段逻辑跑完之后才随事务释放，
//     后续请求拿到锁时必然看到一名「有 admin 角色的可用管理员」，从而被门控拒绝。
func (svc *SetupService) createSetupAdmin(ctx context.Context, email, password, nickname string) (*ent.User, error) {
	tx, err := svc.s.Store.SQL().BeginTx(ctx, nil)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	// 事务异常路径统一回滚；正常路径提交后 Rollback 返回 ErrTxDone，是空操作，
	// 因此可以无条件 defer，不必在每条分支手写。
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 咨询锁：为什么用它、为什么用 xact 版本，见 setupAdvisoryLockKey 的注释。
	// 用 $1 绑定常量而不是拼接 SQL。
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock($1)`, setupAdvisoryLockKey); err != nil {
		return nil, apperr.ErrInternal.WithCause(fmt.Errorf("获取安装向导互斥锁失败: %w", err))
	}

	// 在事务内构造 ent 客户端：之后每个查询都走 tx 这条连接。
	txClient := ent.NewClient(ent.Driver(entsql.NewDriver(dialect.Postgres, entsql.Conn{ExecQuerier: tx})))

	count, err := svc.availableAdminCountForTx(ctx, txClient)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	if count > 0 {
		// 锁内复查失败 = 另一个并发请求刚刚建好了管理员。
		return nil, ErrSetupClosed(DecideSetupAvailable(true, count).Reason)
	}

	// 邮箱占用单独预检一次，好给出「换一个邮箱」这种可操作的提示；
	// 唯一约束仍然存在，插入时的约束错误作为兜底（见下面的 ent.IsConstraintError）。
	switch exists, err := txClient.User.Query().Where(entuser.EmailEQ(email)).Exist(ctx); {
	case err != nil:
		return nil, apperr.ErrInternal.WithCause(err)
	case exists:
		return nil, apperr.ErrConflict.WithMessage(
			"该邮箱已被既有账号占用，无法作为初始管理员：" + email +
				"（请换一个邮箱，或先清理该账号）")
	}

	// 复用自举的建号逻辑（同一套字段语义），只改三处：
	// register_source 标记为 setup、不强制改密（密码是运维自己设的）、
	// 且只走「新建」路径（邮箱已存在时明确报冲突而不是静默提升既有账号）。
	//
	// CreateAdminAccount 内部会调用 Authz.SetRoles（写 casbin_rule，另一条连接）——
	// 它不随本事务回滚，但顺序是「先授权、后提交用户行」：
	// 提交失败最坏留下一条指向不存在用户的孤儿角色行，会被 availableAdminIDs
	// 里的「用户行是否存在」判定过滤掉，比「有管理员却没有 admin 角色」安全得多。
	notRequired := false
	user, err := CreateAdminAccount(ctx, txClient, svc.s.Authz, EnsureAdminOptions{
		Email:              email,
		Password:           password,
		Nickname:           firstNonEmpty(strings.TrimSpace(nickname), defaultNickname(email)),
		BcryptCost:         svc.s.Cfg.Auth.BcryptCost,
		RegisterSource:     setupRegisterSource,
		MustChangePassword: &notRequired,
		Roles:              []string{bootstrapAdminRole},
	})
	if err != nil {
		if errors.Is(err, ErrAdminEmailTaken) || ent.IsConstraintError(err) {
			return nil, apperr.ErrConflict.WithMessage(
				"该邮箱已被既有账号占用，无法作为初始管理员：" + email +
					"（请换一个邮箱，或先清理该账号）")
		}
		return nil, apperr.ErrInternal.WithCause(err)
	}

	// Commit 即释放咨询锁；此刻角色已经授予完毕，临界区语义完整。
	if err := tx.Commit(); err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	committed = true
	return user, nil
}

// SetupActor 构造安装向导的审计操作者。
//
// 与 SystemActor 的区别：这里能明确到「人」，只是此刻还没有登录态
// （账号刚建出来），因此用刚创建的用户身份做审计主语，
// 而不是打成 system，便于事后区分「自动自举」与「运维装出来的账号」。
func SetupActor(userID, email string) Actor {
	return Actor{UserID: userID, Email: email}
}

// newSetupAdminView 组装安装向导返回的管理员视图。
//
// 不复用 User.NewView/NewUserView 的数据库查询路径：此处刚创建用户，
// 角色是已知的（必然只有 admin），再查一次数据库只会多一次往返与一处失败点。
func newSetupAdminView(u *ent.User) UserView {
	view := NewUserView(u, []string{bootstrapAdminRole})
	// 兜底：NewUserView 直接读实体字段，若调用方传入的实体是「提升既有账号」
	// 路径返回的旧快照，这里的字段可能不是最新值。安装向导只走新建路径，
	// 因此以入参为准即可；显式再写一遍是为了让差异在 review 时可见。
	view.Email = u.Email
	view.MustChangePassword = u.MustChangePassword
	return view
}

// —— 测试邮件（不持久化） ——

// TestMailInput 是「用向导表单里的参数试发一封邮件」的入参。
type TestMailInput struct {
	// Mail 是运维在表单里填的完整配置，**只用于本次发信，不写库**。
	Mail config.Mail
	// To 收件人；留空时回落到 cfg.Auth.Setup.TestMailTo。
	To string
}

// SendSetupTestMail 用请求体里的邮件配置试发一封测试邮件。
//
// 为什么**不持久化**，也不复用 svc.s.Mailer：
//   - 这一步的目的恰恰是「在正式提交之前验证 host/port/加密方式/授权码是否正确」。
//     如果先落库再试发，一个填错的配置就已经写进数据库并热替换了投递器，
//     系统会立刻处于「注册验证码发不出去」的状态——而这正是向导要避免的；
//   - 用一个临时的 mail.New(cfg) 发信，与最终保存后真正使用的实现完全同一条代码路径
//     （同一个构造函数、同一个协议实现），因此「测试通过」对「保存后可用」是有意义的预测；
//   - 失败返回 503 + 可读原因（连接失败 / STARTTLS 握手失败 / 认证失败 / RCPT 被拒），
//     让运维知道该改哪一项，而不是只看到一句「失败」。其中**认证失败**用专用码
//     10009 并带上排查步骤与服务器原文（与后台邮件设置页共用 describeMailFailure）。
func (svc *SetupService) SendSetupTestMail(ctx context.Context, in TestMailInput) error {
	if _, err := svc.assertSetupAvailable(ctx); err != nil {
		return err
	}

	to := strings.TrimSpace(in.To)
	if to == "" {
		to = strings.TrimSpace(svc.s.Cfg.Auth.Setup.TestMailTo)
	}
	if !looksLikeEmail(to) {
		return apperr.ErrInvalidParams.WithMessage("收件邮箱格式不正确")
	}

	cfg := in.Mail
	if strings.TrimSpace(cfg.Driver) == "" {
		cfg.Driver = "console"
	}
	// 密码首尾空白几乎总是复制授权码时带进来的产物，而 SMTP 认证会因此失败，
	// 且失败原因（535 认证失败）完全指不到「多了个空格」这个真实原因上。
	//
	// 这里**自动去除**而不是拒绝（与后台设置页 MailService.Save 完全一致）：
	// 拒绝会让「向导能填的参数，后台改一次就存不进去」这种不一致再次出现。
	// 告警只记录发生了去除这一事实与主机/账号，不含口令本身。
	cfg.SMTP.Password = trimMailSecret(cfg.SMTP.Password, cfg.SMTP.Host, cfg.SMTP.Username)
	// 先 trim 再校验：全空白的口令会被下面的「用户名与密码需同时填写」拦下，
	// 而不是被当成一个有效口令送去认证。
	if err := cfg.Validate(); err != nil {
		return apperr.ErrSettingInvalid.WithMessage(err.Error())
	}

	mailer, err := mail.New(cfg)
	if err != nil {
		return apperr.ErrSettingInvalid.WithMessage("邮件配置无法生效：" + err.Error())
	}

	if err := mailer.Send(ctx, mail.TestMessage(to, mailer.Name(), time.Now())); err != nil {
		// 与后台邮件设置页共用同一套文案（认证失败 → 10009 + 排查步骤 + 服务器原文）。
		return describeMailFailure("测试邮件发送失败", err)
	}
	return nil
}
