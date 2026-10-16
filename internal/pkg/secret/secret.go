// Package secret 提供用于「加密存储敏感配置」的对称加解密能力。
//
// 使用场景：管理员在后台填写 SMTP 密码等凭据后需要落库保存，
// 但数据库中不应出现明文口令。这里采用 AES-256-GCM：
//   - GCM 自带完整性校验，密文被篡改会直接解密失败，而不是解出乱码；
//   - 每次加密使用独立随机 nonce，相同明文两次加密结果不同。
//
// 密钥从主密钥（JWT secret）派生，因此更换主密钥会导致已保存的凭据失效，
// 需要在后台重新填写——这是刻意的取舍：宁可要求重填，也不把凭据明文落库。
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// DerivedKeySize 是 AES-256 要求的密钥长度。
const DerivedKeySize = 32

// ErrInvalidCiphertext 表示密文格式不合法或完整性校验失败。
var ErrInvalidCiphertext = errors.New("密文无效或已被篡改")

// DeriveKey 从主密钥派生指定用途的 32 字节子密钥。
//
// 用途隔离（purpose）保证不同功能不会共用同一把密钥材料：
// 即便某处密文泄漏，也无法直接用于解密别处的数据。
func DeriveKey(master, purpose string) []byte {
	sum := sha256.Sum256([]byte(master + "#" + purpose))
	return sum[:]
}

// Encrypt 加密明文，返回 base64(nonce || ciphertext)。
//
// 空明文返回空串：表示「未设置」，与「设置为空字符串」区分开，
// 避免把空密码当成有效凭据写进配置。
func Encrypt(key []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if len(key) != DerivedKeySize {
		return "", fmt.Errorf("密钥长度必须为 %d 字节，当前 %d", DerivedKeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}

	// 把 nonce 前置到密文，解密时按同样长度切分。
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密 Encrypt 的输出。
func Decrypt(key []byte, encoded string) (string, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return "", nil
	}
	if len(key) != DerivedKeySize {
		return "", fmt.Errorf("密钥长度必须为 %d 字节，当前 %d", DerivedKeySize, len(key))
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", ErrInvalidCiphertext
	}

	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// 不把底层错误细节暴露出去：它同样意味着「密钥不对或密文被改」。
		return "", ErrInvalidCiphertext
	}
	return string(plaintext), nil
}
