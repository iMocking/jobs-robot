package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// TokenTypeAccess 表示访问令牌。
const TokenTypeAccess = "access"

// Claims 是访问令牌载荷。
type Claims struct {
	Email string   `json:"email"`
	Roles []string `json:"roles"`
	// PasswordVersion 与 Redis 中的 user pwdv 比对，改密后旧令牌立即失效。
	PasswordVersion int `json:"pv"`
	// SessionID 用于校验会话是否已被登出/吊销。
	SessionID string `json:"sid"`
	Type      string `json:"typ"`
	jwt.RegisteredClaims
}

// UserID 从 sub 声明解析用户 ID（ULID 字符串）。
func (c *Claims) UserID() (string, error) {
	id := c.Subject
	if id == "" {
		return "", apperr.ErrTokenInvalid
	}
	return id, nil
}

// JWTManager 负责访问令牌的签发与校验。
type JWTManager struct {
	secret    []byte
	issuer    string
	accessTTL time.Duration
	// leeway 是允许的时钟偏差，用于容忍多实例之间的微小时间差。
	leeway time.Duration
}

// NewJWTManager 构造令牌管理器。
func NewJWTManager(cfg config.JWT) *JWTManager {
	return &JWTManager{
		secret:    []byte(cfg.Secret),
		issuer:    cfg.Issuer,
		accessTTL: cfg.AccessTTL,
		leeway:    30 * time.Second,
	}
}

// AccessTTL 返回访问令牌有效期。
func (m *JWTManager) AccessTTL() time.Duration { return m.accessTTL }

// Issue 签发访问令牌，返回令牌与其过期时间。
func (m *JWTManager) Issue(userID string, email string, roles []string, passwordVersion int, sessionID string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.accessTTL)
	if roles == nil {
		roles = []string{}
	}
	claims := &Claims{
		Email:           email,
		Roles:           roles,
		PasswordVersion: passwordVersion,
		SessionID:       sessionID,
		Type:            TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    m.issuer,
			ID:        sessionID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, apperr.ErrInternal.WithCause(err)
	}
	return signed, expiresAt, nil
}

// Parse 校验并解析访问令牌，失败时返回带错误码的业务错误。
func (m *JWTManager) Parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		// 只接受 HMAC 算法，防止 alg=none / RS256 混淆攻击。
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, apperr.ErrTokenInvalid
		}
		return m.secret, nil
	},
		jwt.WithIssuer(m.issuer),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithLeeway(m.leeway),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperr.ErrTokenExpired
		}
		return nil, apperr.ErrTokenInvalid.WithCause(err)
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, apperr.ErrTokenInvalid
	}
	if claims.Type != TokenTypeAccess {
		return nil, apperr.ErrTokenInvalid.WithMessage("令牌类型不正确")
	}
	if claims.SessionID == "" {
		return nil, apperr.ErrTokenInvalid
	}
	return claims, nil
}
