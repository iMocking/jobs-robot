package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// SessionMeta 是会话的来源信息。
type SessionMeta struct {
	UserAgent string
	IP        string
}

// Session 是服务端持有的登录会话。
type Session struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Email           string    `json:"email"`
	PasswordVersion int       `json:"password_version"`
	UserAgent       string    `json:"user_agent"`
	IP              string    `json:"ip"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	RefreshHash     string    `json:"refresh_hash"`
}

// TokenPair 是签发给客户端的令牌组合。
type TokenPair struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	ExpiresIn        int       `json:"expires_in"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	SessionID        string    `json:"session_id"`
}

// SessionManager 管理登录会话与刷新令牌。
//
// 设计取舍：
//   - 访问令牌是无状态 JWT，但校验时会额外确认「会话未被登出」与「密码版本未变」，
//     因此登出与改密能够立即生效，而不必等访问令牌自然过期。
//   - 刷新令牌是随机串，只在 Redis 中保存其 SHA-256，库被读取也无法直接冒用。
type SessionManager struct {
	rdb        redis.UniversalClient
	prefix     string
	jwt        *JWTManager
	refreshTTL time.Duration
}

// NewSessionManager 构造会话管理器。
func NewSessionManager(rdb redis.UniversalClient, keyPrefix string, jwtManager *JWTManager, refreshTTL time.Duration) *SessionManager {
	return &SessionManager{rdb: rdb, prefix: keyPrefix, jwt: jwtManager, refreshTTL: refreshTTL}
}

// RefreshTTL 返回刷新令牌有效期。
func (m *SessionManager) RefreshTTL() time.Duration { return m.refreshTTL }

func (m *SessionManager) sessionKey(sid string) string { return m.prefix + "session:" + sid }
func (m *SessionManager) refreshKey(token string) string {
	return m.prefix + "refresh:" + hashToken(token)
}
func (m *SessionManager) userSessionsKey(uid string) string {
	return m.prefix + "user_sessions:" + uid
}
func (m *SessionManager) pwdVersionKey(uid string) string {
	return m.prefix + "pwdv:" + uid
}

// Issue 创建新会话并签发令牌对。
func (m *SessionManager) Issue(ctx context.Context, userID string, email string, roles []string, passwordVersion int, meta SessionMeta) (*TokenPair, error) {
	sid, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	refreshToken, err := randomToken(32)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	sess := Session{
		ID:              sid,
		UserID:          userID,
		Email:           email,
		PasswordVersion: passwordVersion,
		UserAgent:       truncate(meta.UserAgent, 512),
		IP:              truncate(meta.IP, 64),
		CreatedAt:       now,
		ExpiresAt:       now.Add(m.refreshTTL),
		RefreshHash:     hashToken(refreshToken),
	}

	accessToken, accessExp, err := m.jwt.Issue(userID, email, roles, passwordVersion, sid)
	if err != nil {
		return nil, err
	}

	pipe := m.rdb.TxPipeline()
	pipe.Set(ctx, m.sessionKey(sid), mustJSON(sess), m.refreshTTL)
	pipe.Set(ctx, m.refreshKey(refreshToken), sid, m.refreshTTL)
	pipe.SAdd(ctx, m.userSessionsKey(userID), sid)
	pipe.Expire(ctx, m.userSessionsKey(userID), m.refreshTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	return &TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        "Bearer",
		ExpiresIn:        int(time.Until(accessExp).Seconds()),
		ExpiresAt:        accessExp,
		RefreshExpiresAt: sess.ExpiresAt,
		SessionID:        sid,
	}, nil
}

// RolesFunc 由调用方注入，用于在刷新令牌时重新解析用户角色（角色数据源在 Casbin）。
type RolesFunc func(ctx context.Context, userID string) ([]string, error)

