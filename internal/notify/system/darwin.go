//go:build darwin

package system

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// available 判断 osascript 是否可用.
func available() bool {
	_, err := exec.LookPath("osascript")
	return err == nil
}

// notifySystem 通过通知中心弹出提示.
func notifySystem(ctx context.Context, title, body string, sound bool) error {
	script := fmt.Sprintf("display notification %s with title %s",
		appleScriptString(body), appleScriptString(title))
	if sound {
		script += ` sound name "Ping"`
	}

	output, err := exec.CommandContext(ctx, "osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("调用 osascript 失败: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// appleScriptString 把文本转成 AppleScript 字符串字面量.
func appleScriptString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			// 丢弃回车, 保留换行.
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
