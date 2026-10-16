package auth

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
	"github.com/jsong/jobs-robot/internal/pkg/ulidid"
)

// 这些测试分成两类：
//   - 纯函数测试（密码、令牌、验证码摘要）：任何环境都能跑；
//   - Redis 集成测试（验证码发放与校验）：本机 Redis 不可用时自动跳过，
//     而不是伪装通过。

func testJWTConfig() config.JWT {
	return config.JWT{
		Secret:     strings.Repeat("k", 48),
		Issuer:     "jobs-robot-test",
		AccessTTL:  2 * time.Minute,
		RefreshTTL: 24 * time.Hour,
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("Passw0rdStr0ng", 12)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	if hash == "Passw0rdStr0ng" {
		t.Fatal("密码哈希不应等于明文")
	}
	if !VerifyPassword(hash, "Passw0rdStr0ng") {
		t.Error("正确密码应通过校验")
	}
	if VerifyPassword(hash, "Passw0rdStr0ngX") {
		t.Error("错误密码不应通过校验")
	}
	if VerifyPassword("", "Passw0rdStr0ng") {
		t.Error("空哈希不应通过校验（避免历史脏数据被当成合法账号）")
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"合规密码", "Passw0rdStr0ng", false},
		{"过短", "Pw0rd", true},
		{"缺少数字", "PasswordOnly", true},
		{"缺少字母", "1234567890", true},
		{"常见弱口令", "password1", true},
		{"首尾空格", " Passw0rdStr0ng ", true},
		// bcrypt 只处理前 72 字节，超长密码必须被拒绝而不是被静默截断。
		{"超过 72 字节", strings.Repeat("a", 72) + "1", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePasswordStrength(tc.password, 8)
			if tc.wantErr && err == nil {
				t.Errorf("密码 %q 应被拒绝，但通过了校验", tc.password)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("密码 %q 应通过校验，却报错: %v", tc.password, err)
			}
			if tc.wantErr && err != nil {
				if _, ok := apperr.As(err); !ok {
					t.Errorf("错误应为业务错误，实际为 %T", err)
				}
			}
		})
	}
}

func TestIssueAndParseAccessToken(t *testing.T) {
	manager := NewJWTManager(testJWTConfig())

	userID := ulidid.New()
	token, expiresAt, err := manager.Issue(userID, "user@example.com", []string{"reviewer"}, 3, "session-1")
	if err != nil {
		t.Fatalf("签发令牌失败: %v", err)
	}
	if time.Until(expiresAt) <= 0 {
		t.Fatal("令牌过期时间应在未来")
	}

	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("解析令牌失败: %v", err)
	}
	if id, err := claims.UserID(); err != nil || id != userID {
		t.Errorf("用户 ID 解析错误: id=%s err=%v", id, err)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("邮箱不一致: %s", claims.Email)
	}
	if claims.SessionID != "session-1" {
		t.Errorf("会话 ID 不一致: %s", claims.SessionID)
	}
	if claims.PasswordVersion != 3 {
		t.Errorf("密码版本不一致: %d", claims.PasswordVersion)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "reviewer" {
		t.Errorf("角色不一致: %v", claims.Roles)
	}
}

func TestParseRejectsTamperedAndForeignToken(t *testing.T) {
	manager := NewJWTManager(testJWTConfig())
	other := NewJWTManager(config.JWT{
		Secret:    strings.Repeat("z", 48),
		Issuer:    "jobs-robot-test",
		AccessTTL: time.Minute,
	})

	token, _, err := manager.Issue(ulidid.New(), "a@example.com", nil, 0, "sid")
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}

	// 篡改签名
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("令牌格式异常: %s", token)
	}
	tampered := parts[0] + "." + parts[1] + "." + "AAAA" + parts[2][4:]
	if _, err := manager.Parse(tampered); err == nil {
		t.Error("签名被篡改的令牌不应解析成功")
	}

	// 其他密钥签发的令牌
	foreign, _, err := other.Issue(ulidid.New(), "a@example.com", nil, 0, "sid")
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	if _, err := manager.Parse(foreign); err == nil {
		t.Error("其他密钥签发的令牌不应解析成功")
	}

	// 空令牌
	if _, err := manager.Parse(""); err == nil {
		t.Error("空令牌不应解析成功")
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	cfg := testJWTConfig()
	cfg.AccessTTL = time.Second
	manager := NewJWTManager(cfg)
	// 关闭时钟偏差容忍，否则 30 秒的 leeway 会让刚过期的令牌仍然可用。
	manager.leeway = 0

	token, _, err := manager.Issue(ulidid.New(), "exp@example.com", nil, 0, "sid")
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}

	// 让令牌自然过期。
	time.Sleep(1500 * time.Millisecond)

	_, err = manager.Parse(token)
	if err == nil {
		t.Fatal("过期令牌不应解析成功")
	}
	if !apperr.Is(err, apperr.CodeTokenExpired) {
		t.Errorf("过期令牌应返回 ErrTokenExpired，实际: %v", err)
	}
}

func TestParseAcceptsTokenWithinLeeway(t *testing.T) {
	cfg := testJWTConfig()
	cfg.AccessTTL = time.Second
	manager := NewJWTManager(cfg) // 保留默认 30 秒 leeway

	token, _, err := manager.Issue(ulidid.New(), "leeway@example.com", nil, 0, "sid")
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)

	// 多实例部署时钟存在微小差异时，刚过期的令牌仍应被接受，避免频繁掉线。
	if _, err := manager.Parse(token); err != nil {
		t.Errorf("在 leeway 窗口内应接受略过期的令牌，实际: %v", err)
	}
}

