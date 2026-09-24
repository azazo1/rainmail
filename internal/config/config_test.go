package config

import (
	"testing"
	"time"
)

func TestParseQuietWindow(t *testing.T) {
	window, err := ParseQuietWindow("23:30-07:00")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if window.Start != 23*60+30 || window.End != 7*60 {
		t.Fatalf("起止分钟数不符: %+v", window)
	}
}

func TestQuietWindowCrossesMidnight(t *testing.T) {
	window, err := ParseQuietWindow("23:30-07:00")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	loc := time.FixedZone("CST", 8*3600)
	cases := []struct {
		hour    int
		minute  int
		inside  bool
		comment string
	}{
		{23, 0, false, "起始之前"},
		{23, 45, true, "起始之后"},
		{0, 30, true, "跨零点之后"},
		{6, 59, true, "结束之前"},
		{7, 0, false, "结束时刻本身不算在内"},
		{12, 0, false, "白天"},
		{22, 0, false, "起始之前"},
	}

	for _, c := range cases {
		at := time.Date(2026, 9, 24, c.hour, c.minute, 0, 0, loc)
		if got := window.Contains(at); got != c.inside {
			t.Fatalf("%02d:%02d (%s) 应为 %v, 实际为 %v", c.hour, c.minute, c.comment, c.inside, got)
		}
	}
}

func TestQuietWindowRejectsBadInput(t *testing.T) {
	for _, raw := range []string{"2300-0700", "23:00", "25:00-07:00", "23:70-07:00", "08:00-08:00"} {
		if _, err := ParseQuietWindow(raw); err == nil {
			t.Fatalf("%q 应被判为非法", raw)
		}
	}
}

func TestDurationParsesStringAndSeconds(t *testing.T) {
	var d Duration
	if err := d.UnmarshalTOML("30m"); err != nil {
		t.Fatalf("解析 30m 失败: %v", err)
	}
	if d.Duration() != 30*time.Minute {
		t.Fatalf("30m 应解析为 30 分钟, 实际为 %s", d)
	}
	if err := d.UnmarshalTOML(int64(90)); err != nil {
		t.Fatalf("解析秒数失败: %v", err)
	}
	if d.Duration() != 90*time.Second {
		t.Fatalf("90 秒应解析为 1m30s, 实际为 %s", d)
	}
	if err := d.UnmarshalTOML("半小时"); err == nil {
		t.Fatal("非法时长应报错")
	}
}
