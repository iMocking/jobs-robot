package auth

import (
	"strings"
	"testing"
)

// TestGeneratePasswordLengthAndCharset 锁定「随机初始密码」的两个硬性要求：
// 长度正确，且只由排除易混淆字符后的字符集组成。
//
// 为什么值得单测：初始管理员密码是运维从启动日志里手工抄写的，
// 一旦字符集被改回包含 0/O 或 1/l/I，就会出现「照着日志输入却登不上」这类
// 极难排查的问题。
func TestGeneratePasswordLengthAndCharset(t *testing.T) {
	const length = 16

	for i := 0; i < 100; i++ {
		password, err := GeneratePassword(length)
		if err != nil {
			t.Fatalf("生成随机密码失败: %v", err)
		}
		if len(password) != length {
			t.Fatalf("密码长度应为 %d，实际 %d（%q）", length, len(password), password)
		}
		for _, r := range password {
			if !strings.ContainsRune(passwordAlphabet, r) {
				t.Fatalf("密码出现字符集之外的字符 %q（密码 %q）", r, password)
			}
		}
		// 易混淆字符必须被排除，否则运维抄写时极易出错。
		for _, bad := range []string{"0", "O", "1", "l", "I"} {
			if strings.Contains(password, bad) {
				t.Fatalf("密码不应包含易混淆字符 %q（密码 %q）", bad, password)
			}
		}
		// 字母与数字都必须出现，否则 ValidatePasswordStrength 会拒绝。
		if !strings.ContainsAny(password, passwordLetters) {
			t.Fatalf("密码必须含字母（密码 %q）", password)
		}
		if !strings.ContainsAny(password, passwordDigits) {
			t.Fatalf("密码必须含数字（密码 %q）", password)
		}
	}
}

// TestGeneratePasswordIsRandom 两次生成不应相同。
func TestGeneratePasswordIsRandom(t *testing.T) {
	first, err := GeneratePassword(24)
	if err != nil {
		t.Fatalf("生成随机密码失败: %v", err)
	}
	second, err := GeneratePassword(24)
	if err != nil {
		t.Fatalf("生成随机密码失败: %v", err)
	}
	if first == second {
		t.Fatalf("两次生成的密码不应相同：%q", first)
	}
}

// TestGeneratePasswordRejectsTooShortLength 长度不足时必须报错。
//
// 少于 2 位就不可能同时容纳字母与数字，与其生成一个过不了强度校验的密码，
// 不如在入口直接拒绝（空串/短串会破坏调用方的假设）。
func TestGeneratePasswordRejectsTooShortLength(t *testing.T) {
	for _, length := range []int{-1, 0, 1} {
		if _, err := GeneratePassword(length); err == nil {
			t.Errorf("长度 %d 应返回错误", length)
		}
	}
}

// TestGeneratePasswordPassesStrengthCheck 生成的密码必须直接通过强度校验。
//
// 这是自举流程的关键约束：随机密码若过不了 ValidatePasswordStrength，
// 全新部署就会在启动时直接失败——恰恰是最不该失败的场景。
// 因此对每个长度都反复抽样，确保「含字母 + 含数字」是构造性保证而非概率事件。
func TestGeneratePasswordPassesStrengthCheck(t *testing.T) {
	for _, length := range []int{2, 8, 12, 16, 23} {
		for i := 0; i < 200; i++ {
			password, err := GeneratePassword(length)
			if err != nil {
				t.Fatalf("生成长度 %d 的随机密码失败: %v", length, err)
			}
			if err := ValidatePasswordStrength(password, 8); err != nil {
				// 长度 2 时唯一的失败原因是低于最小长度 8，这是符合预期的。
				if length >= 8 {
					t.Fatalf("随机生成的密码 %q 应通过强度校验，实际报错: %v", password, err)
				}
			}
			if len(password) != length {
				t.Fatalf("长度应为 %d，实际 %d（%q）", length, len(password), password)
			}
		}
	}
}
