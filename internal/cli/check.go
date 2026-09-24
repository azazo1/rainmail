package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/azazo1/rainmail/internal/app"
	"github.com/azazo1/rainmail/internal/report"
)

func newCheckCommand(flags *globalFlags) *cobra.Command {
	var (
		dryRun   bool
		force    bool
		channels []string
		timeout  time.Duration
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "立刻检查一次并输出判定结果",
		Long: "查询一次天气并给出判定结果. 默认按冷却规则发送提醒,\n" +
			"--dry-run 只展示判定不发送, --force 忽略冷却强制发送.",
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

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			result, err := service.Check(ctx, app.CheckOptions{
				DryRun:   dryRun,
				Force:    force,
				Channels: channels,
			})
			if err != nil {
				return err
			}

			printCheckResult(cmd.OutOrStdout(), result)
			return result.Err()
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "只查询与判定, 不发送提醒")
	cmd.Flags().BoolVar(&force, "force", false, "忽略冷却时间强制发送提醒")
	cmd.Flags().StringSliceVar(&channels, "channel", nil, "限定提醒通道, 可重复指定, 默认全部")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "本次检查的总超时")

	return cmd
}

func printCheckResult(out io.Writer, result *app.Result) {
	fmt.Fprint(out, report.Text(result.Assessment))
	fmt.Fprintln(out, "---")

	if result.Skipped != "" {
		fmt.Fprintf(out, "未发送提醒: %s\n", result.Skipped)
	}
	for _, channel := range result.Channels {
		status := "成功"
		if channel.Err != nil {
			status = "失败: " + channel.Err.Error()
		}
		fmt.Fprintf(out, "通道 %s: %s (%s)\n", channel.Name, status, channel.Duration.Round(time.Millisecond))
	}
	fmt.Fprintf(out, "耗时: %s\n", result.Duration.Round(time.Millisecond))
}
