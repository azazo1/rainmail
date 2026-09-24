# rainmail

一个用 Go 编写的降雨提醒工具. 它按固定间隔查询天气接口, 在未来若干小时内检测到降水时,
通过邮件或桌面通知提醒你. 支持 Windows, macOS 与 Linux.

## 特性

- 多天气接口: 内置 open-meteo (免密钥), OpenWeatherMap, 和风天气, 心知天气,
  以及用 JSON 路径映射的 custom 接口, 可对接任意返回逐小时预报的服务.
- 双提醒通道: SMTP 邮件 (隐式 TLS / STARTTLS / 明文) 与系统桌面通知 (三端原生机制).
- 不重复打扰: 同一场降水在冷却时间内只提醒一次, 免打扰时段可以只保留邮件.
- 配置可演进: 配置文件带结构版本号, 版本变化时自动迁移并备份原文件.
- 结构化日志: 基于 log/slog, 覆盖查询, 判定, 通知与状态变化.

## 快速开始

```shell
just init-config   # 生成带注释的配置, 默认落在平台配置目录
just check         # 立刻检查一次, 只展示判定结果
just test-email    # 发一封测试邮件, 确认 SMTP 参数
just run           # 常驻轮询
```

不使用 just 时, 把 `just xxx` 换成对应的 `go run . xxx`, 构建产物则为 `./rainmail xxx`.

## 安装

```shell
# 从源码构建当前平台
just build

# 交叉编译三端产物到 dist/
just build-all

# 或者直接从模块安装
go install github.com/azazo1/rainmail@latest
```

## 配置

配置文件使用 TOML, 三个平台统一放在 `~/.config/rainmail/config.toml`.
查找顺序为 `--config` 参数, `RAINMAIL_CONFIG` 环境变量, 最后落到上述默认路径.
`rainmail config path` 打印当前生效的路径, `rainmail config show` 打印生效配置 (口令与密钥已打码).

完整字段与注释见 [internal/config/example.toml](internal/config/example.toml),
可以直接用 `rainmail config init` 生成一份. `rainmail config path` 打印当前生效的路径,
`rainmail config show` 打印生效配置 (口令与密钥已打码).

### 天气接口

| provider | 是否需要密钥 | 说明 |
| --- | --- | --- |
| `open-meteo` | 否 | 默认接口, 全球覆盖, 逐小时降水量与概率 |
| `openweathermap` | 是 | 免费额度为 3 小时粒度, 程序会平摊成逐小时 |
| `qweather` | 是 | 和风天气, 国内访问稳定, 兼顾降水概率 |
| `seniverse` | 是 | 心知天气, 国内访问稳定 |
| `custom` | 视接口而定 | 用 gjson 路径映射任意 JSON 响应 |

只有 `weather.provider` 指向的那一组 `weather.providers.*` 参数会被使用.
密钥支持 `${VAR}` 占位符, 建议通过环境变量提供, 不要明文写进配置文件.

custom 接口支持两种响应形态:

- 并行数组, 例如 `{"hourly": {"time": [...], "precipitation": [...]}}`,
  `items_path` 留空, 各 `*_path` 从响应根写起.
- 对象数组, 例如 `{"list": [{"dt": ..., "rain": ...}, ...]}`,
  填写 `items_path = "list"`, 各 `*_path` 相对数组元素书写.

降水量按毫米, 概率按 0-100 的百分数解析. 若接口返回 0-1 的比例, 需要在接口侧换算.

`code_system` 声明接口天气码所属的码表 (`none` / `wmo` / `owm` / `qweather` / `seniverse`),
留空或填 `none` 时只依据降水量判断, 适合码值含义不明的接口.

### 判定规则

在 `lookahead_hours` 覆盖的时间窗口内, 每个小时按两条阈值归类:

- 小时降水量达到 `min_precipitation_mm`, 或天气码本身表示降水: 判定为**确定降水**, 一定提醒.
- 仅降水概率达到 `min_probability`: 判定为**可能降水**, 由 `notify.notify_on_possible` 决定是否提醒.

