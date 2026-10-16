package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/setting"
	"github.com/jsong/jobs-robot/internal/mail"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/secret"
	"github.com/jsong/jobs-robot/internal/store"
)

// SettingKeyMailSMTP 是邮件配置在 settings 表中的键。
const SettingKeyMailSMTP = "mail.smtp"

// MailPasswordMask 是密码字段的占位符。
//
// 后台接口永远不明文返回已保存的 SMTP 密码；前端拿到的是这个占位符，
// 原样回传即表示「不修改密码」。这样既能让管理员看到「已配置」，
// 又不会把口令送到浏览器或写进前端日志。
const MailPasswordMask = "********"

// MailConfigView 是邮件配置视图。
type MailConfigView struct {
	Driver      string `json:"driver"`
	FromName    string `json:"from_name"`
	FromAddress string `json:"from_address"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	TLS         bool   `json:"tls"`

	HasPassword  bool   `json:"has_password"`
	PasswordMask string `json:"password_mask"`

	// Source 说明当前生效配置来自哪里：config（配置文件）或 database（后台设置）。
	Source string `json:"source"`
	// ActiveDriver 是当前实际装载的投递器名称（console/smtp/none）。
	ActiveDriver string `json:"active_driver"`
	// Ready 表示投递器是否可用（driver=console 或 smtp 配置完整时为 true）。
	Ready bool `json:"ready"`

	UpdatedBy string     `json:"updated_by,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// MailConfigInput 是保存邮件配置的入参。
type MailConfigInput struct {
	Driver      string
	FromName    string
	FromAddress string
	Host        string
	Port        int
	Username    string
	// Password 为空或等于占位符时表示沿用已保存的密码。
	Password string
	TLS      *bool
	// ClearPassword 显式清空已保存的密码。
	ClearPassword bool
}

// MailService 负责邮件配置的读取、在线修改、热生效与测试发信。
type MailService struct{ s *Services }

// mailSecretKey 派生用于加密 SMTP 密码的子密钥。
//
// 与 JWT 签名密钥做用途隔离：即便某一处密钥材料泄漏，也不能直接用于解另一处数据。
func mailSecretKey(cfg *config.Config) []byte {
	return secret.DeriveKey(cfg.JWT.Secret, "mail-smtp")
}

// ResolveMailConfig 计算「最终生效」的邮件配置：以配置文件为基线，被数据库设置覆盖。
//
// 之所以做成包级函数：bootstrap 需要在构造 Services 之前就拿到配置来创建投递器。
// 返回的 fromDatabase 表示当前配置是否来自后台设置（用于界面提示）。
func ResolveMailConfig(ctx context.Context, st *store.Store, cfg *config.Config) (config.Mail, bool, error) {
	row, err := st.Ent.Setting.Query().Where(setting.KeyEQ(SettingKeyMailSMTP)).Only(ctx)
	if ent.IsNotFound(err) {
		return cfg.Mail, false, nil
	}
	if err != nil {
		return cfg.Mail, false, fmt.Errorf("读取邮件配置失败: %w", err)
	}

	merged, err := decodeMailSetting(cfg, row.Value)
	if err != nil {
		// 配置损坏时不让服务起不来：回落到配置文件并交由调用方告警。
		return cfg.Mail, false, err
	}
	return merged, true, nil
}

// decodeMailSetting 把数据库中的 JSON 还原为配置对象，并把加密密码解出来。
func decodeMailSetting(cfg *config.Config, value map[string]any) (config.Mail, error) {
	out := cfg.Mail

	if v := mapString(value, "driver"); v != "" {
		out.Driver = v
	}
	if v := mapString(value, "from_name"); v != "" {
		out.FromName = v
	}
	if v := mapString(value, "from_address"); v != "" {
		out.FromAddress = v
	}
	// SMTP 段整体覆盖：允许把 host 清空（表示尚未配置），因此不做「非空才覆盖」判断。
	out.SMTP.Host = mapString(value, "host")
	out.SMTP.Port = mapInt(value, "port")
	out.SMTP.Username = mapString(value, "username")
	if _, ok := value["tls"]; ok {
		out.SMTP.TLS = mapBool(value, "tls")
	}

	encrypted := mapString(value, "password_enc")
	if encrypted != "" {
		plain, err := secret.Decrypt(mailSecretKey(cfg), encrypted)
		if err != nil {
			return out, fmt.Errorf("SMTP 密码解密失败（主密钥可能已变更，请在后台重新填写）: %w", err)
		}
		out.SMTP.Password = plain
	} else {
		out.SMTP.Password = ""
	}
	return out, nil
}

// encodeMailSetting 把配置对象编码为落库的 JSON（密码加密）。
func encodeMailSetting(cfg *config.Config, m config.Mail) (map[string]any, error) {
	encrypted, err := secret.Encrypt(mailSecretKey(cfg), m.SMTP.Password)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"driver":       m.Driver,
		"from_name":    m.FromName,
		"from_address": m.FromAddress,
		"host":         m.SMTP.Host,
		"port":         m.SMTP.Port,
		"username":     m.SMTP.Username,
		"tls":          m.SMTP.TLS,
		"password_enc": encrypted,
	}, nil
}

