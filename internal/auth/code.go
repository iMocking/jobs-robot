package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// CodePurpose 是邮箱验证码的用途。
type CodePurpose string

// 验证码用途枚举，与 ent 中 VerificationCode.purpose 的取值保持一致。
const (
	CodePurposeRegister      CodePurpose = "register"
	CodePurposeResetPassword CodePurpose = "reset_password"
	CodePurposeChangeEmail   CodePurpose = "change_email"
	CodePurposeLogin         CodePurpose = "login"
	// CodePurposeRoleChange 用于管理员在后台变更用户身份时，向操作人本人邮箱发送确认码。
	CodePurposeRoleChange CodePurpose = "role_change"
)

// Valid 判断用途是否合法。
func (p CodePurpose) Valid() bool {
	switch p {
	case CodePurposeRegister, CodePurposeResetPassword, CodePurposeChangeEmail, CodePurposeLogin, CodePurposeRoleChange:
		return true
	default:
		return false
	}
}

// CodeManager 负责邮箱验证码的生成、投递前的频控与校验。
//
// 验证码只以 HMAC-SHA256 形式存放在 Redis（带 TTL），数据库中仅留审计记录。
type CodeManager struct {
	rdb      redis.UniversalClient
	prefix   string
	secret   []byte
	length   int
	ttl      time.Duration
	cooldown time.Duration
	maxTries int
	dailyMax int
}

// CodeOptions 是验证码管理器参数。
type CodeOptions struct {
	KeyPrefix   string
	Secret      []byte
	Length      int
	TTL         time.Duration
	Cooldown    time.Duration
	MaxAttempts int
	DailyLimit  int
}

// NewCodeManager 构造验证码管理器。
func NewCodeManager(rdb redis.UniversalClient, opts CodeOptions) *CodeManager {
	if opts.Length <= 0 {
		opts.Length = 6
	}
	if opts.TTL <= 0 {
		opts.TTL = 10 * time.Minute
	}
	if opts.Cooldown <= 0 {
		opts.Cooldown = time.Minute
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 5
	}
	if opts.DailyLimit <= 0 {
		opts.DailyLimit = 20
	}
	return &CodeManager{
		rdb:      rdb,
		prefix:   opts.KeyPrefix,
		secret:   opts.Secret,
		length:   opts.Length,
		ttl:      opts.TTL,
		cooldown: opts.Cooldown,
		maxTries: opts.MaxAttempts,
		dailyMax: opts.DailyLimit,
	}
}

// IssuedCode 是一次验证码发放的结果。
type IssuedCode struct {
	Email     string
	Purpose   CodePurpose
	Code      string
	Hash      string
	ExpiresAt time.Time
}

// TTL 返回验证码有效期。
func (m *CodeManager) TTL() time.Duration { return m.ttl }

type codeRecord struct {
	Hash     string    `json:"hash"`
	Attempts int       `json:"attempts"`
	IssuedAt time.Time `json:"issued_at"`
}

func (m *CodeManager) codeKey(email string, purpose CodePurpose) string {
	return m.prefix + "code:" + string(purpose) + ":" + email
}

func (m *CodeManager) cooldownKey(email string, purpose CodePurpose) string {
	return m.prefix + "code_cooldown:" + string(purpose) + ":" + email
}

func (m *CodeManager) dailyKey(email string) string {
	return m.prefix + "code_daily:" + email + ":" + time.Now().Format("20060102")
}

