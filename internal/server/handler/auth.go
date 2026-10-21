package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
	"github.com/jsong/jobs-robot/internal/server/middleware"
	"github.com/jsong/jobs-robot/internal/service"
)

// —— 请求体定义 ——

type sendCodeRequest struct {
	Email   string `json:"email" binding:"required"`
	Purpose string `json:"purpose" binding:"required,oneof=register reset_password change_email login role_change"`
}

type registerRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	Nickname string `json:"nickname"`
	Code     string `json:"code"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type updateProfileRequest struct {
	Nickname  *string `json:"nickname"`
	AvatarURL *string `json:"avatar_url"`
	Phone     *string `json:"phone"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type forgotPasswordRequest struct {
	Email string `json:"email" binding:"required"`
}

type resetPasswordRequest struct {
	Email       string `json:"email" binding:"required"`
	Code        string `json:"code" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// SendCode 发送邮箱验证码（注册/找回密码/换绑邮箱）。
//
// POST /api/v1/auth/codes
func (h *Handler) SendCode(c *gin.Context) {
	var req sendCodeRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	result, err := h.svc.Auth.SendCode(c.Request.Context(), req.Email, auth.CodePurpose(req.Purpose), clientIP(c), c.Request.UserAgent())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Register 邮箱注册。
//
// POST /api/v1/auth/register
func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	result, err := h.svc.Auth.Register(c.Request.Context(), service.RegisterInput{
		Email:     req.Email,
		Password:  req.Password,
		Nickname:  req.Nickname,
		Code:      req.Code,
		IP:        clientIP(c),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, result)
}

// Login 邮箱登录。
//
// POST /api/v1/auth/login
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	result, err := h.svc.Auth.Login(c.Request.Context(), service.LoginInput{
		Email:     req.Email,
		Password:  req.Password,
		IP:        clientIP(c),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Refresh 刷新访问令牌。
//
// POST /api/v1/auth/refresh
func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	pair, err := h.svc.Auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, pair)
}

// Logout 登出当前会话。
//
// POST /api/v1/auth/logout
func (h *Handler) Logout(c *gin.Context) {
	var req logoutRequest
	// 允许空 body：此时仅依赖鉴权上下文。
	_ = c.ShouldBindJSON(&req)

	userID, _ := middleware.UserID(c)
	claims, _ := middleware.Claims(c)
	email := ""
	if claims != nil {
		email = claims.Email
	}

	if err := h.svc.Auth.Logout(c.Request.Context(), req.RefreshToken, userID, email); err != nil {
		response.Fail(c, err)
		return
	}
	response.NoContent(c)
}

// ForgotPassword 发送找回密码验证码。
//
// POST /api/v1/auth/password/forgot
func (h *Handler) ForgotPassword(c *gin.Context) {
	var req forgotPasswordRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	result, err := h.svc.Auth.ForgotPassword(c.Request.Context(), req.Email, clientIP(c), c.Request.UserAgent())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// ResetPassword 通过验证码重置密码。
//
// POST /api/v1/auth/password/reset
func (h *Handler) ResetPassword(c *gin.Context) {
	var req resetPasswordRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	if err := h.svc.Auth.ResetPassword(c.Request.Context(), req.Email, req.Code, req.NewPassword); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"message": "密码已重置，请使用新密码登录"})
}

// Me 返回当前登录用户。
//
// GET /api/v1/me
func (h *Handler) Me(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, apperr.ErrUnauthorized)
		return
	}

	view, err := h.svc.Auth.Me(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// UpdateProfile 更新当前用户资料。
//
// PATCH /api/v1/me
func (h *Handler) UpdateProfile(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, apperr.ErrUnauthorized)
		return
	}

	var req updateProfileRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Auth.UpdateProfile(c.Request.Context(), userID, service.UpdateProfileInput{
		Nickname:  req.Nickname,
		AvatarURL: req.AvatarURL,
		Phone:     req.Phone,
	})
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// ChangePassword 修改当前用户密码。
//
// POST /api/v1/me/password
func (h *Handler) ChangePassword(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, apperr.ErrUnauthorized)
		return
	}

	var req changePasswordRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	// 保留当前会话：改密后不应把自己踢下线。
	keepSession := ""
	if claims, ok := middleware.Claims(c); ok {
		keepSession = claims.SessionID
	}

	pair, err := h.svc.Auth.ChangePassword(c.Request.Context(), userID, req.OldPassword, req.NewPassword, keepSession)
	if err != nil {
		response.Fail(c, err)
		return
	}

	// 返回换发后的令牌：密码版本已递增，客户端必须替换本地令牌，
	// 否则会拿着已失效的旧令牌继续请求。
	response.OK(c, gin.H{
		"message": "密码修改成功，其他设备已退出登录",
		"token":   pair,
	})
}

// Sessions 列出当前用户的活跃会话。
//
// GET /api/v1/me/sessions
func (h *Handler) Sessions(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, apperr.ErrUnauthorized)
		return
	}

	list, err := h.svc.Auth.Sessions(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": list})
}

// RevokeSession 注销指定会话。
//
// DELETE /api/v1/me/sessions/:sid
func (h *Handler) RevokeSession(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, apperr.ErrUnauthorized)
		return
	}
	sid := c.Param("sid")
	if sid == "" {
		response.Fail(c, apperr.ErrInvalidParams.WithMessage("会话 ID 不能为空"))
		return
	}

	if err := h.svc.Auth.RevokeSession(c.Request.Context(), userID, sid); err != nil {
		response.Fail(c, err)
		return
	}
	response.NoContent(c)
}
