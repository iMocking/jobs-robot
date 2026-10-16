package casbin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// userSubjectPrefix 让策略表中的主体一眼可辨：g, user:01JD7Q9K2M3N4P5R6S7T8V9W0X, admin。
const userSubjectPrefix = "user:"

// SubjectForUser 把用户 ID（ULID 字符串）转换为 Casbin 主体名。
func SubjectForUser(userID string) string { return userSubjectPrefix + userID }

// ParseSubject 从主体名还原用户 ID。
func ParseSubject(subject string) (string, bool) {
	if !strings.HasPrefix(subject, userSubjectPrefix) {
		return "", false
	}
	id := strings.TrimPrefix(subject, userSubjectPrefix)
	if id == "" {
		return "", false
	}
	return id, true
}

// Authorizer 是授权服务的对外门面。
type Authorizer struct {
	enforcer *casbin.Enforcer
	adapter  *Adapter

	// mu 保护策略写操作：Casbin 的读并发安全，但「读+写」组合需要串行化。
	mu sync.RWMutex
}

// Option 用于定制 Authorizer。
type Option func(*options)

type options struct {
	reloadInterval time.Duration
	seed           bool
}

// WithReloadInterval 设置策略自动重载间隔（多实例部署时让权限变更尽快生效）。
func WithReloadInterval(d time.Duration) Option {
	return func(o *options) { o.reloadInterval = d }
}

// WithoutSeed 跳过默认策略播种（用于测试或完全自定义权限的场景）。
func WithoutSeed() Option {
	return func(o *options) { o.seed = false }
}

// New 构造授权服务：建表 → 载入策略 → 播种默认角色策略。
func New(ctx context.Context, db *sql.DB, opts ...Option) (*Authorizer, error) {
	cfg := &options{reloadInterval: time.Minute, seed: true}
	for _, opt := range opts {
		opt(cfg)
	}

	adapter := NewAdapter(db)
	if err := adapter.EnsureSchema(ctx); err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	m, err := casbinmodel.NewModelFromString(modelText)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(fmt.Errorf("解析 casbin 模型失败: %w", err))
	}

	enforcer, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(fmt.Errorf("初始化 casbin 失败: %w", err))
	}
	enforcer.EnableAutoSave(true)
	enforcer.EnableLog(false)

	a := &Authorizer{enforcer: enforcer, adapter: adapter}
	if cfg.seed {
		if err := a.SeedDefaults(ctx); err != nil {
			return nil, err
		}
	}

	if cfg.reloadInterval > 0 {
		go a.autoReload(ctx, cfg.reloadInterval)
	}

	slog.Info("授权策略已就绪",
		"policies", len(mustPolicies(enforcer)),
		"grouping_rules", len(mustGrouping(enforcer)),
	)
	return a, nil
}

// mustPolicies / mustGrouping 用于日志与统计，取不到时按空处理。
func mustPolicies(e *casbin.Enforcer) [][]string {
	policies, err := e.GetPolicy()
	if err != nil {
		return nil
	}
	return policies
}

func mustGrouping(e *casbin.Enforcer) [][]string {
	rules, err := e.GetGroupingPolicy()
	if err != nil {
		return nil
	}
	return rules
}

// Enforce 判定 subject 能否在 obj 上执行 act。
func (a *Authorizer) Enforce(subject, obj, act string) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	allowed, err := a.enforcer.Enforce(subject, obj, act)
	if err != nil {
		return false, apperr.ErrInternal.WithCause(fmt.Errorf("权限判定失败: %w", err))
	}
	return allowed, nil
}

// EnforceUser 判定某个用户能否访问接口。
func (a *Authorizer) EnforceUser(userID string, obj, act string) (bool, error) {
	return a.Enforce(SubjectForUser(userID), obj, act)
}

// RolesForUser 返回用户的角色列表。
func (a *Authorizer) RolesForUser(userID string) ([]string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	roles, err := a.enforcer.GetRolesForUser(SubjectForUser(userID))
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	if roles == nil {
		roles = []string{}
	}
	return roles, nil
}

