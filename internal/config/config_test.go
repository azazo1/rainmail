package config

import (
	"path/filepath"
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

// setHome 让 os.UserHomeDir 在 Unix 与 Windows 上都指向 dir:
// 前者读 HOME, 后者读 USERPROFILE.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestResolveUsesFixedConfigPath(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	t.Setenv(EnvConfigPath, "")

	path, err := Resolve("")
	if err != nil {
		t.Fatalf("解析默认路径失败: %v", err)
	}
	want := filepath.Join(home, ".config", "rainmail", "config.toml")
	if path != want {
		t.Fatalf("默认配置路径应为 %s, 实际为 %s", want, path)
	}
}

func TestResolveHonorsExplicitAndEnv(t *testing.T) {
	setHome(t, t.TempDir())

	explicit := filepath.Join(t.TempDir(), "explicit.toml")
	got, err := Resolve(explicit)
	if err != nil {
		t.Fatalf("解析显式路径失败: %v", err)
	}
	if got != filepath.Clean(explicit) {
		t.Fatalf("--config 应被原样采用, 实际为 %s", got)
	}

	envPath := filepath.Join(t.TempDir(), "env.toml")
	t.Setenv(EnvConfigPath, envPath)
	fromEnv, err := Resolve("")
	if err != nil {
		t.Fatalf("解析环境变量路径失败: %v", err)
	}
	if fromEnv != filepath.Clean(envPath) {
		t.Fatalf("RAINMAIL_CONFIG 应被采用, 实际为 %s", fromEnv)
	}
}

func TestStatePathUsesLocalStateDir(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)

	cfg := Default()
	path, err := cfg.StatePath()
	if err != nil {
		t.Fatalf("解析状态文件路径失败: %v", err)
	}
	want := filepath.Join(home, ".local", "state", "rainmail", "state.json")
	if path != want {
		t.Fatalf("状态文件路径应为 %s, 实际为 %s", want, path)
	}

	custom := filepath.Join(t.TempDir(), "state.json")
	cfg.Repeat.StateFile = custom
	got, err := cfg.StatePath()
	if err != nil {
		t.Fatalf("解析自定义状态文件路径失败: %v", err)
	}
	if got != custom {
		t.Fatalf("state_file 应被采用, 实际为 %s", got)
	}
}
