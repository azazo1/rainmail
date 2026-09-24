//go:build !darwin && !linux && !windows

package system

import (
	"context"
	"fmt"
	"runtime"
)

// available 在未知平台上恒为 false.
func available() bool { return false }

// notifySystem 在未知平台上直接返回错误.
func notifySystem(_ context.Context, _, _ string, _ bool) error {
	return fmt.Errorf("当前平台 %s 暂不支持系统通知, 可改用邮件通道", runtime.GOOS)
}
