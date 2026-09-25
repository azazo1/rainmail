// Package cli 实现 rainmail 的命令行入口.
package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/azazo1/rainmail/internal/app"
	"github.com/azazo1/rainmail/internal/buildinfo"
	"github.com/azazo1/rainmail/internal/config"
	"github.com/azazo1/rainmail/internal/logx"
	"github.com/azazo1/rainmail/internal/notify"
)

// globalFlags 是各子命令共享的全局参数.
type globalFlags struct {
	configPath string
	logLevel   string
	logFormat  string
	verbose    bool
}

// Execute 运行命令行入口, 返回非 nil 时调用方应以非零码退出.
func Execute() error {
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "rainmail: %v\n", err)
		return err
	}
	return nil
}

func newRootCommand() *cobra.Command {
	flags := &globalFlags{}

	root := &cobra.Command{
		Use:   "rainmail",
		Short: "降雨提醒工具: 检测到降水时通过邮件或系统通知提醒",
		Long: "rainmail 定时查询天气接口, 在观察窗口内检测到降水时,\n" +
			"通过邮件与桌面通知提醒.\n" +
			"内置天气接口: open-meteo / openweathermap / qweather / seniverse / custom.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       buildinfo.Version(),
		Example: "  rainmail config init   # 生成示例配置\n" +
			"  rainmail check         # 立刻检查一次\n" +
			"  rainmail run           # 常驻轮询",
	}

	root.PersistentFlags().StringVarP(&flags.configPath, "config", "c", "",
		"配置文件路径, 默认 ~/.config/rainmail/config.toml, 也可用 $RAINMAIL_CONFIG 指定")
	root.PersistentFlags().StringVar(&flags.logLevel, "log-level", "",
		"日志级别 debug/info/warn/error, 覆盖配置文件")
	root.PersistentFlags().StringVar(&flags.logFormat, "log-format", "",
		"日志格式 text/json, 覆盖配置文件")
	root.PersistentFlags().BoolVarP(&flags.verbose, "verbose", "v", false,
		"等价于 --log-level debug")

	root.AddCommand(
		newRunCommand(flags),
		newCheckCommand(flags),
		newTestCommand(flags),
		newConfigCommand(flags),
		newVersionCommand(),
	)

	return root
}

// runtime 汇总一次命令执行所需的配置与日志设施.
type runtime struct {
	path      string
	cfg       *config.Config
	logger    *slog.Logger
	logCloser io.Closer
}

// setupRuntime 解析配置文件路径, 加载配置, 并按配置与命令行参数初始化日志.
//
// 配置加载阶段还没有日志设施, 因此迁移提示会在 logger 就绪后补记.
func setupRuntime(flags *globalFlags) (*runtime, error) {
	path, err := config.Resolve(flags.configPath)
	if err != nil {
		return nil, err
	}

	cfg, warnings, err := config.Load(path)
	if err != nil {
		return nil, err
	}

	level := cfg.Log.Level
	format := cfg.Log.Format
	if flags.verbose {
		level = "debug"
	}
	if flags.logLevel != "" {
		level = flags.logLevel
	}
	if flags.logFormat != "" {
		format = flags.logFormat
	}

	logger, closer, err := logx.Setup(logx.Options{Level: level, Format: format, File: cfg.Log.File})
	if err != nil {
		return nil, err
	}

	for _, warning := range warnings {
		logger.Warn("配置已自动迁移", "detail", warning)
	}

	return &runtime{path: path, cfg: cfg, logger: logger, logCloser: closer}, nil
}

// Close 释放日志文件句柄.
func (r *runtime) Close() {
	if r.logCloser != nil {
		_ = r.logCloser.Close()
	}
}

// newService 依据配置构造业务服务.
func (r *runtime) newService() (*app.Service, error) {
	return app.New(app.Options{
		Config: r.cfg,
		Logger: r.logger,
	})
}

// notifiersFor 挑选出指定名字的提醒通道, 便于 test 子命令只测试单个通道.
func (r *runtime) notifiersFor(names []string) ([]notify.Notifier, error) {
	all, err := app.BuildNotifiers(r.cfg, r.logger)
	if err != nil {
		return nil, err
	}

	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}

	selected := make([]notify.Notifier, 0, len(all))
	for _, channel := range all {
		if wanted[channel.Name()] {
			selected = append(selected, channel)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("配置中没有启用通道 %v", names)
	}
	return selected, nil
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "打印版本信息",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "%s version %s\n", cmd.Root().Name(), buildinfo.Version())
			return nil
		},
	}
}
