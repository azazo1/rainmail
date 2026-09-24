package cli

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

func newRunCommand(flags *globalFlags) *cobra.Command {
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "run",
		Short: "常驻运行, 按间隔轮询天气并在检测到降水时提醒",
		Long: "常驻运行, 启动时立即检查一次, 之后按 weather.check_interval 轮询.\n" +
			"收到 Ctrl+C 或 SIGTERM 后平滑退出.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := setupRuntime(flags)
			if err != nil {
				return err
			}
			defer rt.Close()

			service, err := rt.newService()
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return service.Run(ctx, interval)
		},
	}

	cmd.Flags().DurationVar(&interval, "interval", 0,
		"轮询间隔, 默认取配置中的 weather.check_interval")

	return cmd
}
