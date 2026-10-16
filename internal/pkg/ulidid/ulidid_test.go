package ulidid

import (
	"strings"
	"testing"
	"time"
)

// TestNewLength 锁定「主键长度固定」这一契约。
//
// 长度固定是 VARCHAR(26) 存储与字符串比较的前提，一旦被破坏，
// 数据库层面会出现截断或写入失败，因此这里逐值断言。
func TestNewLength(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := New()
		if len(id) != Length {
			t.Fatalf("ULID 长度应为 %d，实际 %d（值 %q）", Length, len(id), id)
		}
		if len(id) != 26 {
			t.Fatalf("Length 常量与 ULID 规范（26 位）不一致：%d", Length)
		}
	}
}

// TestValid 验证合法值放行、空串与含非法字符的值被拒绝。
func TestValid(t *testing.T) {
	valid := New()
	if !Valid(valid) {
		t.Errorf("自发生成的 ULID 应为合法值：%q", valid)
	}
	// 小写形态同样合法：Valid 内部会先归一化为大写。
	if !Valid(strings.ToLower(valid)) {
		t.Errorf("小写 ULID 也应被接受：%q", strings.ToLower(valid))
	}
	// 文档（docs/api.md 1.5 节）与冒烟脚本中出现的示例值必须是合法 ULID，
	// 否则照抄示例调用接口会直接拿到 10001 参数错误。
	for _, sample := range []string{
		"01JD7Q9K2M3N4P5R6S7T8V9W0X",
		"01JD7Q9K2M3N4P5R6S7T8V9W0Y",
		"01JD7Q9K2M3N4P5R6S7T8V9W0Z",
	} {
		if !Valid(sample) {
			t.Errorf("文档/脚本中的示例 ULID 必须合法：%q", sample)
		}
	}

	invalid := []struct {
		name string
		val  string
	}{
		{"空串", ""},
		{"长度不足", "01JD7Q9K2M3N4P5R6S7T8V9W0"},
		{"长度超长", "01JD7Q9K2M3N4P5R6S7T8V9W0XY"},
		{"含连字符", "01JD7Q9K2M3N4P5R6S7T8V9W-X"},
		{"含空格", "01JD7Q9K2M3N4P5R6S7T8V9W X"},
		{"含非法 Base32 字符 I", "01JD7Q9K2M3N4P5R6S7T8V9WIX"},
		{"含非法 Base32 字符 U", "01JD7Q9K2M3N4P5R6S7T8V9WUX"},
		{"全非 Base32", "!!!!!!!!!!!!!!!!!!!!!!!!!!"},
		{"纯数字", "1234567890"},
	}
	for _, tc := range invalid {
		if Valid(tc.val) {
			t.Errorf("%s 不应被判定为合法 ULID：%q", tc.name, tc.val)
		}
	}
}

// TestNewIsMonotonic 验证同一毫秒内连续生成的多个 ULID 严格递增。
//
// ULID 前 48 位是毫秒时间戳，字典序即时间序；配合单调熵源，
// 即使在同一毫秒内连续生成，后一个也必须大于前一个——
// 这是「按 id 倒序分页不重不漏」与「游标分页」能成立的基础。
func TestNewIsMonotonic(t *testing.T) {
	const n = 2000
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, New())
	}

	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("ULID 必须严格递增：第 %d 个 %q 不大于前一个 %q", i, ids[i], ids[i-1])
		}
	}
}

// TestTime 验证能够取回 ULID 内嵌的生成时间。
func TestTime(t *testing.T) {
	before := time.Now().Add(-time.Second)
	id := New()
	after := time.Now().Add(time.Second)

	got, err := Time(id)
	if err != nil {
		t.Fatalf("解析 ULID 时间失败: %v", err)
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("ULID 内嵌时间应接近当前时间：got=%s before=%s after=%s",
			got.Format(time.RFC3339Nano), before.Format(time.RFC3339Nano), after.Format(time.RFC3339Nano))
	}
}

// TestTimeRejectsInvalid 验证非法字符串无法解析出时间。
func TestTimeRejectsInvalid(t *testing.T) {
	if _, err := Time("not-a-ulid"); err == nil {
		t.Error("非法 ULID 不应解析出时间")
	}
	if _, err := Time(""); err == nil {
		t.Error("空串不应解析出时间")
	}
}