// Get 返回当前生效的邮件配置（密码以占位符呈现）。
func (svc *MailService) Get(ctx context.Context) (MailConfigView, error) {
	effective, fromDB, err := ResolveMailConfig(ctx, svc.s.Store, svc.s.Cfg)
	if err != nil {
		return MailConfigView{}, apperr.ErrSettingInvalid.WithCause(err)
	}
	return svc.toView(ctx, effective, fromDB), nil
}

// Save 保存在线修改的邮件配置并立即热生效。
func (svc *MailService) Save(ctx context.Context, in MailConfigInput, actor Actor) (MailConfigView, error) {
	current, _, err := ResolveMailConfig(ctx, svc.s.Store, svc.s.Cfg)
	if err != nil {
		return MailConfigView{}, apperr.ErrSettingInvalid.WithCause(err)
	}

	next := config.Mail{
		Driver:      strings.ToLower(strings.TrimSpace(in.Driver)),
		FromName:    strings.TrimSpace(in.FromName),
		FromAddress: strings.TrimSpace(in.FromAddress),
		SMTP: config.SMTP{
			Host:     strings.TrimSpace(in.Host),
			Port:     in.Port,
			Username: strings.TrimSpace(in.Username),
			TLS:      in.TLS != nil && *in.TLS,
		},
	}
	if next.Driver == "" {
		next.Driver = "console"
	}

	// 密码处理：显式清空 > 新密码 > 沿用旧密码。
	//
	// 新密码统一走 trimMailSecret：首尾空白是复制授权码的产物，自动去除并留一条
	// 不含口令的告警。安装向导那条路径（SetupAdmin → Save）同样经由这里，
	// 因此两条路径的行为天然一致。
	//
	// 先 trim 再与占位符比较：否则「占位符外多了一个空格」会被当成真实新口令存下去。
	passwordIn := strings.TrimSpace(in.Password)
	passwordInherited := false
	switch {
	case in.ClearPassword:
		next.SMTP.Password = ""
	case passwordIn == "" || passwordIn == MailPasswordMask:
		next.SMTP.Password = current.SMTP.Password
		passwordInherited = true
	default:
		next.SMTP.Password = trimMailSecret(in.Password, next.SMTP.Host, next.SMTP.Username)
	}

	// 改了用户名却继续沿用旧密码，几乎总是失误（新账号配旧口令），
	// 而且会静默保存出一个必然认证失败的配置——那种配置要等到用户注册收不到验证码
	// 才会被发现。这里直接拦下来，要求同时提供新密码。
	if passwordInherited && next.SMTP.Username != current.SMTP.Username {
		return MailConfigView{}, apperr.ErrSettingInvalid.
			WithMessage("修改 SMTP 用户名时必须同时填写新密码（不能沿用旧密码）")
	}

	if err := next.Validate(); err != nil {
		return MailConfigView{}, apperr.ErrSettingInvalid.WithMessage(err.Error())
	}

	value, err := encodeMailSetting(svc.s.Cfg, next)
	if err != nil {
		return MailConfigView{}, apperr.ErrInternal.WithCause(err)
	}

	if err := svc.persist(ctx, value, actor); err != nil {
		return MailConfigView{}, err
	}

	// 热替换投递器：配置保存成功即生效，无需重启服务。
	mailer, err := mail.New(next)
	if err != nil {
		return MailConfigView{}, apperr.ErrSettingInvalid.WithMessage("邮件配置无法生效：" + err.Error())
	}
	svc.s.Mailer.Swap(mailer)

	// 审计只记录可公开的字段与「密码是否变化」，绝不写入口令本身。
	svc.s.Audit.Record(ctx, actor, "mail.update", "setting", SettingKeyMailSMTP, map[string]any{
		"driver":           next.Driver,
		"from_address":     next.FromAddress,
		"host":             next.SMTP.Host,
		"port":             next.SMTP.Port,
		"username":         next.SMTP.Username,
		"tls":              next.SMTP.TLS,
		"password_changed": in.ClearPassword || (in.Password != "" && in.Password != MailPasswordMask),
	})

	return svc.toView(ctx, next, true), nil
}