// Issue 生成验证码并写入 Redis。
//
// 频控：同一邮箱同一用途 cooldown 内只能发一次；每日总量受 dailyMax 限制。
func (m *CodeManager) Issue(ctx context.Context, email string, purpose CodePurpose) (*IssuedCode, error) {
	email = normalizeEmail(email)
	if !purpose.Valid() {
		return nil, apperr.ErrInvalidParams.WithMessage("验证码用途不合法")
	}

	// 1) 发送冷却
	exists, err := m.rdb.Exists(ctx, m.cooldownKey(email, purpose)).Result()
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	if exists > 0 {
		ttl, _ := m.rdb.TTL(ctx, m.cooldownKey(email, purpose)).Result()
		return nil, apperr.ErrCodeTooFrequent.WithDetails(map[string]any{
			"retry_after_seconds": int(ttl.Seconds()) + 1,
		})
	}

	// 2) 每日上限
	daily, err := m.rdb.Get(ctx, m.dailyKey(email)).Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	if daily >= m.dailyMax {
		return nil, apperr.ErrCodeDailyLimit.WithDetails(map[string]any{"daily_limit": m.dailyMax})
	}

	// 3) 生成验证码
	code, err := randomDigits(m.length)
	if err != nil {
		return nil, err
	}
	hash := m.Hash(email, purpose, code)
	record, err := json.Marshal(codeRecord{Hash: hash, IssuedAt: time.Now()})
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	pipe := m.rdb.TxPipeline()
	pipe.Set(ctx, m.codeKey(email, purpose), string(record), m.ttl)
	pipe.Set(ctx, m.cooldownKey(email, purpose), "1", m.cooldown)
	pipe.Incr(ctx, m.dailyKey(email))
	pipe.Expire(ctx, m.dailyKey(email), 24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	return &IssuedCode{
		Email:     email,
		Purpose:   purpose,
		Code:      code,
		Hash:      hash,
		ExpiresAt: time.Now().Add(m.ttl),
	}, nil
}

// Verify 校验验证码；校验成功即消费（一次性），失败会累计尝试次数。
func (m *CodeManager) Verify(ctx context.Context, email string, purpose CodePurpose, code string) error {
	email = normalizeEmail(email)
	key := m.codeKey(email, purpose)

	raw, err := m.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return apperr.ErrCodeInvalid
	}
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}

	var record codeRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		_ = m.rdb.Del(ctx, key).Err()
		return apperr.ErrCodeInvalid
	}

	expected := m.Hash(email, purpose, code)
	if !hmac.Equal([]byte(expected), []byte(record.Hash)) {
		record.Attempts++
		if record.Attempts >= m.maxTries {
			// 达到尝试上限：作废验证码，迫使重新获取。
			_ = m.rdb.Del(ctx, key).Err()
			return apperr.ErrCodeInvalid.WithMessage("验证码错误次数过多，请重新获取")
		}
		if updated, err := json.Marshal(record); err == nil {
			// KEEPTTL：不因重写而延长验证码寿命。
			_ = m.rdb.SetArgs(ctx, key, string(updated), redis.SetArgs{KeepTTL: true}).Err()
		}
		return apperr.ErrCodeInvalid.WithDetails(map[string]any{
			"remaining_attempts": m.maxTries - record.Attempts,
		})
	}

	// 校验通过：立即消费并解除冷却，避免同一验证码重复使用。
	pipe := m.rdb.TxPipeline()
	pipe.Del(ctx, key)
	pipe.Del(ctx, m.cooldownKey(email, purpose))
	if _, err := pipe.Exec(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// Invalidate 主动作废某用途下的验证码。
func (m *CodeManager) Invalidate(ctx context.Context, email string, purpose CodePurpose) error {
	email = normalizeEmail(email)
	pipe := m.rdb.TxPipeline()
	pipe.Del(ctx, m.codeKey(email, purpose))
	pipe.Del(ctx, m.cooldownKey(email, purpose))
	if _, err := pipe.Exec(ctx); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// Hash 计算验证码摘要：HMAC-SHA256(secret, email|purpose|code)。
//
// 绑定邮箱与用途，即使摘要泄漏也无法跨账号/跨用途复用。
func (m *CodeManager) Hash(email string, purpose CodePurpose, code string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(normalizeEmail(email)))
	mac.Write([]byte("|"))
	mac.Write([]byte(purpose))
	mac.Write([]byte("|"))
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

// normalizeEmail 统一小写并去除首尾空白。
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NormalizeEmail 供其他包复用同一套邮箱归一化规则。
func NormalizeEmail(email string) string { return normalizeEmail(email) }

// randomDigits 生成加密安全的定长数字串（避免 math/rand 可预测问题）。
func randomDigits(length int) (string, error) {
	var sb strings.Builder
	sb.Grow(length)
	max := big.NewInt(10)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", apperr.ErrInternal.WithCause(err)
		}
		sb.WriteString(strconv.Itoa(int(n.Int64())))
	}
	return sb.String(), nil
}
