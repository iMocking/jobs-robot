package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalStore 把图片写入本地磁盘目录。
//
// baseURL 决定返回 URL 的前缀：
//   - 为空时返回相对路径 <api_prefix>/files/<key>（同源部署，img 标签可直接用）；
//   - 配置了 public_base_url 时返回 <public_base_url>/<key>（网关/CDN/独立域名）。
type LocalStore struct {
	dir     string // 绝对路径的存储根目录
	baseURL string // 访问前缀（可能为空）
}

// NewLocal 构造本地存储；目录不存在时自动创建。
func NewLocal(dir, baseURL string) (*LocalStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("本地图片存储目录不能为空")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("解析本地图片存储目录失败: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("创建本地图片存储目录失败: %w", err)
	}
	return &LocalStore{dir: abs, baseURL: strings.TrimSuffix(strings.TrimSpace(baseURL), "/")}, nil
}

// Name 实现 ImageStore。
func (s *LocalStore) Name() string { return "local" }

// Save 实现 ImageStore：写入本地目录并返回可访问 URL。
func (s *LocalStore) Save(_ context.Context, key string, r io.Reader, _ string, size int64) (string, error) {
	rel := SafeKey(key)
	if rel == "" {
		return "", fmt.Errorf("图片存储路径不合法")
	}

	// 路径穿越防御：目标文件必须落在存储根目录内。
	target := filepath.Join(s.dir, filepath.FromSlash(rel))
	inside, err := filepath.Rel(s.dir, target)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("图片存储路径越界，已拒绝")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("创建图片存储子目录失败: %w", err)
	}

	// 先写同目录临时文件再改名：避免半截文件被 /files 路由读走。
	tmp, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	written, err := io.Copy(tmp, r)
	if err != nil {
		tmp.Close()
		return "", fmt.Errorf("写入图片文件失败: %w", err)
	}
	if size > 0 && written != size {
		tmp.Close()
		return "", fmt.Errorf("上传文件不完整（期望 %d 字节，实际 %d）", size, written)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return "", fmt.Errorf("保存图片文件失败: %w", err)
	}

	if s.baseURL != "" {
		return s.baseURL + "/" + rel, nil
	}
	return "/api/v1/files/" + rel, nil
}
