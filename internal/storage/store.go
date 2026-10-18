// Package storage 提供用户上传图片（头像等）的存储抽象。
//
// 驱动二选一，由图片服务配置（configs/config.yaml 的 image 段 + 数据库
// storage.image 设置覆盖）决定：
//   - local：写入本地磁盘目录，返回本服务 /files 路由或公网前缀下的可访问 URL；
//   - oss：通过阿里云 OSS 的 PutObject 上传（标准库实现 V1 签名，无外部依赖）。
//
// 安全约定：
//   - 只接受服务端认可的四类图片（jpg/png/gif/webp），按文件头嗅探而不是扩展名；
//   - 文件一律使用调用方生成的无害 key（如 avatars/<uid>/<ulid>.<ext>），
//     本地驱动仍会做路径穿越防御（key 必须落在存储根目录内）；
//   - 上传大小上限由调用方（service 层）把关。
package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

// MaxImageBytes 是单张上传图片的大小上限（10MB）。
const MaxImageBytes = 10 << 20

// ImageStore 保存一张图片并返回可公开访问的 URL。
type ImageStore interface {
	// Name 返回驱动名（local/oss），用于界面展示。
	Name() string
	// Save 写入图片数据。key 是相对路径（如 avatars/<userID>/<ulid>.png），
	// 返回值是可直接用于 <img src> 的完整 URL。
	Save(ctx context.Context, key string, r io.Reader, contentType string, size int64) (string, error)
}

// SniffImage 读取文件头并判断是否为可接受的图片，返回内容类型。
//
// http.DetectContentType 能识别 png/jpeg/gif/webp（也包含其他类型），
// 这里把识别结果收敛到白名单：识别不了或类型不符直接拒绝。
func SniffImage(r io.Reader) (string, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", fmt.Errorf("读取图片内容失败: %w", err)
	}
	buf = buf[:n]

	switch http.DetectContentType(buf) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return http.DetectContentType(buf), nil
	default:
		return "", fmt.Errorf("仅支持 PNG / JPEG / GIF / WebP 图片")
	}
}

// ExtensionFor 返回内容类型对应的文件扩展名。
func ExtensionFor(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

// SafeKey 规范化存储 key：只允许小写字母/数字/斜杠/点，去首尾斜杠，
// 拒绝任何可能逃逸目录的片段（.. 或 \）。调用方拼 key 时用本函数兜底。
func SafeKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, "\\", "/")
	key = path.Clean("/" + key)
	key = strings.TrimPrefix(key, "/")
	return key
}
