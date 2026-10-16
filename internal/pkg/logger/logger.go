// Package logger 基于标准库 log/slog 构建应用日志，避免额外依赖。
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Options 是日志初始化参数。
type Options struct {
	Level  string // debug | info | warn | error
	Format string // json | console
	Output string // stdout | stderr | 文件路径
}

// Init 构建 logger 并同时设置为 slog 默认 logger。
// 返回的 io.Closer 用于释放文件句柄（输出为 stdout/stderr 时为 nil）。
func Init(opts Options) (*slog.Logger, io.Closer, error) {
	writer, closer, err := openOutput(opts.Output)
	if err != nil {
		return nil, nil, err
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     parseLevel(opts.Level),
		AddSource: strings.EqualFold(opts.Level, "debug"),
	}

	var handler slog.Handler
	if strings.EqualFold(opts.Format, "json") {
		handler = slog.NewJSONHandler(writer, handlerOpts)
	} else {
		handler = slog.NewTextHandler(writer, handlerOpts)
	}

	l := slog.New(handler)
	slog.SetDefault(l)
	return l, closer, nil
}

func openOutput(output string) (io.Writer, io.Closer, error) {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "", "stdout":
		return os.Stdout, nil, nil
	case "stderr":
		return os.Stderr, nil, nil
	default:
		if dir := filepath.Dir(output); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, fmt.Errorf("创建日志目录 %s 失败: %w", dir, err)
			}
		}
		f, err := os.OpenFile(output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("打开日志文件 %s 失败: %w", output, err)
		}
		return f, f, nil
	}
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
