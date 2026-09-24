package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/azazo1/rainmail/internal/config"
)

func newConfigCommand(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "管理配置文件",
	}
	cmd.AddCommand(
		newConfigInitCommand(flags),
		newConfigPathCommand(flags),
		newConfigShowCommand(flags),
	)
	return cmd
}

func newConfigInitCommand(flags *globalFlags) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "生成带注释的示例配置文件",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Resolve(flags.configPath)
			if err != nil {
				return err
			}
			if config.Exists(path) && !force {
				return fmt.Errorf("配置文件 %s 已存在, 如需覆盖请加 --force", path)
			}
			if dir := filepath.Dir(path); dir != "" && dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
				}
			}
			if err := os.WriteFile(path, []byte(config.ExampleTemplate()), 0o600); err != nil {
				return fmt.Errorf("写入配置文件 %s 失败: %w", path, err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "已生成配置文件: %s\n", path)
			fmt.Fprintln(out, "请按注释填写邮箱与收件地址后再执行 rainmail check 验证.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "覆盖已存在的配置文件")
	return cmd
}

func newConfigPathCommand(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "打印生效的配置文件路径",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Resolve(flags.configPath)
			if err != nil {
				return err
			}
			statePath := filepath.Join(filepath.Dir(path), "state.json")
			if config.Exists(path) {
				if cfg, _, err := config.Load(path); err == nil {
					statePath = cfg.StatePath(path)
				}
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "配置文件: %s (%s)\n", path, existence(path))
			fmt.Fprintf(out, "状态文件: %s\n", statePath)
			return nil
		},
	}
}

func newConfigShowCommand(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "打印生效配置, 敏感字段已打码",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := setupRuntime(flags)
			if err != nil {
				return err
			}
			defer rt.Close()

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "# 配置文件: %s\n", rt.path)
			fmt.Fprintf(out, "# 状态文件: %s\n\n", rt.cfg.StatePath(rt.path))
			return toml.NewEncoder(out).Encode(rt.cfg.MaskSecrets())
		},
	}
}

func existence(path string) string {
	if config.Exists(path) {
		return "存在"
	}
	return "尚未创建"
}