// Rotate 使用刷新令牌换取新的令牌对（刷新令牌一次性，旧令牌立即作废）。
//
// rolesFn 可为 nil；传入时会把最新角色写入新的访问令牌，避免角色调整后旧角色长期生效。
func (m *SessionManager) Rotate(ctx context.Context, refreshToken string, rolesFn RolesFunc) (*TokenPair, error) {
	if refreshToken == "" {
		return nil, apperr.ErrRefreshInvalid
	}
	sid, err := m.rdb.Get(ctx, m.refreshKey(refreshToken)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, apperr.ErrRefreshInvalid
	}
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	raw, err := m.rdb.Get(ctx, m.sessionKey(sid)).Bytes()
	if errors.Is(err, redis.Nil) {
		_ = m.rdb.Del(ctx, m.refreshKey(refreshToken)).Err()
		return nil, apperr.ErrRefreshInvalid
	}
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	var sess Session
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	// 密码版本已变化（改密/重置密码）→ 刷新令牌一并失效。
	current, err := m.PasswordVersion(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if current != sess.PasswordVersion {
		_ = m.Revoke(ctx, refreshToken)
		return nil, apperr.ErrRefreshInvalid.WithMessage("凭证已失效，请重新登录")
	}

	newRefresh, err := randomToken(32)
	if err != nil {
		return nil, err
	}

	roles := []string{}
	if rolesFn != nil {
		if roles, err = rolesFn(ctx, sess.UserID); err != nil {
			return nil, err
		}
	}

	accessToken, accessExp, err := m.jwt.Issue(sess.UserID, sess.Email, roles, sess.PasswordVersion, sid)
	if err != nil {
		return nil, err
	}

	sess.RefreshHash = hashToken(newRefresh)
	pipe := m.rdb.TxPipeline()
	pipe.Del(ctx, m.refreshKey(refreshToken))
	pipe.Set(ctx, m.refreshKey(newRefresh), sid, m.refreshTTL)
	pipe.Set(ctx, m.sessionKey(sid), mustJSON(sess), m.refreshTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	return &TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     newRefresh,
		TokenType:        "Bearer",
		ExpiresIn:        int(time.Until(accessExp).Seconds()),
		ExpiresAt:        accessExp,
		RefreshExpiresAt: time.Now().Add(m.refreshTTL),
		SessionID:        sid,
	}, nil
}

// Revoke 注销单个会话（登出）。
func (m *SessionManager) Revoke(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	key := m.refreshKey(refreshToken)
	sid, err := m.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil // 已失效，视为成功
	}
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return m.revokeBySessionID(ctx, sid)
}

// RevokeSession 按会话 ID 注销（用于「登出指定设备」）。
func (m *SessionManager) RevokeSession(ctx context.Context, sid string) error {
	return m.revokeBySessionID(ctx, sid)
}

func (m *SessionManager) revokeBySessionID(ctx context.Context, sid string) error {
	raw, err := m.rdb.Get(ctx, m.sessionKey(sid)).Bytes()
	if err == nil {
		var sess Session
		if json.Unmarshal(raw, &sess) == nil {
			pipe := m.rdb.TxPipeline()
			pipe.Del(ctx, m.sessionKey(sid))
			// 刷新令牌只存哈希，因此按会话中记录的哈希删除。
			pipe.Del(ctx, m.prefix+"refresh:"+sess.RefreshHash)
			pipe.SRem(ctx, m.userSessionsKey(sess.UserID), sid)
			if _, err := pipe.Exec(ctx); err != nil {
				return apperr.ErrInternal.WithCause(err)
			}
			return nil
		}
	} else if !errors.Is(err, redis.Nil) {
		return apperr.ErrInternal.WithCause(err)
	}
	// 会话已过期，仅清理残留。
	_ = m.rdb.Del(ctx, m.sessionKey(sid)).Err()
	return nil
}

