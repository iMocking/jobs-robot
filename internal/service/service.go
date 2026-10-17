// Package service 承载业务逻辑，是 HTTP 层与数据层之间的唯一通道。
//
// 分层约定：
//   - handler 只做参数绑定与响应封装，不含业务判断
//   - service 负责事务、状态流转、权限上下文校验与视图组装
//   - ent 查询直接写在 service 内，不额外引入 repository 层（避免过度抽象）
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/casbin"
	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/crawler"
	"github.com/jsong/jobs-robot/internal/mail"
	"github.com/jsong/jobs-robot/internal/store"
)

// Services 聚合全部业务服务与共享依赖。
type Services struct {
	Cfg   *config.Config
	Store *store.Store
	Authz *casbin.Authorizer
	// Mailer 是可热替换的投递器：后台修改邮件配置后立即生效，无需重启。
	Mailer   *mail.Dynamic
	Codes    *auth.CodeManager
	Sessions *auth.SessionManager
	Crawler  *crawler.Crawler

	Auth    *AuthService
	User    *UserService
	Company *CompanyService
	Job     *JobService
	Setting *SettingService
	Crawl   *CrawlService
	Meta    *MetaService
	Audit   *AuditService
	Mail    *MailService
	// Setup 是首次安装向导：建管理员 + 落邮件配置 + 试发测试邮件。
	Setup *SetupService
}

// Params 是构造 Services 的依赖。
type Params struct {
	Cfg      *config.Config
	Store    *store.Store
	Authz    *casbin.Authorizer
	Mailer   *mail.Dynamic
	Codes    *auth.CodeManager
	Sessions *auth.SessionManager
	Crawler  *crawler.Crawler
}

// New 组装服务集合。
//
// 各子服务只持有 *Services，互相通过聚合体访问，
// 因此新增服务时无需调整构造函数签名。
func New(p Params) *Services {
	s := &Services{
		Cfg:      p.Cfg,
		Store:    p.Store,
		Authz:    p.Authz,
		Mailer:   p.Mailer,
		Codes:    p.Codes,
		Sessions: p.Sessions,
		Crawler:  p.Crawler,
	}

	s.Audit = &AuditService{s: s}
	s.Auth = &AuthService{s: s}
	s.User = &UserService{s: s}
	s.Company = &CompanyService{s: s}
	s.Job = &JobService{s: s}
	s.Setting = &SettingService{s: s}
	s.Crawl = &CrawlService{s: s}
	s.Meta = &MetaService{s: s}
	s.Mail = &MailService{s: s}
	s.Setup = &SetupService{s: s}
	return s
}

// —— 共享小工具 ——

// Actor 描述一次操作的操作者，用于审计留痕。
//
// 由 HTTP 层从鉴权上下文与请求元信息填充，service 层只负责记录，不做信任判断。
type Actor struct {
	UserID    string
	Email     string
	IP        string
	UserAgent string
	Method    string
	Path      string
}

// SystemActor 表示由系统自动任务发起的操作（如定时抓取、配置播种）。
//
// 语义保持广泛：它泛指「不属于任何具体机器身份的系统动作」，
// 没有 UserID，因此审计里 actor_id 为空、actor_email 为 "system"。
func SystemActor() Actor {
	return Actor{Email: "system"}
}

// systemActors 保存 bootstrap 时注册的机器身份 Actor（key → Actor）。
//
// 刻意用包级 map 而不是依赖注入：SystemActor() 的既有调用点都是静态构造，
// 改成注入会把签名污染到一大片与身份无关的代码。写操作只在启动阶段发生，
// 之后全部是读，用 RWMutex 足够。
var (
	systemActorsMu sync.RWMutex
	systemActors   = make(map[string]Actor)
)

// RegisterSystemActor 在 bootstrap 时把系统身份注册为可用的 Actor。
//
// 注册后的 Actor 只带 UserID（系统身份没有邮箱），审计因此能通过 users 表
// 解析出「Jobs AI Robot」这样的昵称，而不会写出一个虚假的邮箱。
func RegisterSystemActor(key string, actor Actor) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	systemActorsMu.Lock()
	defer systemActorsMu.Unlock()
	systemActors[key] = actor
}

// SystemActorFor 返回指定系统身份；未注册时返回 SystemActor() 兜底。
//
// 兜底是必要的：单元测试、仅跑服务层的脚本或初始化失败时都可能没有注册，
// 此时宁可按「无归属的系统操作」记录，也不要 panic 或静默丢失审计。
func SystemActorFor(key string) Actor {
	systemActorsMu.RLock()
	actor, ok := systemActors[strings.TrimSpace(key)]
	systemActorsMu.RUnlock()
	if ok && actor.UserID != "" {
		return actor
	}
	return SystemActor()
}

// systemActorID 返回指定系统身份的 ULID；未注册时返回 nil。
//
// 用于需要「可空操作人 ID」的字段（如岗位修订的 operator_id、抓取触发起源）。
func systemActorID(key string) *string {
	actor := SystemActorFor(key)
	if actor.UserID == "" {
		return nil
	}
	id := actor.UserID
	return &id
}

// IsSystem 判断是否为系统操作：没有操作人 ID 且没有具体经办邮箱，即视为系统自动任务。
func (a Actor) IsSystem() bool { return a.UserID == "" && (a.Email == "system" || a.Email == "") }

// normalizeEmail 统一邮箱大小写与空白，保证唯一性判定一致。
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// paginate 归一化分页参数，防止超大 page_size 拖垮数据库。
func paginate(page, pageSize, defaultSize, maxSize int) (limit, offset, normPage, normSize int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultSize
	}
	if maxSize > 0 && pageSize > maxSize {
		pageSize = maxSize
	}
	return pageSize, (page - 1) * pageSize, page, pageSize
}

// hashContent 计算内容指纹，用于判断岗位是否发生变更。
func hashContent(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(strings.TrimSpace(p)))
		h.Write([]byte{0x1f}) // 分隔符，避免不同字段拼接产生歧义
	}
	return hex.EncodeToString(h.Sum(nil))
}

// derefString 安全解引用字符串指针。
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// timeKey 把可空时间转换为内容指纹可用的稳定字符串。
func timeKey(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// truncateRunes 按字符数截断，避免把多字节字符截半。
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// startOfDay 返回某天零点，用于按日统计。
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
