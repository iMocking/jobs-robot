package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// 随机密码的字符集：剔除 0/O、1/l/I 等易混淆字符。
//
// 初始管理员的密码需要运维从启动日志或凭据文件里「抄写」一次，
// 保留易混淆字符会显著提高抄错、反复重试的概率。
const (
	passwordLetters  = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"
	passwordDigits   = "23456789"
	passwordAlphabet = passwordLetters + passwordDigits
)

// GeneratePassword 生成易于抄写且足够强的随机密码（排除易混淆字符）。
//
// 使用 crypto/rand 而非 math/rand：这里生成的是可直接登录后台的初始凭据，
// 可预测的伪随机序列等同于把管理员账号交出去。
//
// 结果保证同时包含字母与数字（首两位分别取字母与数字后整体洗牌），因此
// 必然通过 ValidatePasswordStrength 的「需包含字母 / 需包含数字」两条规则。
// 这一步不是锦上添花：纯均匀抽样在 16 位长度下有约 3% 的概率抽不到数字，
// 那种情况下启动自举会因为密码强度不足而直接失败——恰恰是最不该失败的场景。
func GeneratePassword(length int) (string, error) {
	if length < 2 {
		return "", fmt.Errorf("密码长度至少为 2（需要同时包含字母与数字），当前为 %d", length)
	}

	var sb strings.Builder
	sb.Grow(length)

	// 先各取一个字母与一个数字，保证强度规则必然满足。
	letter, err := randomChar(passwordLetters)
	if err != nil {
		return "", err
	}
	digit, err := randomChar(passwordDigits)
	if err != nil {
		return "", err
	}
	sb.WriteByte(letter)
	sb.WriteByte(digit)

	for i := 2; i < length; i++ {
		c, err := randomChar(passwordAlphabet)
		if err != nil {
			return "", err
		}
		sb.WriteByte(c)
	}

	// 洗牌，避免「密码第 1 位是字母、第 2 位是数字」成为可预测的固定形态。
	out := []byte(sb.String())
	if err := shuffle(out); err != nil {
		return "", err
	}
	return string(out), nil
}

// randomChar 从字符集中等概率取一个字符。
func randomChar(alphabet string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
	if err != nil {
		return 0, fmt.Errorf("生成随机密码失败: %w", err)
	}
	return alphabet[n.Int64()], nil
}

// shuffle 用 Fisher–Yates 原地洗牌，随机源同样是 crypto/rand。
func shuffle(buf []byte) error {
	for i := len(buf) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return fmt.Errorf("生成随机密码失败: %w", err)
		}
		j := int(n.Int64())
		buf[i], buf[j] = buf[j], buf[i]
	}
	return nil
}
