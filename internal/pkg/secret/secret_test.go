package secret

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := DeriveKey("master-key-for-test", "mail-smtp")

	cases := []string{
		"smtp-password",
		"含中文的口令",
		"p@ss w0rd!#$%^&*()",
		strings.Repeat("x", 512),
	}
	for _, plain := range cases {
		encoded, err := Encrypt(key, plain)
		if err != nil {
			t.Fatalf("加密失败: %v", err)
		}
		if strings.Contains(encoded, plain) && plain != "" {
			t.Errorf("密文中不应出现明文: %q", encoded)
		}

		got, err := Decrypt(key, encoded)
		if err != nil {
			t.Fatalf("解密失败: %v", err)
		}
		if got != plain {
			t.Errorf("解密结果不一致: got=%q want=%q", got, plain)
		}
	}
}

func TestEncryptEmptyIsEmpty(t *testing.T) {
	key := DeriveKey("master", "mail-smtp")

	encoded, err := Encrypt(key, "")
	if err != nil {
		t.Fatalf("空明文加密不应报错: %v", err)
	}
	if encoded != "" {
		t.Errorf("空明文应返回空串（表示未设置），实际 %q", encoded)
	}

	got, err := Decrypt(key, "")
	if err != nil || got != "" {
		t.Errorf("空密文应解出空串，got=%q err=%v", got, err)
	}
}

func TestEncryptUsesRandomNonce(t *testing.T) {
	key := DeriveKey("master", "mail-smtp")

	first, err := Encrypt(key, "same-password")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	second, err := Encrypt(key, "same-password")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if first == second {
		t.Error("相同明文两次加密结果应不同（随机 nonce），否则可被识别出「密码没变」")
	}
}

func TestDecryptRejectsWrongKeyAndTampering(t *testing.T) {
	key := DeriveKey("master-a", "mail-smtp")
	otherKey := DeriveKey("master-b", "mail-smtp")

	encoded, err := Encrypt(key, "smtp-password")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}

	if _, err := Decrypt(otherKey, encoded); err == nil {
		t.Error("换用其他主密钥派生的密钥不应解密成功")
	}

	// 用途隔离：同一主密钥、不同 purpose 派生出的密钥也不应解得开。
	if _, err := Decrypt(DeriveKey("master-a", "other-purpose"), encoded); err == nil {
		t.Error("不同用途的派生密钥不应解密成功")
	}

	// 篡改密文尾部（GCM 应检出完整性错误）
	tampered := encoded[:len(encoded)-2] + "AA"
	if _, err := Decrypt(key, tampered); err == nil {
		t.Error("被篡改的密文不应解密成功")
	}

	if _, err := Decrypt(key, "not-base64!!!"); err == nil {
		t.Error("非法 base64 不应解密成功")
	}
	if _, err := Decrypt(key, "c2hvcnQ"); err == nil {
		t.Error("长度不足的密文不应解密成功")
	}
}

func TestDeriveKeyIsStableAndIsolated(t *testing.T) {
	a1 := DeriveKey("master", "purpose-a")
	a2 := DeriveKey("master", "purpose-a")
	b := DeriveKey("master", "purpose-b")

	if len(a1) != DerivedKeySize {
		t.Fatalf("派生密钥长度应为 %d，实际 %d", DerivedKeySize, len(a1))
	}
	if string(a1) != string(a2) {
		t.Error("相同主密钥与用途应派生出相同密钥，否则重启后无法解密已存凭据")
	}
	if string(a1) == string(b) {
		t.Error("不同用途应派生出不同密钥")
	}
}
