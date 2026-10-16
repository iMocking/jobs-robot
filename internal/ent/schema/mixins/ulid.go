// Package mixins 存放跨实体复用的 ent schema 片段。
package mixins

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	entmixin "entgo.io/ent/schema/mixin"

	"github.com/jsong/jobs-robot/internal/pkg/ulidid"
)

// ULID 为实体提供 ULID 字符串主键，替代 ent 默认的数字自增 id。
//
// 做成 mixin 而不是在每张表里重复声明，是为了让「主键必须是 ULID」
// 成为一条无法遗漏的约定：新增实体只要带上这个 mixin 即可。
type ULID struct{ entmixin.Schema }

// Fields 声明主键字段。
//
// 不使用 Immutable()：ent 只有在字段可变时才会为 Create 构造器生成 SetID，
// 保留它便于需要「指定 ID 导入历史数据」的场景（例如从旧库迁移时保留原标识）。
func (ULID) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(ulidid.Length).
			NotEmpty().
			DefaultFunc(ulidid.New).
			Comment("ULID 主键：26 位 Crockford Base32，按时间单调递增"),
	}
}