// SendTest 使用当前配置发送一封测试邮件。
//
// 这是后台邮件设置页最实用的按钮：不真的发一封信，管理员无法确认
// host/port/TLS/授权码是否都填对了。
//
// 失败文案由 describeMailFailure 统一生成：认证失败（535 等）会得到
// 专用错误码 10009 + 排查步骤 + 服务器原文，且**不再**出现「服务器内部错误」——
// 详见 internal/service/mail_hint.go。
func (svc *MailService) SendTest(ctx context.Context, to string, actor Actor) error {
	to = strings.TrimSpace(to)
	if !looksLikeEmail(to) {
		return apperr.ErrInvalidParams.WithMessage("收件邮箱格式不正确")
	}

	driver := svc.s.Mailer.Name()
	if err := svc.s.Mailer.Send(ctx, mail.TestMessage(to, driver, time.Now())); err != nil {
		svc.s.Audit.Record(ctx, actor, "mail.test_failed", "setting", SettingKeyMailSMTP, map[string]any{
			"to": to, "driver": driver,
			// 错误文本已经过口令脱敏（见 internal/mail/errors.go），
			// 这里额外记一个「是否为认证失败」，便于事后按配置问题筛选审计。
			"error":       err.Error(),
			"auth_failed": errors.Is(err, mail.ErrAuthFailed),
		})
		return describeMailFailure("测试邮件发送失败", err)
	}

	svc.s.Audit.Record(ctx, actor, "mail.test", "setting", SettingKeyMailSMTP, map[string]any{
		"to": to, "driver": driver,
	})
	return nil
}

// Revert 删除后台设置，回落到 configs/config.yaml 中的邮件配置。
func (svc *MailService) Revert(ctx context.Context, actor Actor) (MailConfigView, error) {
	if _, err := svc.s.Store.Ent.Setting.Delete().
		Where(setting.KeyEQ(SettingKeyMailSMTP)).Exec(ctx); err != nil {
		return MailConfigView{}, apperr.ErrInternal.WithCause(err)
	}

	mailer, err := mail.New(svc.s.Cfg.Mail)
	if err != nil {
		return MailConfigView{}, apperr.ErrInternal.WithCause(err)
	}
	svc.s.Mailer.Swap(mailer)

	svc.s.Audit.Record(ctx, actor, "mail.revert", "setting", SettingKeyMailSMTP, nil)
	return svc.toView(ctx, svc.s.Cfg.Mail, false), nil
}

// EnsureDefaults 首次启动时把配置文件中的邮件配置写入数据库（幂等，不覆盖已有设置）。
func (svc *MailService) EnsureDefaults(ctx context.Context, actor Actor) error {
	exists, err := svc.s.Store.Ent.Setting.Query().Where(setting.KeyEQ(SettingKeyMailSMTP)).Exist(ctx)
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	if exists {
		return nil
	}

	value, err := encodeMailSetting(svc.s.Cfg, svc.s.Cfg.Mail)
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return svc.persist(ctx, value, actor)
}

// persist 写入或覆盖邮件配置行。
func (svc *MailService) persist(ctx context.Context, value map[string]any, actor Actor) error {
	const description = "邮件投递配置：可由后台在线修改，SMTP 密码加密存储"

	existing, err := svc.s.Store.Ent.Setting.Query().Where(setting.KeyEQ(SettingKeyMailSMTP)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		if _, err := svc.s.Store.Ent.Setting.Create().
			SetKey(SettingKeyMailSMTP).
			SetValue(value).
			SetDescription(description).
			SetGroup("mail").
			SetIsSystem(true).
			SetUpdatedBy(actor.Email).
			Save(ctx); err != nil {
			return apperr.ErrInternal.WithCause(err)
		}
		return nil
	case err != nil:
		return apperr.ErrInternal.WithCause(err)
	}

	if _, err := svc.s.Store.Ent.Setting.UpdateOneID(existing.ID).
		SetValue(value).
		SetDescription(description).
		SetGroup("mail").
		SetIsSystem(true).
		SetUpdatedBy(actor.Email).
		Save(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// toView 组装视图，密码只以占位符暴露。
func (svc *MailService) toView(ctx context.Context, m config.Mail, fromDatabase bool) MailConfigView {
	view := MailConfigView{
		Driver:       m.Driver,
		FromName:     m.FromName,
		FromAddress:  m.FromAddress,
		Host:         m.SMTP.Host,
		Port:         m.SMTP.Port,
		Username:     m.SMTP.Username,
		TLS:          m.SMTP.TLS,
		HasPassword:  m.SMTP.Password != "",
		PasswordMask: MailPasswordMask,
		Source:       "config",
		ActiveDriver: svc.s.Mailer.Name(),
		Ready:        svc.s.Mailer.Current() != nil,
	}
	if fromDatabase {
		view.Source = "database"
		if row, err := svc.s.Store.Ent.Setting.Query().
			Where(setting.KeyEQ(SettingKeyMailSMTP)).Only(ctx); err == nil {
			view.UpdatedBy = row.UpdatedBy
			updated := row.UpdateTime
			view.UpdatedAt = &updated
		}
	}
	return view
}
