// Package ent 存放 ent 生成的 ORM 代码与 schema 定义。
//
// 重新生成代码：
//
//	go generate ./internal/ent/...
//
// 或直接执行：
//
//	go run entgo.io/ent/cmd/ent generate ./internal/ent/schema
package ent

//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --target . ./schema
