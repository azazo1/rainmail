// Package notify 定义提醒消息与提醒通道抽象.
//
// 具体通道(邮件, 系统通知)实现 Notifier 接口,
// Dispatcher 负责把一条消息分发到多个通道, 单个通道失败不影响其它通道.
package notify

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Level 表示提醒的紧急程度.
type Level string

// 提醒级别取值.
const (
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
)

// Message 是一条待发送的提醒.
type Message struct {
	Title string
	// Text 是纯文本正文.
	Text string
	// HTML 是可选 HTML 正文, 为空时只发送纯文本.
	HTML  string
	Level Level
	// Tags 携带便于追踪的键值, 例如 event_key.
	Tags map[string]string
}

// Notifier 是一个提醒通道.
type Notifier interface {
	// Name 返回通道标识, 用于日志与 test 子命令.
	Name() string
	// Send 发送一条提醒.
	Send(ctx context.Context, msg Message) error
}

// Result 记录单个通道的发送结果.
type Result struct {
	Name     string
	Err      error
	Duration time.Duration
}

// Dispatcher 把消息依次发送到多个通道.
type Dispatcher struct {
	notifiers []Notifier
	logger    *slog.Logger
}

// NewDispatcher 构造分发器.
func NewDispatcher(logger *slog.Logger, notifiers ...Notifier) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{notifiers: notifiers, logger: logger}
}

// Notifiers 返回已注册的通道.
func (d *Dispatcher) Notifiers() []Notifier { return d.notifiers }

// Names 返回已注册的通道名.
func (d *Dispatcher) Names() []string {
	names := make([]string, 0, len(d.notifiers))
	for _, n := range d.notifiers {
		names = append(names, n.Name())
	}
	return names
}

// SendTo 向指定通道发送, names 为空表示全部已注册通道.
func (d *Dispatcher) SendTo(ctx context.Context, names []string, msg Message) []Result {
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}

	results := make([]Result, 0, len(d.notifiers))
	for _, notifier := range d.notifiers {
		if len(wanted) > 0 && !wanted[notifier.Name()] {
			continue
		}
		start := time.Now()
		err := notifier.Send(ctx, msg)
		elapsed := time.Since(start)
		results = append(results, Result{Name: notifier.Name(), Err: err, Duration: elapsed})

		if err != nil {
			d.logger.Error("提醒通道发送失败",
				"channel", notifier.Name(),
				"error", err,
				"elapsed", elapsed.Round(time.Millisecond))
			continue
		}
		d.logger.Info("提醒已发送",
			"channel", notifier.Name(),
			"elapsed", elapsed.Round(time.Millisecond))
	}
	return results
}

// Errors 把结果中的错误合并成一个 error, 全部成功时返回 nil.
func Errors(results []Result) error {
	var errs []error
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, errors.New(r.Name+": "+r.Err.Error()))
		}
	}
	return errors.Join(errs...)
}

// Succeeded 返回成功发送的通道名.
func Succeeded(results []Result) []string {
	names := make([]string, 0, len(results))
	for _, r := range results {
		if r.Err == nil {
			names = append(names, r.Name)
		}
	}
	return names
}
