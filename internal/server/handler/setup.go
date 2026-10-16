package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
	"github.com/jsong/jobs-robot/internal/service"
)

// 本文件是「首次安装向导」的 HTTP 入口，对应前端 /setup 页面。
//
// 两个接口刻意放在**公开分组**：它们要回答的正是「系统里还没有任何账号可登录」这一场景，
// 因此不可能要求鉴权。安全边界由三层构成，而不是靠中间件：
//  1. 配置开关 auth.setup.enabled（默认开）；
//  2. service 层的「不存在可用管理员」判定 + PostgreSQL 咨询锁串行化
//     （判定与创建之间的竞态只能靠锁解决，见 internal/service/setup.go）；
//  3. 路由级限流 auth.rate_limit.setup_*：未初始化窗口内它是公开接口，
//     不限流就等于把「创建管理员」与「发测试邮件」暴露给任意来源。
//
// 一旦系统完成初始化，前两层会让两个接口都返回 409 且不产生任何副作用。

// setupMailRequest 是安装向导里的邮件配置入参。
//
// 形状与后台的 mailConfigRequest 保持一致，但**没有 clear_password**：
// 初始化阶段不存在「已保存的密码」需要清空；密码留空即表示该 SMTP 不需要认证。
type setupMailRequest struct {
	Driver      string `json:"driver"`
	FromName    string `json:"from_name"`
	FromAddress string `json:"from_address"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	TLS         *bool  `json:"tls"`
}

// toConfig 把请求体转成配置对象并统一裁剪首尾空白。
//
// 返回 nil 表示「本次不配置邮件服务」（对应请求体里 mail 为 null），
// 而不是「配置了一个空 SMTP」——两者在 service 层的语义完全不同。
func (r *setupMailRequest) toConfig() *config.Mail {
	if r == nil {
		return nil
	}
	return &config.Mail{
		Driver:      strings.TrimSpace(r.Driver),
		FromName:    strings.TrimSpace(r.FromName),
		FromAddress: strings.TrimSpace(r.FromAddress),
		SMTP: config.SMTP{
			Host:     strings.TrimSpace(r.Host),
			Port:     r.Port,
			Username: strings.TrimSpace(r.Username),
			// 密码在这里不 TrimSpace：service 层有统一的 trimMailSecret
			// （自动去除首尾空白 + 记一条不含口令的告警），
			// 两条发信路径共用它，避免「有的地方裁、有的地方不裁」。
			Password: r.Password,
			TLS:      r.TLS != nil && *r.TLS,
		},
	}
}

// setupRequest 是 POST /system/setup 的入参。
type setupRequest struct {
	AdminEmail    string `json:"admin_email" binding:"required"`
	AdminPassword string `json:"admin_password" binding:"required"`
	Nickname      string `json:"nickname"`
	// Mail 为 null / 省略表示运维选择「先跳过邮件服务」：只建管理员，不落邮件配置。
	Mail *setupMailRequest `json:"mail"`
}

// setupMailTestRequest 是 POST /system/setup/mail/test 的入参。
//
// Mail 必填：该接口的语义是「用**表单里正在填的**参数试发一封」，不是
// 「用已保存的配置发一封」——后者是后台 /admin/mail/test 的职责。
type setupMailTestRequest struct {
	Mail *setupMailRequest `json:"mail" binding:"required"`
	To   string            `json:"to"`
}

// SystemSetup 执行一次性初始化：创建管理员，可选地落邮件配置。
//
// POST /api/v1/system/setup
func (h *Handler) SystemSetup(c *gin.Context) {
	var req setupRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	result, err := h.svc.Setup.SetupAdmin(c.Request.Context(), service.SetupAdminInput{
		AdminEmail:    req.AdminEmail,
		AdminPassword: req.AdminPassword,
		Nickname:      req.Nickname,
		Mail:          req.Mail.toConfig(),
		// 审计需要知道这次初始化是从哪个 IP / UA 发起的：
		// 此时还没有登录态，h.actor(c) 取不到用户身份，但请求元信息是拿得到的。
		RequestActor: h.actor(c),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	// 201：本次请求**创建**了系统里的第一个管理员（不是幂等重放，重复调用会 409）。
	// 前端 systemApi.setup 不区分 200/201，只看信封里的 data。
	response.Created(c, result)
}

// SetupMailTest 用向导表单里的邮件参数试发一封测试邮件（不落库、不热替换投递器）。
//
// POST /api/v1/system/setup/mail/test
func (h *Handler) SetupMailTest(c *gin.Context) {
	var req setupMailTestRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	mailCfg := req.Mail.toConfig()
	if mailCfg == nil {
		// 理论上 binding:"required" 已经拦住，这里只是不让 nil 解引用变成 500
		response.Fail(c, apperr.ErrInvalidParams.WithMessage("请先填写邮件服务参数"))
		return
	}

	if err := h.svc.Setup.SendSetupTestMail(c.Request.Context(), service.TestMailInput{
		Mail: *mailCfg,
		To:   req.To,
	}); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"message": "测试邮件已发送，请查收（含垃圾箱）"})
}
