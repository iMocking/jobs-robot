package service

import (
	"context"
	"strings"
	"time"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/casbin"
	"github.com/jsong/jobs-robot/internal/ent"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
	"github.com/jsong/jobs-robot/internal/ent/verificationcode"
	"github.com/jsong/jobs-robot/internal/mail"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// AuthService 负责注册、登录、令牌与会话、邮箱验证码、密码找回。
type AuthService struct{ s *Services }

// SendCodeResult 是验证码发送结果。
type SendCodeResult struct {
	Email     string `json:"email"`
	Purpose   string `json:"purpose"`
	ExpiresIn int    `json:"expires_in"`
	// DebugCode 仅在 auth.expose_code_in_response=true 时返回，供本地联调与自动化测试使用。
	DebugCode string `json:"debug_code,omitempty"`
	// Message 用于「找回密码」这类不暴露账号是否存在的场景，始终返回统一话术。
	Message string `json:"message"`
}

// RegisterInput 是注册入参。
type RegisterInput struct {
	Email     string
	Password  string
	Nickname  string
	Code      string
	IP        string
	UserAgent string
}

// LoginInput 是登录入参。
type LoginInput struct {
	Email     string
	Password  string
	IP        string
	UserAgent string
}

// UpdateProfileInput 是资料更新入参。
type UpdateProfileInput struct {
	Nickname  *string
	AvatarURL *string
	Phone     *string
}

// TokenResult 是签发令牌的结果。
type TokenResult struct {
	User  UserView        `json:"user"`
	Token *auth.TokenPair `json:"token"`
}

// purposeLabels 是验证码用途的中文文案。
var purposeLabels = map[auth.CodePurpose]string{
	auth.CodePurposeRegister:      "注册",
	auth.CodePurposeResetPassword: "找回密码",
	auth.CodePurposeChangeEmail:   "更换邮箱",
	auth.CodePurposeLogin:         "登录",
}

func labelOf(p auth.CodePurpose) string {
	if v, ok := purposeLabels[p]; ok {
		return v
	}
	return "身份验证"
}

// SendCode 发送邮箱验证码。
//
// 隐私考量：找回密码场景不区分「邮箱是否已注册」，一律返回相同话术，
// 避免被用来枚举平台用户；验证码只在账号确实存在时才真正发出。
func (svc *AuthService) SendCode(ctx context.Context, email string, purpose auth.CodePurpose, ip, ua string) (*SendCodeResult, error) {
	if !purpose.Valid() {
		return nil, apperr.ErrInvalidParams.WithMessage("验证码用途不合法")
	}
	email = normalizeEmail(email)
	if !looksLikeEmail(email) {
		return nil, apperr.ErrInvalidParams.WithMessage("邮箱格式不正确")
	}

	user, err := svc.s.Store.Ent.User.Query().
		Where(entuser.EmailEQ(email)).
		Only(ctx)
	switch {
	case err == nil:
	case ent.IsNotFound(err):
		user = nil
	default:
		return nil, apperr.ErrInternal.WithCause(err)
	}

	switch purpose {
	case auth.CodePurposeRegister:
		if user != nil {
			return nil, apperr.ErrEmailExists
		}
	case auth.CodePurposeResetPassword, auth.CodePurposeLogin:
		// 账号不存在时静默返回成功，保持与存在时完全一致的外部表现。
		if user == nil {
			return &SendCodeResult{
				Email:   email,
				Purpose: string(purpose),
				Message: "如果该邮箱已注册，验证码已发送，请查收邮件",
			}, nil
		}
	}

	issued, err := svc.s.Codes.Issue(ctx, email, purpose)
	if err != nil {
		return nil, err
	}

	label := labelOf(purpose)
	if err := svc.s.Mailer.Send(ctx, mail.VerificationCodeMessage(email, issued.Code, label, svc.s.Codes.TTL())); err != nil {
		// 发信失败必须回滚验证码，否则用户拿不到码却要等冷却。
		_ = svc.s.Codes.Invalidate(ctx, email, purpose)

		// 邮件通道故障属于「依赖不可用」而不是「服务端 bug」：
		// 返回 503 + 可读提示，前端才能给出「邮件服务暂不可用，请稍后重试」这类正常文案；
		// 若统一返回 500「服务器内部错误」，用户与前端都无从判断该怎么办。
		// 底层原因（例如 DNS 解析失败、认证被拒）仍会完整写入服务端日志。
		return nil, apperr.ErrUnavailable.
			WithMessage("验证码邮件发送失败，请稍后重试或联系管理员检查邮件配置").
			WithCause(err)
	}

	svc.recordCodeIssue(ctx, email, purpose, issued, user, ip, ua)

	result := &SendCodeResult{
		Email:     email,
		Purpose:   string(purpose),
		ExpiresIn: int(svc.s.Codes.TTL().Seconds()),
		Message:   "验证码已发送，请查收邮件",
	}
	if svc.s.Cfg.Auth.ExposeCodeInResponse {
		result.DebugCode = issued.Code
	}
	return result, nil
}

// Register 注册新用户。
//
// 流程：校验密码强度 → 查重 → 校验邮箱验证码 → 落库 → 授予默认角色 → 签发令牌。
func (svc *AuthService) Register(ctx context.Context, in RegisterInput) (*TokenResult, error) {
	email := normalizeEmail(in.Email)
	if !looksLikeEmail(email) {
		return nil, apperr.ErrInvalidParams.WithMessage("邮箱格式不正确")
	}
	if err := auth.ValidatePasswordStrength(in.Password, svc.s.Cfg.Auth.PasswordMinLength); err != nil {
		return nil, err
	}

	exists, err := svc.s.Store.Ent.User.Query().Where(entuser.EmailEQ(email)).Exist(ctx)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	if exists {
		return nil, apperr.ErrEmailExists
	}

	verified := false
	if svc.s.Cfg.Auth.RegisterNeedVerify {
		if strings.TrimSpace(in.Code) == "" {
			return nil, apperr.ErrCodeInvalid.WithMessage("请填写邮箱验证码")
		}
		if err := svc.s.Codes.Verify(ctx, email, auth.CodePurposeRegister, in.Code); err != nil {
			return nil, err
		}
		verified = true
	}

	hash, err := auth.HashPassword(in.Password, svc.s.Cfg.Auth.BcryptCost)
	if err != nil {
		return nil, err
	}

	nickname := strings.TrimSpace(in.Nickname)
	if nickname == "" {
		nickname = defaultNickname(email)
	}
	now := time.Now()

	builder := svc.s.Store.Ent.User.Create().
		SetEmail(email).
		SetPasswordHash(hash).
		SetNickname(truncateRunes(nickname, 64)).
		SetStatus(entuser.StatusActive).
		SetEmailVerified(verified).
		SetRegisterIP(truncateRunes(in.IP, 64)).
		SetRegisterSource("email").
		SetPasswordChangedAt(now)
	if verified {
		builder = builder.SetEmailVerifiedAt(now)
	}

	user, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, apperr.ErrEmailExists
		}
		return nil, apperr.ErrInternal.WithCause(err)
	}

	// 授予默认角色：普通用户。
	roles := []string{casbin.RoleUser}
	if err := svc.s.Authz.SetRoles(user.ID, roles); err != nil {
		return nil, err
	}

	token, err := svc.issueToken(ctx, user, roles, in.IP, in.UserAgent)
	if err != nil {
		return nil, err
	}
	svc.markCodeUsed(ctx, email, auth.CodePurposeRegister)

	svc.s.Audit.Record(ctx, Actor{
		UserID: user.ID, Email: user.Email, IP: in.IP, UserAgent: in.UserAgent,
	}, "auth.register", "user", user.ID, map[string]any{"email": email, "email_verified": verified})

	return &TokenResult{User: NewUserView(user, roles), Token: token}, nil
}

