// Package logx 提供 rainmail 的日志设施, 基于标准库 log/slog.
//
// 约定: 关键阶段与外部交互使用 Info, 细节诊断使用 Debug,
// 异常分支使用 Error 并携带足够的上下文字段.
package logx

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Options 描述日志输出配置.
type Options struct {
	Level  string // debug | info | warn | error, 默认 info
	Format string // text | json, 默认 text
	File   string // 可选, 日志同时追加写入该文件
}

// Setup 构建 logger 并设置为 slog 默认 logger.
//
// 返回的 io.Closer 在日志写入文件时非 nil, 需要调用方在退出前关闭.
func Setup(opts Options) (*slog.Logger, io.Closer, error) {
	writers := []io.Writer{os.Stderr}
	var closer io.Closer

	if opts.File != "" {
		if dir := filepath.Dir(opts.File); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, fmt.Errorf("创建日志目录 %s 失败: %w", dir, err)
			}
		}
		f, err := os.OpenFile(opts.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("打开日志文件 %s 失败: %w", opts.File, err)
		}
		writers = append(writers, f)
		closer = f
	}

	level, err := ParseLevel(opts.Level)
	if err != nil {
		closeQuietly(closer)
		return nil, nil, err
	}

	handlerOpts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.String(slog.TimeKey, t.Format("2006-01-02 15:04:05"))
				}
			}
			return a
		},
	}

	out := io.MultiWriter(writers...)
	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(opts.Format)) {
	case "", "text":
		handler = slog.NewTextHandler(out, handlerOpts)
	case "json":
		handler = slog.NewJSONHandler(out, handlerOpts)
	default:
		closeQuietly(closer)
		return nil, nil, fmt.Errorf("未知日志格式 %q, 可选 text 或 json", opts.Format)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, closer, nil
}

// ParseLevel 把字符串转换为 slog 级别.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug", "trace":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("未知日志级别 %q, 可选 debug / info / warn / error", s)
	}
}

func closeQuietly(c io.Closer) {
	if c != nil {
		_ = c.Close()
	}
}
