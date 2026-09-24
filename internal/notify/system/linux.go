//go:build linux

package system

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// available 判断是否存在可用的桌面通知命令.
func available() bool {
	for _, name := range []string{"notify-send", "zenity", "kdialog"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// notifySystem 优先使用 freedesktop 的 notify-send, 缺失时退回 zenity / kdialog.
func notifySystem(ctx context.Context, title, body string, _ bool) error {
	var errs []error

	if path, err := exec.LookPath("notify-send"); err == nil {
		args := []string{"--app-name=rainmail", "--icon=weather-showers-scattered", title, body}
		if output, err := exec.CommandContext(ctx, path, args...).CombinedOutput(); err == nil {
			return nil
		} else {
			errs = append(errs, fmt.Errorf("notify-send: %w (%s)", err, strings.TrimSpace(string(output))))
		}
	}

	if path, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--info", "--title", title, "--text", body}
		if output, err := exec.CommandContext(ctx, path, args...).CombinedOutput(); err == nil {
			return nil
		} else {
			errs = append(errs, fmt.Errorf("zenity: %w (%s)", err, strings.TrimSpace(string(output))))
		}
	}

	if path, err := exec.LookPath("kdialog"); err == nil {
		args := []string{"--title", title, "--msgbox", body}
		if output, err := exec.CommandContext(ctx, path, args...).CombinedOutput(); err == nil {
			return nil
		} else {
			errs = append(errs, fmt.Errorf("kdialog: %w (%s)", err, strings.TrimSpace(string(output))))
		}
	}

	if len(errs) == 0 {
		return errors.New("未找到桌面通知命令, 请安装 libnotify-bin (提供 notify-send)")
	}
	return fmt.Errorf("系统通知发送失败: %w", errors.Join(errs...))
}
