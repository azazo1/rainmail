package migration

import (
	"errors"
	"testing"
)

func testSteps() []Step {
	return []Step{
		{From: 1, To: 2, Description: "把 a 改名为 b", Apply: func(raw map[string]any) error {
			raw["b"] = raw["a"]
			delete(raw, "a")
			return nil
		}},
		{From: 2, To: 3, Description: "补上 c", Apply: func(raw map[string]any) error {
			raw["c"] = 1
			return nil
		}},
	}
}

func TestMigrateWithAppliesAllSteps(t *testing.T) {
	raw := map[string]any{"a": "value"}

	applied, err := MigrateWith(testSteps(), raw, 1, 3)
	if err != nil {
		t.Fatalf("迁移不应失败: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("应执行 2 步迁移, 实际为 %d", len(applied))
	}
	if _, ok := raw["a"]; ok {
		t.Fatal("旧字段 a 应已被删除")
	}
	if raw["b"] != "value" {
		t.Fatalf("字段 b 应为 value, 实际为 %v", raw["b"])
	}
	if raw["c"] != 1 {
		t.Fatalf("字段 c 应为 1, 实际为 %v", raw["c"])
	}
}

func TestMigrateWithRejectsMissingStep(t *testing.T) {
	_, err := MigrateWith(testSteps(), map[string]any{}, 1, 4)
	if err == nil {
		t.Fatal("缺少迁移步骤时应报错")
	}
}

func TestMigrateWithRejectsDowngrade(t *testing.T) {
	_, err := MigrateWith(testSteps(), map[string]any{}, 3, 1)
	if err == nil {
		t.Fatal("目标版本低于当前版本时应报错")
	}
}

func TestMigrateWithReportsStepFailure(t *testing.T) {
	failing := []Step{{
		From: 1, To: 2, Description: "故意失败",
		Apply: func(map[string]any) error { return errors.New("boom") },
	}}

	if _, err := MigrateWith(failing, map[string]any{}, 1, 2); err == nil {
		t.Fatal("迁移步骤出错时应向上报错")
	}
}

func TestMigrateWithSameVersionIsNoop(t *testing.T) {
	applied, err := MigrateWith(testSteps(), map[string]any{}, 2, 2)
	if err != nil {
		t.Fatalf("相同版本不应报错: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("相同版本不应执行迁移, 实际为 %d 步", len(applied))
	}
}

func TestLookupReadsNestedTable(t *testing.T) {
	raw := map[string]any{
		"weather": map[string]any{"provider": "open-meteo"},
	}

	value, ok := Lookup(raw, "weather", "provider")
	if !ok || value != "open-meteo" {
		t.Fatalf("应读取到 open-meteo, 实际为 %v / %v", value, ok)
	}
	if _, ok := Lookup(raw, "weather", "missing"); ok {
		t.Fatal("不存在的键应返回 false")
	}
	if _, ok := Lookup(raw, "weather", "provider", "deeper"); ok {
		t.Fatal("路径穿过非表值时应返回 false")
	}
}
