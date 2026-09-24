package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// QuietWindow 表示一个免打扰时段, 以自午夜起的分钟数描述.
type QuietWindow struct {
	Start int
	End   int
	Raw   string
}

// ParseQuietWindow 解析 "23:30-07:00" 形式的免打扰时段, 支持跨零点.
func ParseQuietWindow(s string) (QuietWindow, error) {
	raw := strings.TrimSpace(s)
	parts := strings.SplitN(raw, "-", 2)
	if len(parts) != 2 {
		return QuietWindow{}, fmt.Errorf("免打扰时段 %q 格式错误, 应为 23:30-07:00", s)
	}

	start, err := parseClock(parts[0])
	if err != nil {
		return QuietWindow{}, fmt.Errorf("免打扰时段 %q 的起始时间无效: %w", s, err)
	}
	end, err := parseClock(parts[1])
	if err != nil {
		return QuietWindow{}, fmt.Errorf("免打扰时段 %q 的结束时间无效: %w", s, err)
	}
	if start == end {
		return QuietWindow{}, fmt.Errorf("免打扰时段 %q 的起止时间相同", s)
	}
	return QuietWindow{Start: start, End: end, Raw: raw}, nil
}

// Contains 判断 t 是否落在时段内. 跨零点时段(如 23:30-07:00)同样成立.
func (w QuietWindow) Contains(t time.Time) bool {
	minutes := t.Hour()*60 + t.Minute()
	if w.Start < w.End {
		return minutes >= w.Start && minutes < w.End
	}
	return minutes >= w.Start || minutes < w.End
}

// String 返回时段原文.
func (w QuietWindow) String() string { return w.Raw }

// ParseQuietWindows 批量解析时段列表.
func ParseQuietWindows(list []string) ([]QuietWindow, error) {
	out := make([]QuietWindow, 0, len(list))
	for _, item := range list {
		w, err := ParseQuietWindow(item)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

// InQuietHours 判断 t 是否处于任一时段内.
func InQuietHours(windows []QuietWindow, t time.Time) bool {
	for _, w := range windows {
		if w.Contains(t) {
			return true
		}
	}
	return false
}

func parseClock(s string) (int, error) {
	text := strings.TrimSpace(s)
	parts := strings.SplitN(text, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("%q 不是 HH:MM 形式", s)
	}
	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, fmt.Errorf("小时 %q 不是数字", parts[0])
	}
	minute, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, fmt.Errorf("分钟 %q 不是数字", parts[1])
	}
	if hour < 0 || hour > 23 {
		return 0, fmt.Errorf("小时 %d 超出 0 到 23", hour)
	}
	if minute < 0 || minute > 59 {
		return 0, fmt.Errorf("分钟 %d 超出 0 到 59", minute)
	}
	return hour*60 + minute, nil
}