降雪默认不计入, 需要提醒时把 `weather.include_snow` 打开.

### 邮件通道

常见邮箱的 SMTP 参数:

| 邮箱 | 服务器 | 端口 | 加密 | 口令 |
| --- | --- | --- | --- | --- |
| QQ 邮箱 | `smtp.qq.com` | 465 | `tls` | 设置中生成的授权码 |
| 163 邮箱 | `smtp.163.com` | 465 | `tls` | 客户端授权码 |
| Gmail | `smtp.gmail.com` | 587 | `starttls` | 应用专用密码 |
| 自建服务器 | 视情况 | 587 / 25 | `starttls` / `none` | 视情况 |

`encryption` 留空时按端口推断 (465 用 `tls`, 587 与 25 用 `starttls`).
使用自签名证书的内网服务器时, 才需要打开 `insecure_skip_verify`.

### 系统通知

| 平台 | 使用的机制 | 依赖 |
| --- | --- | --- |
| macOS | `osascript` 调用通知中心 | 系统自带 |
| Linux | `notify-send`, 缺失时退回 `zenity` / `kdialog` | 需安装 libnotify-bin 等 |
| Windows | PowerShell 调用 WinRT Toast 通知 | 系统自带 PowerShell |

`rainmail test system` 会弹一条测试通知, 可以据此确认平台支持情况.

### 去重与免打扰

同一地点, 同一降水类型, 同一个起始小时视为同一场降水. 提醒发出后写入状态文件
(`~/.local/state/rainmail/state.json`), 在 `repeat.cooldown` 内不会再次提醒.
判定结果变化或冷却结束时会重新提醒.

`notify.quiet_hours` 可以配置免打扰时段 (例如 `["23:30-07:00"]`, 支持跨零点),
默认只跳过桌面通知; 把 `notify.quiet_hours_skip_email` 打开则会连邮件一起跳过.

## 命令

```text
rainmail run              常驻轮询
rainmail check            立刻检查一次
rainmail test email       发送测试邮件
rainmail test system      弹出测试通知
rainmail config init      生成示例配置
rainmail config path      打印配置文件与状态文件路径
rainmail config show      打印生效配置
rainmail version          版本信息
```

全局参数: `--config`, `--log-level`, `--log-format`, `--verbose`.

`rainmail check` 的常用开关: `--dry-run` 只判定不发送, `--force` 忽略冷却强制发送,
`--channel` 限定通道, `--timeout` 设置本次检查的超时.

## 常驻与开机自启

`rainmail run` 自身就是常驻进程, 也可以交给系统服务管理器托管.

Linux (systemd user unit, `~/.config/systemd/user/rainmail.service`):

```ini
[Unit]
Description=rainmail 降雨提醒
After=network-online.target

[Service]
ExecStart=%h/.local/bin/rainmail run
Restart=on-failure
RestartSec=30

[Install]
WantedBy=default.target
```

```shell
systemctl --user daemon-reload
systemctl --user enable --now rainmail
```

macOS (launchd, `~/Library/LaunchAgents/com.example.rainmail.plist`):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.example.rainmail</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/rainmail</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

```shell
launchctl load ~/Library/LaunchAgents/com.example.rainmail.plist
```

Windows 可以用任务计划程序创建"登录时触发"的任务, 程序填 `rainmail.exe`, 参数填 `run`,
并勾选"如果任务失败, 按以下频率重新启动".

## 目录结构

```text
main.go             程序入口
internal/cli        命令行定义
internal/app        业务流程编排
internal/config     配置结构, 校验与版本迁移
internal/weather    天气接口抽象与各接口实现
internal/notify     提醒通道抽象, 邮件与系统通知
internal/report     提醒文案渲染
internal/state      去重与冷却所需的状态持久化
internal/logx       日志设施
```

## 开发

```shell
just test    # 单元测试
just lint    # go vet
```

## 许可

MIT