// ReissueForSession 为「保留下来的会话」重新签发一对令牌。
//
// 使用场景：用户修改密码后，密码版本递增会让所有既有访问令牌立即失效
// （这是有意的安全设计：改密必须能踢掉其它设备）。但当前设备不应被自己踢下线，
// 因此这里把该会话记录的密码版本同步到最新值，并换发访问令牌 + 刷新令牌。
func (m *SessionManager) ReissueForSession(ctx context.Context, sessionID string, roles []string) (*TokenPair, error) {
	if sessionID == "" {
		return nil, apperr.ErrRefreshInvalid
	}

	raw, err := m.rdb.Get(ctx, m.sessionKey(sessionID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, apperr.ErrRefreshInvalid
	}
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	var sess Session
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	current, err := m.PasswordVersion(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}

	newRefresh, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	if roles == nil {
		roles = []string{}
	}
	accessToken, accessExp, err := m.jwt.Issue(sess.UserID, sess.Email, roles, current, sessionID)
	if err != nil {
		return nil, err
	}

	oldRefreshHash := sess.RefreshHash
	sess.PasswordVersion = current
	sess.RefreshHash = hashToken(newRefresh)
	sess.ExpiresAt = time.Now().Add(m.refreshTTL)

	pipe := m.rdb.TxPipeline()
	if oldRefreshHash != "" {
		pipe.Del(ctx, m.prefix+"refresh:"+oldRefreshHash)
	}
	pipe.Set(ctx, m.prefix+"refresh:"+sess.RefreshHash, sessionID, m.refreshTTL)
	pipe.Set(ctx, m.sessionKey(sessionID), mustJSON(sess), m.refreshTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	return &TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     newRefresh,
		TokenType:        "Bearer",
		ExpiresIn:        int(time.Until(accessExp).Seconds()),
		ExpiresAt:        accessExp,
		RefreshExpiresAt: sess.ExpiresAt,
		SessionID:        sessionID,
	}, nil
}

// RevokeAllExcept 注销该用户的全部会话（keepSID 非空时保留该会话，用于改密后保持当前设备登录）。
func (m *SessionManager) RevokeAllExcept(ctx context.Context, userID string, keepSID string) error {
	sids, err := m.rdb.SMembers(ctx, m.userSessionsKey(userID)).Result()
	if err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	for _, sid := range sids {
		if keepSID != "" && sid == keepSID {
			continue
		}
		if err := m.revokeBySessionID(ctx, sid); err != nil {
			return err
		}
	}
	return nil
}

// Validate 校验访问令牌是否仍然有效（会话存在且密码版本一致）。
func (m *SessionManager) Validate(ctx context.Context, claims *Claims) error {
	userID, err := claims.UserID()
	if err != nil {
		return err
	}

	pipe := m.rdb.Pipeline()
	pwdvCmd := pipe.Get(ctx, m.pwdVersionKey(userID))
	existsCmd := pipe.Exists(ctx, m.sessionKey(claims.SessionID))
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return apperr.ErrInternal.WithCause(err)
	}

	current := 0
	if v, err := pwdvCmd.Int(); err == nil {
		current = v
	} else if !errors.Is(err, redis.Nil) {
		return apperr.ErrInternal.WithCause(err)
	}
	if current != claims.PasswordVersion {
		return apperr.ErrTokenExpired.WithMessage("凭证已失效，请重新登录")
	}
	if existsCmd.Val() == 0 {
		return apperr.ErrTokenExpired.WithMessage("登录状态已失效，请重新登录")
	}
	return nil
}

// PasswordVersion 读取用户的密码版本号（不存在时为 0）。
func (m *SessionManager) PasswordVersion(ctx context.Context, userID string) (int, error) {
	v, err := m.rdb.Get(ctx, m.pwdVersionKey(userID)).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, apperr.ErrInternal.WithCause(err)
	}
	return v, nil
}

// BumpPasswordVersion 递增密码版本，使该用户已签发的访问令牌全部失效。
func (m *SessionManager) BumpPasswordVersion(ctx context.Context, userID string) error {
	if err := m.rdb.Incr(ctx, m.pwdVersionKey(userID)).Err(); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// ListSessions 列出用户当前所有活跃会话。
func (m *SessionManager) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	sids, err := m.rdb.SMembers(ctx, m.userSessionsKey(userID)).Result()
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	sessions := make([]Session, 0, len(sids))
	for _, sid := range sids {
		raw, err := m.rdb.Get(ctx, m.sessionKey(sid)).Bytes()
		if errors.Is(err, redis.Nil) {
			_ = m.rdb.SRem(ctx, m.userSessionsKey(userID), sid).Err()
			continue
		}
		if err != nil {
			return nil, apperr.ErrInternal.WithCause(err)
		}
		var sess Session
		if err := json.Unmarshal(raw, &sess); err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// randomToken 生成 n 字节的 URL 安全随机串。
func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", apperr.ErrInternal.WithCause(fmt.Errorf("生成随机串失败: %w", err))
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashToken 计算令牌的 SHA-256 十六进制摘要，用于在 Redis 中定位而不保存明文。
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
