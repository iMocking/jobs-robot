package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// testRedis 返回可用的 Redis 客户端；不可用时跳过测试（不伪装通过）。
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
		t.Skipf("本机 Redis(%s) 不可用，跳过限流集成测试: %v", addr, err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// newTestEngine 构造只挂了限流中间件的最小 gin 引擎。
func newTestEngine(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/codes", handler, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0})
	})
	return engine
}

func doPost(engine *gin.Engine, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/codes", nil)
	req.RemoteAddr = remoteAddr
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

func uniqueKey(t *testing.T) string {
	t.Helper()
	return "ratelimit-test:" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func TestRateLimitBlocksAfterLimitAndSetsHeaders(t *testing.T) {
	rdb := testRedis(t)
	prefix := uniqueKey(t) + ":"
	t.Cleanup(func() {
		keys, _ := rdb.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = rdb.Del(context.Background(), keys...).Err()
		}
	})

	engine := newTestEngine(RateLimit(rdb, prefix, "codes", 2, time.Minute))

	if code := doPost(engine, "10.0.0.1:1234").Code; code != http.StatusOK {
		t.Fatalf("第 1 次请求应放行，实际 %d", code)
	}
	if code := doPost(engine, "10.0.0.1:1234").Code; code != http.StatusOK {
		t.Fatalf("第 2 次请求应放行（限额为 2），实际 %d", code)
	}

	blocked := doPost(engine, "10.0.0.1:1234")
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("第 3 次请求应被限流为 429，实际 %d", blocked.Code)
	}
	if got := blocked.Header().Get("Retry-After"); got == "" {
		t.Error("429 响应应带 Retry-After 头")
	} else if seconds, err := strconv.Atoi(got); err != nil || seconds < 1 {
		t.Errorf("Retry-After 应为不小于 1 的秒数，实际 %q", got)
	}
	if got := blocked.Header().Get("X-RateLimit-Limit"); got != "2" {
		t.Errorf("X-RateLimit-Limit 应为 2，实际 %q", got)
	}
	if got := blocked.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining 应为 0，实际 %q", got)
	}

	// 不同 IP 各自计数，不应被同一个桶拖累。
	if code := doPost(engine, "10.0.0.2:1234").Code; code != http.StatusOK {
		t.Errorf("另一个 IP 应独立计数并放行，实际 %d", code)
	}
}

func TestRateLimitSeparatesByKeyParts(t *testing.T) {
	rdb := testRedis(t)
	prefix := uniqueKey(t) + ":"
	t.Cleanup(func() {
		keys, _ := rdb.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = rdb.Del(context.Background(), keys...).Err()
		}
	})

	// 同一 IP、限额 1，但 keyParts 不同 → 互不影响。
	engine := gin.New()
	engine.POST("/codes", RateLimit(rdb, prefix, "codes", 1, time.Minute), func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.POST("/login", RateLimit(rdb, prefix, "login", 1, time.Minute), func(c *gin.Context) { c.Status(http.StatusOK) })

	post := func(path string) int {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.RemoteAddr = "10.1.1.1:9999"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder.Code
	}

	if code := post("/codes"); code != http.StatusOK {
		t.Fatalf("/codes 首次应放行，实际 %d", code)
	}
	if code := post("/login"); code != http.StatusOK {
		t.Errorf("不同 keyParts 应使用独立计数桶，实际 %d", code)
	}
	if code := post("/codes"); code != http.StatusTooManyRequests {
		t.Errorf("/codes 第二次应被限流，实际 %d", code)
	}
}

func TestRateLimitPassthroughWhenDisabled(t *testing.T) {
	// limit<=0 或 window<=0 时应完全直通，且不访问 Redis（此处传 nil 也不会 panic）。
	for _, tc := range []struct {
		name   string
		limit  int
		window time.Duration
	}{
		{"limit 为 0", 0, time.Minute},
		{"limit 为负", -1, time.Minute},
		{"window 为 0", 10, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newTestEngine(RateLimit(nil, "prefix:", "codes", tc.limit, tc.window))
			for i := 0; i < 5; i++ {
				if code := doPost(engine, "10.2.2.2:1234").Code; code != http.StatusOK {
					t.Fatalf("不限流时第 %d 次请求应放行，实际 %d", i+1, code)
				}
			}
		})
	}
}

func TestRateLimitFailsOpenOnRedisError(t *testing.T) {
	// 指向一个必然连不上的地址：限流组件故障时必须放行，不能拖垮登录注册。
	broken := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 200 * time.Millisecond,
		MaxRetries:  -1,
	})
	t.Cleanup(func() { _ = broken.Close() })

	engine := newTestEngine(RateLimit(broken, "prefix:", "codes", 1, time.Minute))
	for i := 0; i < 3; i++ {
		code := doPost(engine, "10.3.3.3:1234").Code
		if code != http.StatusOK {
			t.Fatalf("Redis 故障时应 fail-open 放行，第 %d 次实际 %d", i+1, code)
		}
	}
}

func TestRateLimitErrorCodeIsTooManyRequests(t *testing.T) {
	rdb := testRedis(t)
	prefix := uniqueKey(t) + ":"
	t.Cleanup(func() {
		keys, _ := rdb.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = rdb.Del(context.Background(), keys...).Err()
		}
	})

	engine := gin.New()
	engine.POST("/codes",
		RateLimit(rdb, prefix, "codes", 1, time.Minute),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/codes", nil)
		req.RemoteAddr = "10.4.4.4:1234"
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder
	}

	_ = post()
	blocked := post()

	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("应返回 429，实际 %d", blocked.Code)
	}
	// 统一信封：业务错误码必须是「请求过于频繁」。
	if body := blocked.Body.String(); !contains(body, strconv.Itoa(int(apperr.CodeTooManyReqs))) {
		t.Errorf("响应体应包含错误码 %d，实际 %s", apperr.CodeTooManyReqs, body)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
