package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/response"
	"github.com/jsong/jobs-robot/internal/server/middleware"
	"github.com/jsong/jobs-robot/internal/service"
)

// —— 图片服务设置 ——
//
// 与邮件设置同款的两层模型：configs/config.yaml 的 image 段为基线，可被数据库
// storage.image 覆盖；后台在线修改后立即热生效（下次上传即用新配置）。
// 安全约定：
//   - 响应中的 OSS AccessKeySecret 永远是占位符（service.ImageSecretMask），明文不出服务端；
//   - 提交时密钥留空或原样回传占位符 = 不修改已保存的密钥；
//   - 权限仅限管理员（/api/v1/* 通配策略）。

type imageConfigRequest struct {
	Driver        string `json:"driver" binding:"required,oneof=local oss"`
	LocalDir      string `json:"local_dir"`
	PublicBaseURL string `json:"public_base_url"`

	Endpoint    string `json:"oss_endpoint"`
	Bucket      string `json:"oss_bucket"`
	AccessKeyID string `json:"oss_access_key_id"`
	// Secret 为空或等于占位符时表示沿用已保存的密钥。
	Secret      string `json:"secret"`
	ClearSecret bool   `json:"clear_secret"`
}

// AdminGetImageConfig 读取当前生效的图片服务配置。
//
// GET /api/v1/admin/image-storage
func (h *Handler) AdminGetImageConfig(c *gin.Context) {
	view, err := h.svc.Image.Get(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminSaveImageConfig 保存在线修改的图片服务配置，并立即热生效。
//
// PUT /api/v1/admin/image-storage
func (h *Handler) AdminSaveImageConfig(c *gin.Context) {
	var req imageConfigRequest
	if err := bindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}

	view, err := h.svc.Image.Save(c.Request.Context(), service.ImageConfigInput{
		Driver:        req.Driver,
		LocalDir:      req.LocalDir,
		PublicBaseURL: req.PublicBaseURL,
		Endpoint:      req.Endpoint,
		Bucket:        req.Bucket,
		AccessKeyID:   req.AccessKeyID,
		Secret:        req.Secret,
		ClearSecret:   req.ClearSecret,
	}, h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// AdminRevertImageConfig 删除后台设置，回落到配置文件中的图片存储配置。
//
// DELETE /api/v1/admin/image-storage
func (h *Handler) AdminRevertImageConfig(c *gin.Context) {
	view, err := h.svc.Image.Revert(c.Request.Context(), h.actor(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// UploadAvatar 上传当前用户头像：multipart 表单字段 file。
//
// POST /api/v1/me/avatar
//
// 限制：
//   - 仅 image/jpeg|png|gif|webp（按文件头嗅探，不信任扩展名）；
//   - 单文件不超过 storage.MaxImageBytes（10MB）；
//   - 系统机器身份（kind=system）不允许上传。
func (h *Handler) UploadAvatar(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, apperr.ErrUnauthorized)
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, apperr.ErrInvalidParams.WithMessage("请选择要上传的图片文件"))
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		response.Fail(c, apperr.ErrInternal.WithCause(err))
		return
	}
	defer file.Close()

	view, err := h.svc.Image.UploadAvatar(c.Request.Context(), userID, file, fileHeader.Size)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// Files 公开只读地服务本地图片存储目录。
//
// GET <api_prefix>/files/*filepath
//
// 只有本地驱动（driver=local）有文件可服务；OSS 模式由 OSS 提供公网访问，
// 本路由直接返回 404。路径在 storage.LocalStore 写入时已做穿越防御，
// http.FileServer 的 CleanPath 再兜一层，只暴露存储根目录内的文件。
func (h *Handler) Files(c *gin.Context) {
	effective, _, err := service.ResolveImageConfig(c.Request.Context(), h.svc.Store, h.svc.Cfg)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if effective.Driver != "local" || strings.TrimSpace(effective.LocalDir) == "" {
		response.Fail(c, apperr.ErrNotFound)
		return
	}

	http.StripPrefix("/api/v1/files", http.FileServer(http.Dir(effective.LocalDir))).ServeHTTP(c.Writer, c.Request)
}