// Login 校验凭据并签发令牌。
//
// 安全要点：
//   - 账号不存在时也执行一次 bcrypt 校验，避免通过响应时间枚举已注册邮箱；
//   - 连续失败达到阈值后临时锁定账号，抵御在线撞库。
func (svc *AuthService) Login(ctx context.Context, in LoginInput) (*TokenResult, error) {
	email := normalizeEmail(in.Email)
	if !looksLikeEmail(email) {
		return nil, apperr.ErrInvalidCredentials
	}

	user, err := svc.s.Store.Ent.User.Query().Where(entuser.EmailEQ(email)).Only(ctx)
	if ent.IsNotFound(err) {
		auth.VerifyDummy(in.Password)
		return nil, apperr.ErrInvalidCredentials
	}
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	// 锁定中直接拒绝，且不透露剩余时间以外的信息。
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		return nil, apperr.ErrAccountLocked.WithDetails(map[string]any{
			"locked_until": user.LockedUntil,
		})
	}
	if user.Status == entuser.StatusDisabled {
		return nil, apperr.ErrAccountDisabled
	}
	if svc.s.Cfg.Auth.RegisterNeedVerify && !user.EmailVerified {
		return nil, apperr.ErrAccountUnverified
	}

	if !auth.VerifyPassword(user.PasswordHash, in.Password) {
		svc.registerLoginFailure(ctx, user)
		return nil, apperr.ErrInvalidCredentials
	}

	// 登录成功：清零失败计数并记录登录信息。
	now := time.Now()
	updater := svc.s.Store.Ent.User.UpdateOneID(user.ID).
		SetFailedLoginCount(0).
		SetLastLoginAt(now).
		SetLastLoginIP(truncateRunes(in.IP, 64)).
		SetNillableLockedUntil(nil)
	if user.Status == entuser.StatusLocked {
		updater = updater.SetStatus(entuser.StatusActive)
	}
	if _, err := updater.Save(ctx); err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	roles, err := svc.ensureRoles(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	token, err := svc.issueToken(ctx, user, roles, in.IP, in.UserAgent)
	if err != nil {
		return nil, err
	}

	svc.s.Audit.Record(ctx, Actor{
		UserID: user.ID, Email: user.Email, IP: in.IP, UserAgent: in.UserAgent,
	}, "auth.login", "user", user.ID, nil)

	user.LastLoginAt = &now
	user.LastLoginIP = truncateRunes(in.IP, 64)
	return &TokenResult{User: NewUserView(user, roles), Token: token}, nil
}

