package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"

	"github.com/azazo1/rainmail/internal/config/migration"
)

// EnvConfigPath 是覆盖配置文件路径的环境变量名.
const EnvConfigPath = "RAINMAIL_CONFIG"

// Resolve 按优先级确定配置文件路径, 但不保证文件真实存在.
//
// 优先级: 显式指定 > RAINMAIL_CONFIG 环境变量 > 平台配置目录 > 当前目录.
func Resolve(explicit string) (string, error) {
	if explicit != "" {
		return absPath(explicit)
	}
	if env := os.Getenv(EnvConfigPath); env != "" {
		return absPath(env)
	}

	var candidates []string
	if dir, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(dir, "rainmail", "config.toml"))
	}
	candidates = append(candidates, "rainmail.toml", "config.toml")

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return absPath(candidate)
		}
	}
	return absPath(candidates[0])
}

// Exists 判断配置文件是否存在.
func Exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Load 读取并解析配置文件, 必要时先执行结构版本迁移.
//
// 返回的 warnings 描述迁移过程中发生的事情, 由调用方决定如何展示.
func Load(path string) (*Config, []string, error) {
	if migration.Current != CurrentVersion {
		return nil, nil, fmt.Errorf("配置结构版本不一致: config 为 %d, migration 为 %d",
			CurrentVersion, migration.Current)
	}

	raw := map[string]any{}
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("配置文件 %s 不存在, 可先执行 rainmail config init 生成: %w", path, err)
		}
		return nil, nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}

	version, err := readVersion(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("配置文件 %s 的 config_version 无效: %w", path, err)
	}
	if version > CurrentVersion {
		return nil, nil, fmt.Errorf("配置文件 %s 的结构版本 %d 高于本程序支持的 %d, 请升级 rainmail",
			path, version, CurrentVersion)
	}

	var warnings []string
	if version < CurrentVersion {
		warnings, err = migrateConfigFile(path, raw, version)
		if err != nil {
			return nil, warnings, err
		}
	}

	cfg := Default()
	if err := decodeMap(raw, cfg); err != nil {
		return nil, warnings, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	cfg.ConfigVersion = CurrentVersion
	cfg.Normalize()
	expandSecrets(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, warnings, fmt.Errorf("配置文件 %s 校验失败:\n%w", path, err)
	}
	return cfg, warnings, nil
}

func readVersion(raw map[string]any) (int, error) {
	value, ok := raw["config_version"]
	if !ok {
		return 0, nil
	}
	switch t := value.(type) {
	case int64:
		return int(t), nil
	case int:
		return t, nil
	case float64:
		return int(t), nil
	case string:
		parsed, err := strconv.Atoi(t)
		if err != nil {
			return 0, fmt.Errorf("%q 不是整数", t)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("类型 %T 不是整数", value)
	}
}

// migrateConfigFile 备份原文件, 执行迁移并把结果写回.
func migrateConfigFile(path string, raw map[string]any, from int) ([]string, error) {
	var warnings []string

	if from == 0 {
		warnings = append(warnings, fmt.Sprintf("配置文件 %s 缺少 config_version, 已按 v%d 结构解析并补写", path, CurrentVersion))
	} else {
		applied, err := migration.Migrate(raw, from, CurrentVersion)
		if err != nil {
			return warnings, fmt.Errorf("迁移配置文件 %s 失败: %w", path, err)
		}
		warnings = append(warnings, applied...)
	}
	raw["config_version"] = CurrentVersion

	backup := fmt.Sprintf("%s.v%d.bak", path, from)
	if err := copyFile(path, backup); err != nil {
		return warnings, fmt.Errorf("备份原配置到 %s 失败: %w", backup, err)
	}
	if err := writeRaw(path, raw); err != nil {
		return warnings, fmt.Errorf("写回迁移后的配置 %s 失败: %w", path, err)
	}
	warnings = append(warnings, fmt.Sprintf("原配置已备份为 %s", backup))
	return warnings, nil
}

func writeRaw(path string, raw map[string]any) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
		return fmt.Errorf("编码配置失败: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func decodeMap(raw map[string]any, cfg *Config) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
		return err
	}
	if _, err := toml.NewDecoder(&buf).Decode(cfg); err != nil {
		return err
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

// expandSecrets 展开敏感字段中的 ${ENV} 占位符, 避免明文写进配置文件.
func expandSecrets(cfg *Config) {
	cfg.Notify.Email.Username = os.ExpandEnv(cfg.Notify.Email.Username)
	cfg.Notify.Email.Password = os.ExpandEnv(cfg.Notify.Email.Password)

	for name, settings := range cfg.Weather.Providers {
		settings.APIKey = os.ExpandEnv(settings.APIKey)
		settings.URL = os.ExpandEnv(settings.URL)
		if len(settings.Headers) > 0 {
			headers := make(map[string]string, len(settings.Headers))
			for k, v := range settings.Headers {
				headers[k] = os.ExpandEnv(v)
			}
			settings.Headers = headers
		}
		cfg.Weather.Providers[name] = settings
	}
}

func absPath(p string) (string, error) {
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("解析路径 %s 失败: %w", p, err)
	}
	return abs, nil
}

func joinDir(configPath, name string) string {
	return filepath.Join(filepath.Dir(configPath), name)
}
