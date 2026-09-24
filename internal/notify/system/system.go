// Package system 通过各平台的桌面通知机制提醒用户.
//
// macOS 使用 osascript, Linux 使用 notify-send/zenity/kdialog,
// Windows 使用 PowerShell 调用 WinRT 的 Toast 通知.
package system

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/azazo1/rainmail/internal/notify"
)

// maxBodyRunes 限制通知正文长度, 桌面通知不适合承载长文本.
const maxBodyRunes = 400

// Notifier 是系统通知通道.
type Notifier struct {
	sound  bool
	logger *slog.Logger
}

// New 构造系统通知通道.
func New(sound bool, logger *slog.Logger) *Notifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &Notifier{sound: sound, logger: logger}
}

// Name 返回通道名.
func (n *Notifier) Name() string { return "system" }

// Available 判断当前平台是否具备可用的通知机制.
func (n *Notifier) Available() bool { return available() }

// Send 弹出桌面通知.
func (n *Notifier) Send(ctx context.Context, msg notify.Message) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()
	if err := notifySystem(ctx, msg.Title, condense(msg.Text, maxBodyRunes), n.sound); err != nil {
		return err
	}
	n.logger.Debug("系统通知已发送", "elapsed", time.Since(start).Round(time.Millisecond))
	return nil
}

// condense 去掉空行并截断正文.
func condense(text string, limit int) string {
	lines := make([]string, 0, 8)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}

	out := strings.Join(lines, "\n")
	runes := []rune(out)
	if len(runes) > limit {
		return string(runes[:limit]) + "..."
	}
	return out
}
