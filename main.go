// rainmail 是一个降雨提醒工具: 定时查询天气接口, 在检测到未来有降水时
// 通过邮件或系统通知提醒用户.
package main

import (
	"os"

	// 嵌入 IANA 时区数据库, 保证 Windows 等缺少系统时区库的平台也能解析配置中的时区名.
	_ "time/tzdata"

	"github.com/azazo1/rainmail/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
