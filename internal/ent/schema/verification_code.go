package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"

	"github.com/jsong/jobs-robot/internal/ent/schema/mixins"
	"github.com/jsong/jobs-robot/internal/pkg/ulidid"
)

// VerificationCode 是邮箱验证码的发放与消费记录（用于审计与风控统计）。
//
// 说明：验证码的权威校验放在 Redis 中（带 TTL 与失败次数计数），
// 本表只做留痕，便于排查「验证码是否发出/被谁消费」。
// 表中只保存验证码的哈希值，不保存明文。
type VerificationCode struct {
	ent.Schema
}

// Mixin 提供 ULID 主键与 created_at / updated_at。
func (VerificationCode) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.ULID{}, mixin.Time{}}
}

// Fields 定义验证码字段。
func (VerificationCode) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			MaxLen(255).
			NotEmpty(),
		field.Enum("purpose").
			Values("register", "reset_password", "change_email", "login").
			Default("register"),
		field.String("code_hash").
			MaxLen(128).
			NotEmpty().
			Comment("验证码哈希（HMAC-SHA256），不存明文"),
		field.String("user_id").
			MaxLen(ulidid.Length).
			Optional().
			Nillable(),
		field.Time("expires_at"),
		field.Time("used_at").
			Optional().
			Nillable(),
		field.Int("attempts").
			Default(0),
		field.Int("max_attempts").
			Default(5),
		field.String("ip").
			MaxLen(64).
			Default(""),
		field.String("user_agent").
			MaxLen(512).
			Default(""),
		field.String("delivery").
			MaxLen(32).
			Default("console").
			Comment("投递通道：console/smtp"),
	}
}

// Edges 定义验证码关联。
func (VerificationCode) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("verification_codes").
			Field("user_id").
			Unique(),
	}
}

// Indexes 定义验证码索引。
func (VerificationCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email", "purpose"),
	}
}
