// Package migration 负责配置文件结构版本之间的迁移.
//
// 每个 Step 描述一次相邻版本的变化, Migrate 从旧版本逐级应用到目标版本.
// 迁移只操作解码后的原始 map, 不依赖 config 包, 避免循环依赖.
//
// 新增一次迁移的做法:
//  1. 递增 config.CurrentVersion;
//  2. 在本包 steps 中追加 Step{From: N, To: N+1, Description: "...", Apply: ...};
//  3. Apply 内只做结构改写, 不要读取环境变量或网络.
package migration

import "fmt"

// Current 是当前配置结构版本, 必须与 config.CurrentVersion 保持一致.
const Current = 1

// Step 描述一次相邻版本之间的迁移.
type Step struct {
	From        int
	To          int
	Description string
	Apply       func(raw map[string]any) error
}

// steps 按 From 升序登记所有历史迁移.
//
// 示例(仅作格式说明, 当前版本尚无迁移):
//
//	{From: 1, To: 2, Description: "把 weather.notify_probability 重命名为 notify.notify_on_possible", Apply: ...}
var steps []Step

// Migrate 把 raw 从 from 版本迁移到 to 版本, 返回执行过的迁移描述.
func Migrate(raw map[string]any, from, to int) ([]string, error) {
	return MigrateWith(steps, raw, from, to)
}

// MigrateWith 使用指定步骤集执行迁移, 便于测试.
func MigrateWith(set []Step, raw map[string]any, from, to int) ([]string, error) {
	if from == to {
		return nil, nil
	}
	if from > to {
		return nil, fmt.Errorf("配置版本 %d 高于本程序支持的 %d, 请升级 rainmail", from, to)
	}

	var applied []string
	current := from
	for current < to {
		step, ok := findStep(set, current)
		if !ok {
			return applied, fmt.Errorf("缺少从版本 %d 到 %d 的迁移步骤", current, current+1)
		}
		if err := step.Apply(raw); err != nil {
			return applied, fmt.Errorf("执行迁移 %d -> %d (%s) 失败: %w", step.From, step.To, step.Description, err)
		}
		applied = append(applied, fmt.Sprintf("%d -> %d: %s", step.From, step.To, step.Description))
		current = step.To
	}
	return applied, nil
}

func findStep(set []Step, from int) (Step, bool) {
	for _, s := range set {
		if s.From == from {
			return s, true
		}
	}
	return Step{}, false
}

// Lookup 按键路径读取嵌套表, 供迁移步骤使用.
func Lookup(raw map[string]any, path ...string) (any, bool) {
	var current any = raw
	for _, key := range path {
		table, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := table[key]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}
