package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OSSStore 通过阿里云 OSS 的 PutObject 上传图片。
//
// 刻意不引入官方 SDK（保持依赖最小、离线可构建）：OSS V1 签名
// 只用到 HMAC-SHA1 + Base64 + 规范化的请求头，标准库完全够用。
type OSSStore struct {
	endpoint    string // 如 https://oss-cn-hangzhou.aliyuncs.com
	bucket      string
	accessKeyID string
	secret      string
	publicURL   string // 文件公网访问前缀
	client      *http.Client
}

// NewOSS 构造 OSS 存储。endpoint 可不带 scheme，自动补 https://。
//
// publicURL 为空时按 bucket.endpoint 推导（仅适用于经典公网域名；
// 自定义域名 / CDN / 内网 endpoint 必须显式配置 public_url）。
func NewOSS(endpoint, bucket, accessKeyID, secret, publicURL string) (*OSSStore, error) {
	endpoint = strings.TrimSpace(endpoint)
	bucket = strings.TrimSpace(bucket)
	if endpoint == "" || bucket == "" || accessKeyID == "" || secret == "" {
		return nil, fmt.Errorf("OSS 配置不完整：endpoint/bucket/access_key_id/access_key_secret 均不能为空")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	if publicURL = strings.TrimSuffix(strings.TrimSpace(publicURL), "/"); publicURL == "" {
		host := strings.TrimPrefix(endpoint, "https://")
		host = strings.TrimPrefix(host, "http://")
		publicURL = "https://" + bucket + "." + host
	}

	return &OSSStore{
		endpoint:    endpoint,
		bucket:      bucket,
		accessKeyID: accessKeyID,
		secret:      secret,
		publicURL:   publicURL,
		client:      &http.Client{Timeout: 60 * time.Second},
	}, nil
}

// Name 实现 ImageStore。
func (s *OSSStore) Name() string { return "oss" }

// Save 实现 ImageStore：向 OSS 执行签名 PUT，成功后返回公网访问 URL。
func (s *OSSStore) Save(ctx context.Context, key string, r io.Reader, contentType string, size int64) (string, error) {
	object := SafeKey(key)
	if object == "" {
		return "", fmt.Errorf("图片存储路径不合法")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	date := time.Now().UTC().Format(http.TimeFormat)
	// CanonicalizedResource = /<bucket>/<object>；object 无需 URL 编码（允许含 / 与点）。
	canonicalResource := "/" + s.bucket + "/" + object
	stringToSign := "PUT\n\n" + contentType + "\n" + date + "\n" + canonicalResource

	mac := hmac.New(sha1.New, []byte(s.secret))
	mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	url := s.endpoint + "/" + s.bucket + "/" + object
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, r)
	if err != nil {
		return "", fmt.Errorf("构造 OSS 上传请求失败: %w", err)
	}
	req.Header.Set("Date", date)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "OSS "+s.accessKeyID+":"+signature)
	if size > 0 {
		req.ContentLength = size
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传到 OSS 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("OSS 上传失败（HTTP %d）：%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return s.publicURL + "/" + object, nil
}
