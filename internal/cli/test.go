package cli

import (
	"context"
	"fmt"
	"html"
	"time"

	"github.com/spf13/cobra"

	"github.com/azazo1/rainmail/internal/app"
	"github.com/azazo1/rainmail/internal/notify"
)

func newTestCommand(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "测试提醒通道是否配置正确",
		Long:  "发送一条测试提醒, 用于确认 SMTP 参数或桌面通知机制可用.",
	}
	cmd.AddCommand(
		newTestChannelCommand(flags, app.ChannelEmail, "email", "发送一封测试邮件"),
		newTestChannelCommand(flags, app.ChannelSystem, "system", "弹出一条测试通知"),
	)
	return cmd
}

func newTestChannelCommand(flags *globalFlags, channel, use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := setupRuntime(flags)
			if err != nil {
				return err
			}
			defer rt.Close()

			selected, err := rt.notifiersFor([]string{channel})
			if err != nil {
				return err
			}
			dispatcher := notify.NewDispatcher(rt.logger, selected...)

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			message := notify.Message{
				Title: "测试提醒",
				Text:  testText(rt),
				HTML:  testHTML(rt),
				Level: notify.LevelInfo,
			}

			fmt.Fprintf(cmd.OutOrStdout(), "正在通过 %s 通道发送测试提醒\n", channel)
			results := dispatcher.SendTo(ctx, []string{channel}, message)
			for _, result := range results {
				if result.Err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "通道 %s 发送失败: %v\n", result.Name, result.Err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "通道 %s 发送成功 (%s)\n",
					result.Name, result.Duration.Round(time.Millisecond))
			}
			return notify.Errors(results)
		},
	}
}

func testText(rt *runtime) string {
	return fmt.Sprintf("这是一条来自 rainmail 的测试提醒, 收到即表示该通道可用.\n\n地点: %s\n配置: %s\n时间: %s\n",
		rt.cfg.Location.Name,
		rt.path,
		time.Now().Format("2006-01-02 15:04:05"))
}

func testHTML(rt *runtime) string {
	return fmt.Sprintf(`<div style="font-family:-apple-system,'Segoe UI',Roboto,Arial,sans-serif;font-size:14px;line-height:1.7">
<p>这是一条来自 rainmail 的测试提醒, 收到即表示该通道可用.</p>
<ul>
<li>地点: %s</li>
<li>配置: %s</li>
<li>时间: %s</li>
</ul>
</div>`,
		html.EscapeString(rt.cfg.Location.Name),
		html.EscapeString(rt.path),
		time.Now().Format("2006-01-02 15:04:05"))
}
