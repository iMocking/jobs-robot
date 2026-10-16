package auth

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// bcryptMaxBytes 是 bcrypt 的输入长度上限；超过部分会被静默截断，必须在入口拦截。
const bcryptMaxBytes = 72

// weakPasswords 是明确拒绝的常见弱口令。
var weakPasswords = map[string]struct{}{
	"password":   {},
	"12345678":   {},
	"123456789":  {},
	"1234567890": {},
	"qwertyui":   {},
	"qwerty123":  {},
	"abc123456":  {},
	"admin123":   {},
	"iloveyou":   {},
	"11111111":   {},
	"password1":  {},
	"passw0rd":   {},
	"a1234567":   {},
	"1qaz2wsx":   {},
	"woaini1314": {},
}

// HashPassword 使用 bcrypt 生成密码哈希。
func HashPassword(plain string, cost int) (string, error) {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", apperr.ErrInternal.WithCause(err)
	}
	return string(hashed), nil
}

// VerifyPassword 校验明文密码是否与哈希匹配。
//
// 注意：无论用户是否存在都应调用本函数（对不存在的用户可用假哈希占位），
// 以免通过响应时间差枚举出已注册邮箱。
func VerifyPassword(hash, plain string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// dummyHash 用于「用户不存在」时消耗等量 CPU，抵御时间侧信道攻击。
const dummyHash = "$2a$12$C6UzMDM.H6dfI/f/IKcEe.uQPjEeMBvTTGZ4Y5Xy8oQ0k9nJ7q6Kq"

// VerifyDummy 在用户不存在时执行一次假校验，保持响应时间一致。
func VerifyDummy(plain string) bool {
	return VerifyPassword(dummyHash, plain)
}

// ValidatePasswordStrength 校验密码强度，返回业务错误。
func ValidatePasswordStrength(plain string, minLen int) error {
	var problems []string

	if utf8.RuneCountInString(plain) < minLen {
		problems = append(problems, "长度至少 "+itoa(minLen)+" 位")
	}
	// bcrypt 只处理前 72 字节，超长密码会让「后 72 字节」失去意义，直接拒绝。
	if len(plain) > bcryptMaxBytes {
		problems = append(problems, "长度不得超过 72 字节")
	}
	if plain != strings.TrimSpace(plain) {
		problems = append(problems, "首尾不能包含空白字符")
	}

	var hasLetter, hasDigit bool
	for _, r := range plain {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter {
		problems = append(problems, "需包含至少一个字母")
	}
	if !hasDigit {
		problems = append(problems, "需包含至少一个数字")
	}
	if _, bad := weakPasswords[strings.ToLower(plain)]; bad {
		problems = append(problems, "不能使用常见弱密码")
	}

	if len(problems) > 0 {
		return apperr.ErrPasswordTooWeak.WithDetails(map[string]any{"problems": problems})
	}
	return nil
}

// itoa 避免为一次转换引入 strconv 依赖而重复导入。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