// registerLoginFailure 累加失败次数，达到阈值则临时锁定。
func (svc *AuthService) registerLoginFailure(ctx context.Context, user *ent.User) {
	failed := user.FailedLoginCount + 1

	updater := svc.s.Store.Ent.User.UpdateOneID(user.ID).SetFailedLoginCount(failed)
	if svc.s.Cfg.Auth.LoginFailMax > 0 && failed >= svc.s.Cfg.Auth.LoginFailMax {
		lockedUntil := time.Now().Add(svc.s.Cfg.Auth.LoginLockDuration)
		updater = updater.SetLockedUntil(lockedUntil).SetStatus(entuser.StatusLocked)
	}
	if _, err := updater.Save(ctx); err != nil {
		// 失败计数写入失败不影响主流程，仅记录。
		svc.s.Audit.Record(ctx, SystemActor(), "auth.login_failure_count_error", "user", user.ID,
			map[string]any{"error": err.Error()})
	}
}

// Refresh 刷新令牌。
func (svc *AuthService) Refresh(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	pair, err := svc.s.Sessions.Rotate(ctx, refreshToken, func(ctx context.Context, userID string) ([]string, error) {
		return svc.ensureRoles(ctx, userID)
	})
	if err != nil {
		return nil, err
	}
	return pair, nil
}

// Logout 注销当前会话。
func (svc *AuthService) Logout(ctx context.Context, refreshToken string, userID string, email string) error {
	if err := svc.s.Sessions.Revoke(ctx, refreshToken); err != nil {
		return err
	}
	svc.s.Audit.Record(ctx, Actor{UserID: userID, Email: email}, "auth.logout", "user", userID, nil)
	return nil
}

