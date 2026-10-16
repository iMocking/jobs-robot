// Package ulidid 提供系统统一的字符串主键：ULID。
//
// 为什么用 ULID 而不是自增整数或 UUIDv4：
//   - **不暴露业务规模**：自增 id 会把「你是第几个用户/第几条岗位」写在 URL 里，
//     既泄露增长数据，也让遍历接口变得容易；
//   - **可按时间排序**：ULID 前 48 位是毫秒时间戳，字典序即时间序，
//     因此它既能做主键，也能直接用于游标分页与「某时间之后创建」的范围查询；
//   - **写入友好**：相比 UUIDv4 的完全随机，ULID 的时间前缀让 B-tree 索引
//     保持局部顺序，插入不会把索引页打散；
//   - **无中心依赖**：本地生成，不需要数据库序列或额外服务。
//
// 生成的字符串是 26 位 Crockford Base32（大写，不含 I/L/O/U），长度固定，
// 因此可以直接用 VARCHAR(26) 存储并安全地做字符串比较。
package ulidid

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Length 是 ULID 字符串的固定长度。
const Length = 26

var (
	// entropy 提供单调递增的随机位，保证「同一毫秒内生成的多个 ID 仍然有序」。
	// ulid.Monotonic 返回的 Reader 不是并发安全的，因此下面用互斥锁包住调用。
	entropyMu sync.Mutex
	entropy   = ulid.Monotonic(cryptorand.Reader, 0)
)

// New 生成一个新的 ULID 字符串。
//
// 并发安全；在极端情况下（系统时间超出 ULID 可表示范围、熵源异常）也不会 panic，
// 而是退化为「时间戳前缀 + 随机后缀」的等长字符串——主键生成失败不该让业务写入崩掉。
func New() string {
	entropyMu.Lock()
	defer entropyMu.Unlock()

	id, err := ulid.New(ulid.Timestamp(time.Now()), entropy)
	if err == nil {
		return id.String()
	}
	return fallback()
}

// Valid 判断字符串是否为合法的 ULID。
//
// 用于校验来自路径参数或请求体的 ID，避免把任意字符串带进数据库查询。
func Valid(s string) bool {
	if len(s) != Length {
		return false
	}
	_, err := ulid.ParseStrict(strings.ToUpper(s))
	return err == nil
}

// Time 返回 ULID 中内嵌的生成时间。
func Time(s string) (time.Time, error) {
	id, err := ulid.ParseStrict(strings.ToUpper(s))
	if err != nil {
		return time.Time{}, err
	}
	return ulid.Time(id.Time()), nil
}

// fallback 在 ULID 生成失败时给出等长的降级值。
//
// 布局与 ULID 保持一致（时间戳 + 随机位），因此长度与可排序性不变。
func fallback() string {
	var buf [16]byte
	binary.BigEndian.PutUint64(buf[0:8], uint64(time.Now().UnixMilli()))
	if _, err := cryptorand.Read(buf[8:]); err != nil {
		binary.BigEndian.PutUint64(buf[8:], uint64(time.Now().UnixNano()))
	}
	return ulid.ULID(buf).String()
}