// SetRoles 覆盖式设置用户角色（先清空该用户全部 g 规则）。
func (a *Authorizer) SetRoles(userID string, roles []string) error {
	for _, r := range roles {
		if !IsBuiltinRole(r) {
			return apperr.ErrInvalidParams.WithMessage("不支持的角色：" + r)
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	subject := SubjectForUser(userID)
	if _, err := a.enforcer.RemoveFilteredGroupingPolicy(0, subject); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	for _, r := range roles {
		if _, err := a.enforcer.AddGroupingPolicy(subject, r); err != nil {
			return apperr.ErrInternal.WithCause(err)
		}
	}
	return nil
}

// AssignRole 追加一个角色。
func (a *Authorizer) AssignRole(userID string, role string) error {
	if !IsBuiltinRole(role) {
		return apperr.ErrInvalidParams.WithMessage("不支持的角色：" + role)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, err := a.enforcer.AddGroupingPolicy(SubjectForUser(userID), role); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// RemoveRole 移除一个角色。
func (a *Authorizer) RemoveRole(userID string, role string) error {
	if !IsBuiltinRole(role) {
		return apperr.ErrInvalidParams.WithMessage("不支持的角色：" + role)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, err := a.enforcer.RemoveGroupingPolicy(SubjectForUser(userID), role); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// UsersForRole 返回拥有该角色的用户 ID 列表。
func (a *Authorizer) UsersForRole(role string) ([]string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	subjects, err := a.enforcer.GetUsersForRole(role)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	ids := make([]string, 0, len(subjects))
	for _, s := range subjects {
		if id, ok := ParseSubject(s); ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Policies 返回全部 p 规则（sub, obj, act）。
func (a *Authorizer) Policies() ([][]string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	policies, err := a.enforcer.GetPolicy()
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}
	return policies, nil
}

// AddPolicy 新增一条权限策略。
func (a *Authorizer) AddPolicy(role, obj, act string) error {
	if !IsBuiltinRole(role) {
		return apperr.ErrInvalidParams.WithMessage("不支持的角色：" + role)
	}
	if strings.TrimSpace(obj) == "" || strings.TrimSpace(act) == "" {
		return apperr.ErrInvalidParams.WithMessage("资源路径与动作不能为空")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, err := a.enforcer.AddPolicy(role, obj, act); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// RemovePolicy 删除一条权限策略。
func (a *Authorizer) RemovePolicy(role, obj, act string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, err := a.enforcer.RemovePolicy(role, obj, act); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// Reload 从数据库重新载入策略（用于多实例场景下手动同步）。
func (a *Authorizer) Reload() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.enforcer.LoadPolicy(); err != nil {
		return apperr.ErrInternal.WithCause(err)
	}
	return nil
}

// SeedDefaults 播种内置角色与默认策略；已存在的规则不会重复写入。
func (a *Authorizer) SeedDefaults(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, rule := range defaultGroupingRules {
		has, err := a.enforcer.HasGroupingPolicy(rule[0], rule[1])
		if err != nil {
			return apperr.ErrInternal.WithCause(err)
		}
		if has {
			continue
		}
		if _, err := a.enforcer.AddGroupingPolicy(rule[0], rule[1]); err != nil {
			return apperr.ErrInternal.WithCause(err)
		}
	}

	for _, rule := range defaultPolicies {
		has, err := a.enforcer.HasPolicy(rule[0], rule[1], rule[2])
		if err != nil {
			return apperr.ErrInternal.WithCause(err)
		}
		if has {
			continue
		}
		if _, err := a.enforcer.AddPolicy(rule[0], rule[1], rule[2]); err != nil {
			return apperr.ErrInternal.WithCause(err)
		}
	}
	return nil
}

// autoReload 周期性把数据库中的策略同步到内存，使多实例部署下权限变更尽快生效。
func (a *Authorizer) autoReload(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.Reload(); err != nil {
				slog.Warn("重新载入授权策略失败", "error", err)
			}
		}
	}
}

// Stats 返回策略规模，供运维接口展示。
func (a *Authorizer) Stats() map[string]int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return map[string]int{
		"policies":       len(mustPolicies(a.enforcer)),
		"grouping_rules": len(mustGrouping(a.enforcer)),
	}
}

// compile-time 断言：确保适配器满足 Casbin 的持久化接口。
var _ interface {
	LoadPolicy(m casbinmodel.Model) error
} = (*Adapter)(nil)