// Me 返回当前登录用户信息。
func (svc *AuthService) Me(ctx context.Context, userID string) (UserView, error) {
	user, err := svc.s.Store.Ent.User.Get(ctx, userID)
	if ent.IsNotFound(err) {
		return UserView{}, apperr.ErrUserNotFound
	}
	if err != nil {
		return UserView{}, apperr.ErrInternal.WithCause(err)
	}
	roles, err := svc.s.Authz.RolesForUser(userID)
	if err != nil {
		return UserView{}, err
	}
	return NewUserView(user, roles), nil
}

// UserEntity 返回用户实体，供其他服务复用。
func (svc *AuthService) UserEntity(ctx context.Context, userID string) (*ent.User, error) {
	user, err := svc.s.Store.Ent.User.Get(ctx, userID)
	if ent.IsNotFound(err) {
		return nil, apperr.ErrUserNotFound
	}
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	return user, nil
}

// UpdateProfile 更新个人资料。
func (svc *AuthService) UpdateProfile(ctx context.Context, userID string, in UpdateProfileInput) (UserView, error) {
	updater := svc.s.Store.Ent.User.UpdateOneID(userID)
	if in.Nickname != nil {
		nickname := strings.TrimSpace(*in.Nickname)
		if nickname == "" {
			return UserView{}, apperr.ErrInvalidParams.WithMessage("昵称不能为空")
		}
		updater = updater.SetNickname(truncateRunes(nickname, 64))
	}
	if in.AvatarURL != nil {
		updater = updater.SetAvatarURL(truncateRunes(strings.TrimSpace(*in.AvatarURL), 512))
	}
	if in.Phone != nil {
		phone := strings.TrimSpace(*in.Phone)
		if phone == "" {
			updater = updater.SetNillablePhone(nil)
		} else {
			updater = updater.SetPhone(truncateRunes(phone, 32))
		}
	}

	user, err := updater.Save(ctx)
	if ent.IsNotFound(err) {
		return UserView{}, apperr.ErrUserNotFound
	}
	if err != nil {
		return UserView{}, apperr.ErrInternal.WithCause(err)
	}

	roles, err := svc.s.Authz.RolesForUser(userID)
	if err != nil {
		return UserView{}, err
	}
	return NewUserView(user, roles), nil
}

