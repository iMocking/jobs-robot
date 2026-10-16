//go:build tools

// Package tools 仅用于把构建期工具（如 ent 代码生成器）声明为模块依赖，
// 保证 `go mod tidy` 能锁定其版本，团队成员与 CI 生成的代码完全一致。
// 该文件带 tools 构建标签，不参与正常编译。
package tools

import (
	_ "entgo.io/ent/cmd/ent"
)