func TestCodeHashBindsEmailAndPurpose(t *testing.T) {
	manager := NewCodeManager(nil, CodeOptions{Secret: []byte("test-secret"), Length: 6})

	base := manager.Hash("User@Example.com", CodePurposeRegister, "123456")
	if base != manager.Hash("user@example.com", CodePurposeRegister, "123456") {
		t.Error("邮箱大小写归一化后摘要应一致")
	}
	if base == manager.Hash("other@example.com", CodePurposeRegister, "123456") {
		t.Error("不同邮箱的摘要必须不同（防止跨账号复用）")
	}
	if base == manager.Hash("user@example.com", CodePurposeResetPassword, "123456") {
		t.Error("不同用途的摘要必须不同")
	}
	if base == manager.Hash("user@example.com", CodePurposeRegister, "654321") {
		t.Error("不同验证码的摘要必须不同")
	}
	if strings.Contains(base, "123456") {
		t.Error("摘要中不应出现明文验证码")
	}
}

func TestRandomDigitsLengthAndCharset(t *testing.T) {
	for i := 0; i < 20; i++ {
		code, err := randomDigits(6)
		if err != nil {
			t.Fatalf("生成验证码失败: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("验证码长度应为 6，实际 %d", len(code))
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("验证码应只包含数字，实际 %q", code)
			}
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  User@Example.COM  "); got != "user@example.com" {
		t.Errorf("邮箱归一化结果错误: %q", got)
	}
}

// —— Redis 集成测试 ——

// testRedis 返回可用的 Redis 客户端；不可用时跳过测试。
func testRedis(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("JOBSROBOT_TEST_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:56379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skipf("本机 Redis(%s) 不可用，跳过集成测试: %v", addr, err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestCodeIssueAndVerify(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	prefix := "jobsrobot:test:" + time.Now().Format("150405.000000") + ":"

	manager := NewCodeManager(rdb, CodeOptions{
		KeyPrefix:   prefix,
		Secret:      []byte("test-secret"),
		Length:      6,
		TTL:         2 * time.Minute,
		Cooldown:    time.Minute,
		MaxAttempts: 3,
		DailyLimit:  5,
	})
	t.Cleanup(func() {
		keys, _ := rdb.Keys(ctx, prefix+"*").Result()
		if len(keys) > 0 {
			_ = rdb.Del(ctx, keys...).Err()
		}
	})

	email := "CodeTest@Example.com"
	issued, err := manager.Issue(ctx, email, CodePurposeRegister)
	if err != nil {
		t.Fatalf("发放验证码失败: %v", err)
	}
	if len(issued.Code) != 6 {
		t.Fatalf("验证码长度异常: %q", issued.Code)
	}

	// 冷却期内重复发放应被拒绝
	if _, err := manager.Issue(ctx, email, CodePurposeRegister); !apperr.Is(err, apperr.CodeCodeTooFrequent) {
		t.Errorf("冷却期内重复发放应返回 ErrCodeTooFrequent，实际: %v", err)
	}

	// 错误验证码：累计尝试次数
	for i := 0; i < 2; i++ {
		if err := manager.Verify(ctx, email, CodePurposeRegister, "000000"); !apperr.Is(err, apperr.CodeCodeInvalid) {
			t.Errorf("错误验证码应返回 ErrCodeInvalid，实际: %v", err)
		}
	}

	// 使用正确验证码仍应成功（尚未达到尝试上限）
	if err := manager.Verify(ctx, email, CodePurposeRegister, issued.Code); err != nil {
		t.Fatalf("正确验证码应通过校验: %v", err)
	}

	// 验证码一次性：再次校验必须失败
	if err := manager.Verify(ctx, email, CodePurposeRegister, issued.Code); err == nil {
		t.Error("验证码消费后不应再次通过校验")
	}
}

func TestCodeAttemptsExhausted(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	prefix := "jobsrobot:test:" + time.Now().Format("150405.000000") + ":"

	manager := NewCodeManager(rdb, CodeOptions{
		KeyPrefix:   prefix,
		Secret:      []byte("test-secret"),
		Length:      6,
		TTL:         time.Minute,
		Cooldown:    time.Second,
		MaxAttempts: 2,
		DailyLimit:  5,
	})
	t.Cleanup(func() {
		keys, _ := rdb.Keys(ctx, prefix+"*").Result()
		if len(keys) > 0 {
			_ = rdb.Del(ctx, keys...).Err()
		}
	})

	email := "exhaust@example.com"
	issued, err := manager.Issue(ctx, email, CodePurposeResetPassword)
	if err != nil {
		t.Fatalf("发放验证码失败: %v", err)
	}

	// 达到尝试上限后，验证码应被作废，即使之后输入正确也不再通过。
	for i := 0; i < 2; i++ {
		_ = manager.Verify(ctx, email, CodePurposeResetPassword, "111111")
	}
	if err := manager.Verify(ctx, email, CodePurposeResetPassword, issued.Code); err == nil {
		t.Error("达到尝试上限后验证码应已作废")
	}
}