// ChangePassword 修改密码（需验证原密码），并使其他设备的登录状态失效。
//
// 返回值是「当前设备换发的新令牌」：密码版本递增会让所有既有访问令牌失效，
// 因此当前设备必须拿到一对新令牌，否则用户会被自己踢下线。keepSessionID 为空时返回 nil。
func (svc *AuthService) ChangePassword(ctx context.Context, userID string, oldPassword, newPassword, keepSessionID string) (*auth.TokenPair, error) {
	user, err := svc.UserEntity(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !auth.VerifyPassword(user.PasswordHash, oldPassword) {
		return nil, apperr.ErrOldPasswordWrong
	}
	if oldPassword == newPassword {
		return nil, apperr.ErrPasswordSame
	}
	if err := auth.ValidatePasswordStrength(newPassword, svc.s.Cfg.Auth.PasswordMinLength); err != nil {
		return nil, err
	}

	if err := svc.setPassword(ctx, user, newPassword); err != nil {
		return nil, err
	}
	// 改密后：递增密码版本（旧访问令牌立刻失效）+ 吊销其他会话。
	if err := svc.s.Sessions.BumpPasswordVersion(ctx, userID); err != nil {
		return nil, err
	}
	if err := svc.s.Sessions.RevokeAllExcept(ctx, userID, keepSessionID); err != nil {
		return nil, err
	}

	var pair *auth.TokenPair
	if keepSessionID != "" {
		roles, err := svc.ensureRoles(ctx, userID)
		if err != nil {
			return nil, err
		}
		pair, err = svc.s.Sessions.ReissueForSession(ctx, keepSessionID, roles)
		if err != nil {
			// 当前会话恰好已过期时不阻断改密结果，客户端重新登录即可。
			svc.s.Audit.Record(ctx, SystemActor(), "auth.reissue_after_change_failed", "user", userID,
				map[string]any{"error": err.Error()})
			pair = nil
		}
	}

	if err := svc.s.Mailer.Send(ctx, mail.PasswordResetDoneMessage(user.Email, time.Now())); err != nil {
		// 通知邮件失败不影响改密结果。
		svc.s.Audit.Record(ctx, SystemActor(), "auth.notify_mail_failed", "user", userID,
			map[string]any{"error": err.Error()})
	}

	svc.s.Audit.Record(ctx, Actor{UserID: userID, Email: user.Email}, "auth.change_password", "user", userID, nil)
	return pair, nil
}

// ForgotPassword 发送找回密码验证码。
func (svc *AuthService) ForgotPassword(ctx context.Context, email string, ip, ua string) (*SendCodeResult, error) {
	return svc.SendCode(ctx, email, auth.CodePurposeResetPassword, ip, ua)
}

// ResetPassword 通过邮箱验证码重置密码，并强制所有设备重新登录。
func (svc *AuthService) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	email = normalizeEmail(email)
	if !looksLikeEmail(email) {
		return apperr.ErrInvalidParams.WithMessage("邮箱格式不正确")
	}
	if err := auth.ValidatePasswordStrength(newPassword, svc.s.Cfg.Auth.PasswordMinLength); err != nil {
		return err
	}

	user, err := svc.s.Store.Ent.User.Query().Where(entuser.EmailEQ(email)).Only(ctx)
	if ent.IsNotFound(err) {
		// 不暴露账号是否存在，但也不创建任何状态。
		return apperr.ErrCodeInvalid
	}
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}

	if err := svc.s.Codes.Verify(ctx, email, auth.CodePurposeResetPassword, code); err != nil {
		return err
	}
	if err := svc.setPassword(ctx, user, newPassword); err != nil {
		return err
	}
	if err := svc.s.Sessions.BumpPasswordVersion(ctx, user.ID); err != nil {
		return err
	}
	if err := svc.s.Sessions.RevokeAllExcept(ctx, user.ID, ""); err != nil {
		return err
	}
	svc.markCodeUsed(ctx, email, auth.CodePurposeResetPassword)

	if err := svc.s.Mailer.Send(ctx, mail.PasswordResetDoneMessage(user.Email, time.Now())); err != nil {
		svc.s.Audit.Record(ctx, SystemActor(), "auth.notify_mail_failed", "user", user.ID,
			map[string]any{"error": err.Error()})
	}

	svc.s.Audit.Record(ctx, Actor{Email: email}, "auth.reset_password", "user", user.ID, nil)
	return nil
}

// Sessions 列出当前活跃会话。
func (svc *AuthService) Sessions(ctx context.Context, userID string) ([]SessionView, error) {
	list, err := svc.s.Sessions.ListSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	views := make([]SessionView, 0, len(list))
	for _, item := range list {
		views = append(views, SessionView{
			ID:        item.ID,
			IP:        item.IP,
			UserAgent: item.UserAgent,
			CreatedAt: item.CreatedAt,
			ExpiresAt: item.ExpiresAt,
		})
	}
	return views, nil
}

// RevokeSession 注销指定会话（下线某台设备）。
func (svc *AuthService) RevokeSession(ctx context.Context, userID string, sessionID string) error {
	sessions, err := svc.s.Sessions.ListSessions(ctx, userID)
	if err != nil {
		return err
	}
	for _, item := range sessions {
		if item.ID == sessionID {
			if err := svc.s.Sessions.RevokeSession(ctx, sessionID); err != nil {
				return err
			}
			svc.s.Audit.Record(ctx, Actor{UserID: userID}, "auth.revoke_session", "session", sessionID, nil)
			return nil
		}
	}
	return apperr.ErrNotFound.WithMessage("会话不存在或已过期")
}

// —— 内部辅助 ——

