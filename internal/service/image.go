package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/setting"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/secret"
	"github.com/jsong/jobs-robot/internal/pkg/ulidid"
	"github.com/jsong/jobs-robot/internal/storage"
	"github.com/jsong/jobs-robot/internal/store"
)

// SettingKeyImageStorage 是图片服务配置在 settings 表中的键。
const SettingKeyImageStorage = "storage.image"

// ImageSecretMask 是 OSS AccessKeySecret 字段的占位符。
//
// 与 MailPasswordMask 同一约定：后台接口永不回显已保存的密钥，
// 前端拿到占位符原样回传即表示「不修改密钥」。
const ImageSecretMask = "********"

// ImageConfigView 是图片服务配置视图。
type ImageConfigView struct {
	Driver string `json:"driver"`

	// 本地驱动参数
	LocalDir      string `json:"local_dir"`
	PublicBaseURL string `json:"public_base_url"`

	// OSS 驱动参数
	Endpoint    string `json:"oss_endpoint"`
	Bucket      string `json:"oss_bucket"`
	AccessKeyID string `json:"oss_access_key_id"`

	HasSecret  bool   `json:"has_secret"`
	SecretMask string `json:"secret_mask"`

	// Source 说明当前生效配置来自哪里：config（配置文件）或 database（后台设置）。
	Source string `json:"source"`
	// Ready 表示按当前配置能否构造出可用的存储驱动。
	Ready bool `json:"ready"`

	UpdatedBy string     `json:"updated_by,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// ImageConfigInput 是保存图片服务配置的入参。
type ImageConfigInput struct {
	Driver string
	// 本地驱动
	LocalDir      string
	PublicBaseURL string
	// OSS 驱动
	Endpoint    string
	Bucket      string
	AccessKeyID string
	// Secret 为空或等于占位符时表示沿用已保存的密钥。
	Secret string
	// ClearSecret 显式清空已保存的密钥。
	ClearSecret bool
}

// ImageService 负责图片服务配置的读取、在线修改、热生效与头像上传。
type ImageService struct{ s *Services }

// imageSecretKey 派生用于加密 OSS AccessKeySecret 的子密钥（用途与 SMTP 密码隔离）。
func imageSecretKey(cfg *config.Config) []byte {
	return secret.DeriveKey(cfg.JWT.Secret, "image-storage")
}

// ResolveImageConfig 计算「最终生效」的图片存储配置：以配置文件为基线，被数据库设置覆盖。
//
// 与 ResolveMailConfig 同理做成包级函数：上传与静态文件路由都要在
// 任意时刻拿到「当前生效」的配置，而不是依赖服务启动时的快照。
// 返回的 fromDatabase 表示当前配置是否来自后台设置（用于界面提示）。
func ResolveImageConfig(ctx context.Context, st *store.Store, cfg *config.Config) (config.ImageStorage, bool, error) {
	row, err := st.Ent.Setting.Query().Where(setting.KeyEQ(SettingKeyImageStorage)).Only(ctx)
	if ent.IsNotFound(err) {
		return cfg.Image, false, nil
	}
	if err != nil {
		return cfg.Image, false, fmt.Errorf("读取图片服务配置失败: %w", err)
	}

	merged, err := decodeImageSetting(cfg, row.Value)
	if err != nil {
		return cfg.Image, false, err
	}
	return merged, true, nil
}

// decodeImageSetting 把数据库中的 JSON 还原为配置对象，并把加密的 OSS 密钥解出来。
func decodeImageSetting(cfg *config.Config, value map[string]any) (config.ImageStorage, error) {
	out := cfg.Image

	if v := mapString(value, "driver"); v != "" {
		out.Driver = v
	}
	if _, ok := value["local_dir"]; ok {
		out.LocalDir = mapString(value, "local_dir")
	}
	if _, ok := value["public_base_url"]; ok {
		out.PublicBaseURL = mapString(value, "public_base_url")
	}
	if _, ok := value["oss_endpoint"]; ok {
		out.OSS.Endpoint = mapString(value, "oss_endpoint")
	}
	if _, ok := value["oss_bucket"]; ok {
		out.OSS.Bucket = mapString(value, "oss_bucket")
	}
	if _, ok := value["oss_access_key_id"]; ok {
		out.OSS.AccessKeyID = mapString(value, "oss_access_key_id")
	}

	encrypted := mapString(value, "oss_access_key_secret_enc")
	if encrypted != "" {
		plain, err := secret.Decrypt(imageSecretKey(cfg), encrypted)
		if err != nil {
			return out, fmt.Errorf("OSS 密钥解密失败（主密钥可能已变更，请在后台重新填写）: %w", err)
		}
		out.OSS.AccessKeySecret = plain
	} else {
		out.OSS.AccessKeySecret = ""
	}
	return out, nil
}

// encodeImageSetting 把配置对象编码为落库的 JSON（OSS 密钥加密）。
func encodeImageSetting(cfg *config.Config, img config.ImageStorage) (map[string]any, error) {
	encrypted, err := secret.Encrypt(imageSecretKey(cfg), img.OSS.AccessKeySecret)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"driver":                        img.Driver,
		"local_dir":                     img.LocalDir,
		"public_base_url":               img.PublicBaseURL,
		"oss_endpoint":                  img.OSS.Endpoint,
		"oss_bucket":                    img.OSS.Bucket,
		"oss_access_key_id":             img.OSS.AccessKeyID,
		"oss_access_key_secret_enc":     encrypted,
	}, nil
}

// Get 返回当前生效的图片服务配置（密钥以占位符呈现）。
func (svc *ImageService) Get(ctx context.Context) (ImageConfigView, error) {
	effective, fromDB, err := ResolveImageConfig(ctx, svc.s.Store, svc.s.Cfg)
	if err != nil {
		return ImageConfigView{}, apperr.ErrSettingInvalid.WithCause(err)
	}
	return svc.toView(ctx, effective, fromDB), nil
}

// Save 保存在线修改的图片服务配置并立即热生效。
func (svc *ImageService) Save(ctx context.Context, in ImageConfigInput, actor Actor) (ImageConfigView, error) {
	current, _, err := ResolveImageConfig(ctx, svc.s.Store, svc.s.Cfg)
	if err != nil {
		return ImageConfigView{}, apperr.ErrSettingInvalid.WithCause(err)
	}

	next := config.ImageStorage{
		Driver:        strings.ToLower(strings.TrimSpace(in.Driver)),
		LocalDir:      strings.TrimSpace(in.LocalDir),
		PublicBaseURL: strings.TrimSpace(in.PublicBaseURL),
		OSS: config.ImageOSS{
			Endpoint:    strings.TrimSpace(in.Endpoint),
			Bucket:      strings.TrimSpace(in.Bucket),
			AccessKeyID: strings.TrimSpace(in.AccessKeyID),
		},
	}
	if next.Driver == "" {
		next.Driver = "local"
	}

	// 密钥处理：显式清空 > 新密钥 > 沿用旧密钥（与邮件配置同一约定）。
	secretIn := strings.TrimSpace(in.Secret)
	secretInherited := false
	switch {
	case in.ClearSecret:
		next.OSS.AccessKeySecret = ""
	case secretIn == "" || secretIn == ImageSecretMask:
		next.OSS.AccessKeySecret = current.OSS.AccessKeySecret
		secretInherited = true
	default:
		next.OSS.AccessKeySecret = secretIn
	}

	// 改了 AccessKeyID 却沿用旧密钥，几乎总是配置失误：新账号配旧口令
	// 会静默保存出一个必然上传失败的配置，这里直接拦下来。
	if secretInherited && next.OSS.AccessKeyID != current.OSS.AccessKeyID {
		return ImageConfigView{}, apperr.ErrSettingInvalid.
			WithMessage("修改 OSS AccessKeyID 时必须同时填写新的 AccessKeySecret（不能沿用旧密钥）")
	}

	if err := next.Validate(); err != nil {
		return ImageConfigView{}, apperr.ErrSettingInvalid.WithMessage(err.Error())
	}

	value, err := encodeImageSetting(svc.s.Cfg, next)
	if err != nil {
		return ImageConfigView{}, apperr.ErrInternal.WithCause(err)
	}
	if err := svc.persist(ctx, value, actor); err != nil {
		return ImageConfigView{}, err
	}

	// 审计只记录可公开字段与「密钥是否变化」，绝不写入密钥本身。
	svc.s.Audit.Record(ctx, actor, "image.update", "setting", SettingKeyImageStorage, map[string]any{
		"driver":           next.Driver,
		"local_dir":        next.LocalDir,
		"public_base_url":  next.PublicBaseURL,
		"oss_endpoint":     next.OSS.Endpoint,
		"oss_bucket":       next.OSS.Bucket,
		"oss_access_key_id": next.OSS.AccessKeyID,
		"secret_changed":   in.ClearSecret || (in.Secret != "" && in.Secret != ImageSecretMask),
	})

	return svc.toView(ctx, next, true), nil
}

// Revert 删除后台设置，回落到 configs/config.yaml 中的图片存储配置。
func (svc *ImageService) Revert(ctx context.Context, actor Actor) (ImageConfigView, error) {
	if _, err := svc.s.Store.Ent.Setting.Delete().
		Where(setting.KeyEQ(SettingKeyImageStorage)).Exec(ctx); err != nil {
		return ImageConfigView{}, apperr.ErrInternal.WithCause(err)
	}

	svc.s.Audit.Record(ctx, actor, "image.revert", "setting", SettingKeyImageStorage, nil)
	return svc.toView(ctx, svc.s.Cfg.Image, false), nil
}

// EnsureDefaults 首次启动时把配置文件中的图片存储配置写入数据库（幂等，不覆盖已有设置）。
func (svc *ImageService) EnsureDefaults(ctx context.Context, actor Actor) error {
	exists, err := svc.s.Store.Ent.Setting.Query().Where(setting.KeyEQ(SettingKeyImageStorage)).Exist(ctx)
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	if exists {
		return nil
	}

	value, err := encodeImageSetting(svc.s.Cfg, svc.s.Cfg.Image)
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return svc.persist(ctx, value, actor)
}

// persist 写入或覆盖图片服务配置行。
func (svc *ImageService) persist(ctx context.Context, value map[string]any, actor Actor) error {
	const description = "图片存储服务配置：本地目录或 OSS，可由后台在线修改，OSS 密钥加密存储"

	existing, err := svc.s.Store.Ent.Setting.Query().Where(setting.KeyEQ(SettingKeyImageStorage)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		if _, err := svc.s.Store.Ent.Setting.Create().
			SetKey(SettingKeyImageStorage).
			SetValue(value).
			SetDescription(description).
			SetGroup("storage").
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
		SetGroup("storage").
		SetIsSystem(true).
		SetUpdatedBy(actor.Email).
		Save(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// toView 组装视图，密钥只以占位符暴露。
func (svc *ImageService) toView(ctx context.Context, img config.ImageStorage, fromDatabase bool) ImageConfigView {
	view := ImageConfigView{
		Driver:        img.Driver,
		LocalDir:      img.LocalDir,
		PublicBaseURL: img.PublicBaseURL,
		Endpoint:      img.OSS.Endpoint,
		Bucket:        img.OSS.Bucket,
		AccessKeyID:   img.OSS.AccessKeyID,
		HasSecret:     img.OSS.AccessKeySecret != "",
		SecretMask:    ImageSecretMask,
		Source:        "config",
	}
	if _, err := svc.buildStore(img); err == nil {
		view.Ready = true
	}
	if fromDatabase {
		view.Source = "database"
		if row, err := svc.s.Store.Ent.Setting.Query().
			Where(setting.KeyEQ(SettingKeyImageStorage)).Only(ctx); err == nil {
			view.UpdatedBy = row.UpdatedBy
			updated := row.UpdateTime
			view.UpdatedAt = &updated
		}
	}
	return view
}

// Store 返回按当前生效配置构造的图片存储驱动。
//
// 每次调用都重新解析配置并构造：上传频率低，换来「后台改配置后立即生效、
// 无需重启、无需缓存失效」的一致行为。
func (svc *ImageService) Store(ctx context.Context) (storage.ImageStore, error) {
	effective, _, err := ResolveImageConfig(ctx, svc.s.Store, svc.s.Cfg)
	if err != nil {
		return nil, apperr.ErrSettingInvalid.WithCause(err)
	}
	return svc.buildStore(effective)
}

// buildStore 按配置构造存储驱动；配置不合法时返回可读错误。
func (svc *ImageService) buildStore(img config.ImageStorage) (storage.ImageStore, error) {
	switch img.Driver {
	case "local":
		// 本地模式未配置公网前缀时，URL 走本服务自身的 <api_prefix>/files/ 路由。
		baseURL := img.PublicBaseURL
		if baseURL == "" && svc.s.Cfg != nil {
			baseURL = svc.s.Cfg.App.APIPrefix + "/files"
		}
		return storage.NewLocal(img.LocalDir, baseURL)
	case "oss":
		return storage.NewOSS(
			img.OSS.Endpoint,
			img.OSS.Bucket,
			img.OSS.AccessKeyID,
			img.OSS.AccessKeySecret,
			img.OSS.PublicURL,
		)
	default:
		return nil, fmt.Errorf("图片存储驱动只支持 local/oss，当前为 %q", img.Driver)
	}
}

// UploadAvatar 上传当前用户头像：校验图片 → 存储 → 更新 avatar_url → 返回用户视图。
//
// 文件内容与类型由 storage.SniffImage 按文件头判定（不信任扩展名），
// 大小上限为 storage.MaxImageBytes（10MB）。系统机器身份（kind=system）
// 不允许上传头像。
func (svc *ImageService) UploadAvatar(ctx context.Context, userID string, file io.Reader, size int64) (UserView, error) {
	user, err := svc.s.Store.Ent.User.Get(ctx, userID)
	if ent.IsNotFound(err) {
		return UserView{}, apperr.ErrUserNotFound
	}
	if err != nil {
		return UserView{}, apperr.ErrInternal.WithCause(err)
	}
	if user.Kind == entuser.KindSystem {
		return UserView{}, apperr.ErrInvalidParams.WithMessage("系统身份不可修改头像")
	}
	if size <= 0 {
		return UserView{}, apperr.ErrInvalidParams.WithMessage("请选择要上传的图片文件")
	}
	if size > storage.MaxImageBytes {
		return UserView{}, apperr.ErrInvalidParams.
			WithMessage("图片不能超过 10MB").
			WithDetails(map[string]any{"max_bytes": storage.MaxImageBytes})
	}

	// 读文件头判定类型，再把头部接回原流，保证上传的完整内容一致。
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	contentType, err := storage.SniffImage(io.MultiReader(bytes.NewReader(head[:n]), io.LimitReader(file, size-int64(n))))
	if err != nil {
		return UserView{}, apperr.ErrInvalidParams.WithMessage(err.Error())
	}

	store, err := svc.Store(ctx)
	if err != nil {
		return UserView{}, apperr.ErrSettingInvalid.
			WithMessage("图片服务未配置就绪：" + err.Error())
	}

	key := fmt.Sprintf("avatars/%s/%s%s", userID, ulidid.New(), storage.ExtensionFor(contentType))
	reader := io.MultiReader(bytes.NewReader(head[:n]), io.LimitReader(file, size-int64(n)))
	url, err := store.Save(ctx, key, reader, contentType, size)
	if err != nil {
		return UserView{}, apperr.ErrInternal.WithCause(err)
	}

	saved, err := svc.s.Store.Ent.User.UpdateOneID(userID).
		SetAvatarURL(truncateRunes(url, 512)).
		Save(ctx)
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
	svc.s.Audit.Record(ctx, Actor{UserID: userID, Email: user.Email}, "user.update_avatar", "user", userID, nil)
	return NewUserView(saved, roles), nil
}