// issueToken 读取密码版本后签发令牌对。
func (svc *AuthService) issueToken(ctx context.Context, user *ent.User, roles []string, ip, ua string) (*auth.TokenPair, error) {
	pwdVersion, err := svc.s.Sessions.PasswordVersion(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return svc.s.Sessions.Issue(ctx, user.ID, user.Email, roles, pwdVersion, auth.SessionMeta{
		IP:        ip,
		UserAgent: ua,
	})
}

// ensureRoles 保证用户至少拥有普通用户角色，并返回其角色列表。
//
// 这是一道自愈保护：若角色数据被误删（例如直接操作了 casbin_rule 表），
// 用户登录后仍能拿到基本权限，而不是变成一个「无任何权限」的黑洞账号。
func (svc *AuthService) ensureRoles(ctx context.Context, userID string) ([]string, error) {
	roles, err := svc.s.Authz.RolesForUser(userID)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		if err := svc.s.Authz.AssignRole(userID, casbin.RoleUser); err != nil {
			return nil, err
		}
		roles = []string{casbin.RoleUser}
	}
	return roles, nil
}

// setPassword 写入新密码哈希并更新 password_changed_at，同时清除「必须修改密码」标记。
//
// 为什么一并清除：该标记唯一的作用就是提醒用户改掉初始密码，
// 用户已经改完了就必须归零，否则前端会永远弹着「请修改初始密码」的提示条。
func (svc *AuthService) setPassword(ctx context.Context, user *ent.User, plain string) error {
	hash, err := auth.HashPassword(plain, svc.s.Cfg.Auth.BcryptCost)
	if err != nil {
		return err
	}
	if _, err := svc.s.Store.Ent.User.UpdateOneID(user.ID).
		SetPasswordHash(hash).
		SetPasswordChangedAt(time.Now()).
		SetMustChangePassword(false).
		Save(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// recordCodeIssue 落一条验证码审计记录（不保存明文，只保存 HMAC 摘要）。
func (svc *AuthService) recordCodeIssue(ctx context.Context, email string, purpose auth.CodePurpose, issued *auth.IssuedCode, user *ent.User, ip, ua string) {
	builder := svc.s.Store.Ent.VerificationCode.Create().
		SetEmail(email).
		SetPurpose(verificationcode.Purpose(purpose)).
		SetCodeHash(issued.Hash).
		SetExpiresAt(issued.ExpiresAt).
		SetMaxAttempts(svc.s.Cfg.Auth.CodeMaxAttempts).
		SetIP(truncateRunes(ip, 64)).
		SetUserAgent(truncateRunes(ua, 512)).
		SetDelivery(svc.s.Mailer.Name())
	if user != nil {
		builder = builder.SetUserID(user.ID)
	}
	if _, err := builder.Save(ctx); err != nil {
		// 审计写入失败不阻塞业务。
		svc.s.Audit.Record(ctx, SystemActor(), "auth.code_audit_failed", "verification_code", "",
			map[string]any{"error": err.Error(), "email": email})
	}
}

// markCodeUsed 标记最近一条同类验证码为已使用。
func (svc *AuthService) markCodeUsed(ctx context.Context, email string, purpose auth.CodePurpose) {
	latest, err := svc.s.Store.Ent.VerificationCode.Query().
		Where(
			verificationcode.EmailEQ(email),
			verificationcode.PurposeEQ(verificationcode.Purpose(purpose)),
			verificationcode.UsedAtIsNil(),
		).
		Order(ent.Desc(verificationcode.FieldID)).
		First(ctx)
	if err != nil {
		return
	}
	_ = svc.s.Store.Ent.VerificationCode.UpdateOneID(latest.ID).
		SetUsedAt(time.Now()).
		Exec(ctx)
}

// looksLikeEmail 做最小必要校验：含 @、有域名点、无空格。
func looksLikeEmail(email string) bool {
	if email == "" || strings.ContainsAny(email, " \t\r\n") {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return false
	}
	domain := email[at+1:]
	return strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

// defaultNickname 用邮箱前缀作为默认昵称。
func defaultNickname(email string) string {
	if at := strings.Index(email, "@"); at > 0 {
		return truncateRunes(email[:at], 64)
	}
	return "用户"
}
