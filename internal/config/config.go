// Package config 定义 rainmail 的配置结构, 默认值, 校验规则与加载流程.
//
// 配置文件使用 TOML 格式, 顶层 config_version 字段标识结构版本.
// 当结构版本落后于 CurrentVersion 时, 会交由 internal/config/migration 完成迁移,
// 迁移前原始文件会备份为 <config>.vN.bak.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CurrentVersion 是当前配置文件结构版本.
// 只要字段语义或层次发生变化就必须递增, 并在 migration 包登记对应的迁移步骤.
const CurrentVersion = 1

// 支持的天气接口标识.
const (
	ProviderOpenMeteo      = "open-meteo"
	ProviderOpenWeatherMap = "openweathermap"
	ProviderQWeather       = "qweather"
	ProviderSeniverse      = "seniverse"
	ProviderCustom         = "custom"
)

// 支持的 SMTP 加密方式.
const (
	EncryptionTLS      = "tls"
	EncryptionStartTLS = "starttls"
	EncryptionNone     = "none"
)

// Config 是 rainmail 的完整配置.
type Config struct {
	ConfigVersion int      `toml:"config_version"`
	Location      Location `toml:"location"`
	Weather       Weather  `toml:"weather"`
	Notify        Notify   `toml:"notify"`
	Repeat        Repeat   `toml:"repeat"`
	Log           Log      `toml:"log"`
}

// Location 描述关注的地点.
type Location struct {
	Name      string  `toml:"name"`
	Latitude  float64 `toml:"latitude"`
	Longitude float64 `toml:"longitude"`
	// Timezone 为 IANA 名称, 留空表示使用天气接口返回的时区.
	Timezone string `toml:"timezone"`
}

// Weather 描述天气查询行为.
type Weather struct {
	// Provider 决定使用 weather.providers 中的哪一组连接参数.
	Provider string `toml:"provider"`
	// LookaheadHours 是向后观察的小时数.
	LookaheadHours int `toml:"lookahead_hours"`
	// CheckInterval 是常驻模式的轮询间隔.
	CheckInterval Duration `toml:"check_interval"`
	// MinPrecipitationMM 是判定确定降水的小时降水量阈值.
	MinPrecipitationMM float64 `toml:"min_precipitation_mm"`
	// MinProbability 是判定可能降水的降水概率阈值.
	MinProbability int `toml:"min_probability"`
	// IncludeSnow 决定降雪是否也触发提醒.
	IncludeSnow bool     `toml:"include_snow"`
	Timeout     Duration `toml:"timeout"`
	// Providers 以接口标识为键保存各自的连接参数.
	Providers map[string]ProviderSettings `toml:"providers"`
}

// ProviderSettings 承载所有天气接口的连接参数.
// 不同接口只使用其中相关的字段, 详见 config.example.toml 的说明.
type ProviderSettings struct {
	BaseURL  string `toml:"base_url"`
	APIKey   string `toml:"api_key"`
	Language string `toml:"language"`
	Units    string `toml:"units"`

	// 以下字段供 custom 接口使用.
	URL        string            `toml:"url"`
	Method     string            `toml:"method"`
	Headers    map[string]string `toml:"headers"`
	ItemsPath  string            `toml:"items_path"`
	TimePath   string            `toml:"time_path"`
	TimeFormat string            `toml:"time_format"`
	CodeSystem string            `toml:"code_system"`

	PrecipitationPath string `toml:"precipitation_path"`
	ProbabilityPath   string `toml:"probability_path"`
	WeatherCodePath   string `toml:"weather_code_path"`
	TemperaturePath   string `toml:"temperature_path"`
}

// Notify 描述提醒策略.
type Notify struct {
	// NotifyOnPossible 决定概率达标但降水量未达标时是否提醒.
	NotifyOnPossible bool `toml:"notify_on_possible"`
	// QuietHours 是免打扰时段列表, 例如 ["23:30-07:00"].
	QuietHours []string `toml:"quiet_hours"`
	// QuietHoursSkipEmail 决定免打扰时段内是否连邮件一起跳过.
	QuietHoursSkipEmail bool   `toml:"quiet_hours_skip_email"`
	Email               Email  `toml:"email"`
	System              System `toml:"system"`
}

// Email 描述 SMTP 邮件通道.
type Email struct {
	Enabled            bool     `toml:"enabled"`
	Host               string   `toml:"host"`
	Port               int      `toml:"port"`
	Encryption         string   `toml:"encryption"`
	Username           string   `toml:"username"`
	Password           string   `toml:"password"`
	From               string   `toml:"from"`
	FromName           string   `toml:"from_name"`
	To                 []string `toml:"to"`
	SubjectPrefix      string   `toml:"subject_prefix"`
	InsecureSkipVerify bool     `toml:"insecure_skip_verify"`
	Timeout            Duration `toml:"timeout"`
}

// System 描述桌面通知通道.
type System struct {
	Enabled bool `toml:"enabled"`
	Sound   bool `toml:"sound"`
}

// Repeat 描述重复提醒的抑制策略.
type Repeat struct {
	// Cooldown 是同一场降水的提醒冷却时间.
	Cooldown Duration `toml:"cooldown"`
	// StateFile 留空表示使用配置文件同目录的 state.json.
	StateFile string `toml:"state_file"`
}

// Log 描述日志输出.
type Log struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
	File   string `toml:"file"`
}

// Duration 是支持 "30m" / "3h" 这类写法的时长类型.
type Duration time.Duration

// Duration 返回标准库时长.
func (d Duration) Duration() time.Duration { return time.Duration(d) }

// String 返回 "1h30m0s" 形式的文本.
func (d Duration) String() string { return time.Duration(d).String() }

// UnmarshalTOML 解析 TOML 中的时长, 支持字符串与秒数.
func (d *Duration) UnmarshalTOML(v any) error {
	switch t := v.(type) {
	case string:
		parsed, err := time.ParseDuration(strings.TrimSpace(t))
		if err != nil {
			return fmt.Errorf("时长 %q 无法解析, 请使用 30m / 3h 这类写法: %w", t, err)
		}
		*d = Duration(parsed)
	case int64:
		*d = Duration(time.Duration(t) * time.Second)
	case float64:
		*d = Duration(time.Duration(t * float64(time.Second)))
	default:
		return fmt.Errorf("时长必须是字符串或秒数, 收到 %T", v)
	}
	return nil
}

// MarshalTOML 输出带引号的时长文本.
func (d Duration) MarshalTOML() ([]byte, error) {
	return []byte(strconv.Quote(time.Duration(d).String())), nil
}

// Default 返回一份带有合理默认值的配置.
func Default() *Config {
	return &Config{
		ConfigVersion: CurrentVersion,
		Location: Location{
			Name:      "上海",
			Latitude:  31.2304,
			Longitude: 121.4737,
			Timezone:  "Asia/Shanghai",
		},
		Weather: Weather{
			Provider:           ProviderOpenMeteo,
			LookaheadHours:     12,
			CheckInterval:      Duration(30 * time.Minute),
			MinPrecipitationMM: 0.1,
			MinProbability:     60,
			Timeout:            Duration(15 * time.Second),
			Providers: map[string]ProviderSettings{
				ProviderOpenMeteo: {
					BaseURL: "https://api.open-meteo.com/v1/forecast",
				},
				ProviderOpenWeatherMap: {
					BaseURL:  "https://api.openweathermap.org/data/2.5/forecast",
					Language: "zh_cn",
					Units:    "metric",
				},
				ProviderQWeather: {
					BaseURL:  "https://devapi.qweather.com/v7/weather/24h",
					Language: "zh",
				},
				ProviderSeniverse: {
					BaseURL:  "https://api.seniverse.com/v3/weather/hourly.json",
					Language: "zh-Hans",
					Units:    "c",
				},
				ProviderCustom: {
					Method:     "GET",
					TimeFormat: "2006-01-02T15:04",
				},
			},
		},
		Notify: Notify{
			NotifyOnPossible: true,
			Email: Email{
				Port:          465,
				Encryption:    EncryptionTLS,
				SubjectPrefix: "[rainmail]",
				Timeout:       Duration(20 * time.Second),
			},
			System: System{Enabled: true},
		},
		Repeat: Repeat{Cooldown: Duration(3 * time.Hour)},
		Log:    Log{Level: "info", Format: "text"},
	}
}

// Normalize 补齐可以推导的字段, 不覆盖用户显式设置的值.
func (c *Config) Normalize() {
	if c.Weather.Provider == "" {
		c.Weather.Provider = ProviderOpenMeteo
	}
	if c.Weather.Providers == nil {
		c.Weather.Providers = map[string]ProviderSettings{}
	}
	defaults := Default().Weather.Providers
	for name, want := range defaults {
		got, ok := c.Weather.Providers[name]
		if !ok {
			c.Weather.Providers[name] = want
			continue
		}
		if got.BaseURL == "" {
			got.BaseURL = want.BaseURL
		}
		if got.Language == "" {
			got.Language = want.Language
		}
		if got.Units == "" {
			got.Units = want.Units
		}
		if got.Method == "" {
			got.Method = want.Method
		}
		if got.TimeFormat == "" {
			got.TimeFormat = want.TimeFormat
		}
		c.Weather.Providers[name] = got
	}

	if c.Weather.Timeout == 0 {
		c.Weather.Timeout = Duration(15 * time.Second)
	}
	if c.Weather.CheckInterval == 0 {
		c.Weather.CheckInterval = Duration(30 * time.Minute)
	}

	if c.Notify.Email.Port == 0 {
		c.Notify.Email.Port = 465
	}
	if c.Notify.Email.Encryption == "" {
		c.Notify.Email.Encryption = EncryptionForPort(c.Notify.Email.Port)
	}
	if c.Notify.Email.SubjectPrefix == "" {
		c.Notify.Email.SubjectPrefix = "[rainmail]"
	}
	if c.Notify.Email.From == "" {
		c.Notify.Email.From = c.Notify.Email.Username
	}
	if c.Notify.Email.Timeout == 0 {
		c.Notify.Email.Timeout = Duration(20 * time.Second)
	}
}

// EncryptionForPort 依据端口推断加密方式.
func EncryptionForPort(port int) string {
	switch port {
	case 465:
		return EncryptionTLS
	case 587, 25:
		return EncryptionStartTLS
	default:
		return EncryptionTLS
	}
}

// Validate 校验配置, 一次性返回所有问题.
func (c *Config) Validate() error {
	var errs []error

	if c.ConfigVersion != CurrentVersion {
		errs = append(errs, fmt.Errorf("config_version 应为 %d, 实际为 %d", CurrentVersion, c.ConfigVersion))
	}
	if c.Location.Latitude < -90 || c.Location.Latitude > 90 {
		errs = append(errs, fmt.Errorf("location.latitude 必须在 -90 到 90 之间, 实际为 %g", c.Location.Latitude))
	}
	if c.Location.Longitude < -180 || c.Location.Longitude > 180 {
		errs = append(errs, fmt.Errorf("location.longitude 必须在 -180 到 180 之间, 实际为 %g", c.Location.Longitude))
	}
	if c.Location.Timezone != "" {
		if _, err := time.LoadLocation(c.Location.Timezone); err != nil {
			errs = append(errs, fmt.Errorf("location.timezone %q 无法识别: %w", c.Location.Timezone, err))
		}
	}

	switch c.Weather.Provider {
	case ProviderOpenMeteo, ProviderOpenWeatherMap, ProviderQWeather, ProviderSeniverse, ProviderCustom:
	case "":
		errs = append(errs, errors.New("weather.provider 不能为空"))
	default:
		errs = append(errs, fmt.Errorf("weather.provider %q 不受支持, 可选 %s",
			c.Weather.Provider, strings.Join([]string{
				ProviderOpenMeteo, ProviderOpenWeatherMap, ProviderQWeather, ProviderSeniverse, ProviderCustom,
			}, " / ")))
	}
	if c.Weather.LookaheadHours <= 0 || c.Weather.LookaheadHours > 168 {
		errs = append(errs, fmt.Errorf("weather.lookahead_hours 必须在 1 到 168 之间, 实际为 %d", c.Weather.LookaheadHours))
	}
	if c.Weather.MinPrecipitationMM < 0 {
		errs = append(errs, fmt.Errorf("weather.min_precipitation_mm 不能为负数, 实际为 %g", c.Weather.MinPrecipitationMM))
	}
	if c.Weather.MinProbability < 0 || c.Weather.MinProbability > 100 {
		errs = append(errs, fmt.Errorf("weather.min_probability 必须在 0 到 100 之间, 实际为 %d", c.Weather.MinProbability))
	}
	if c.Weather.CheckInterval < 0 {
		errs = append(errs, fmt.Errorf("weather.check_interval 不能为负数, 实际为 %s", c.Weather.CheckInterval))
	}

	for _, window := range c.Notify.QuietHours {
		if _, err := ParseQuietWindow(window); err != nil {
			errs = append(errs, err)
		}
	}
	if c.Notify.Email.Enabled {
		errs = append(errs, c.validateEmail()...)
	}
	if !c.Notify.Email.Enabled && !c.Notify.System.Enabled {
		errs = append(errs, errors.New("notify.email 与 notify.system 至少需要启用一个"))
	}
	if c.Repeat.Cooldown < 0 {
		errs = append(errs, fmt.Errorf("repeat.cooldown 不能为负数, 实际为 %s", c.Repeat.Cooldown))
	}

	switch strings.ToLower(strings.TrimSpace(c.Log.Level)) {
	case "", "debug", "trace", "info", "warn", "warning", "error":
	default:
		errs = append(errs, fmt.Errorf("log.level %q 不受支持, 可选 debug / info / warn / error", c.Log.Level))
	}
	switch strings.ToLower(strings.TrimSpace(c.Log.Format)) {
	case "", "text", "json":
	default:
		errs = append(errs, fmt.Errorf("log.format %q 不受支持, 可选 text / json", c.Log.Format))
	}

	return errors.Join(errs...)
}

func (c *Config) validateEmail() []error {
	var errs []error
	e := c.Notify.Email

	if e.Host == "" {
		errs = append(errs, errors.New("notify.email.host 不能为空"))
	}
	if e.Port < 1 || e.Port > 65535 {
		errs = append(errs, fmt.Errorf("notify.email.port 必须在 1 到 65535 之间, 实际为 %d", e.Port))
	}
	switch e.Encryption {
	case EncryptionTLS, EncryptionStartTLS, EncryptionNone:
	default:
		errs = append(errs, fmt.Errorf("notify.email.encryption %q 不受支持, 可选 tls / starttls / none", e.Encryption))
	}
	if len(e.To) == 0 {
		errs = append(errs, errors.New("notify.email.to 至少需要一个收件地址"))
	}
	if e.From == "" {
		errs = append(errs, errors.New("notify.email.from 不能为空"))
	}
	if e.Encryption != EncryptionNone && e.Host != "" && strings.HasPrefix(e.Host, "127.0.0.1") && e.InsecureSkipVerify {
		errs = append(errs, errors.New("notify.email.insecure_skip_verify 仅在自签名服务器上开启"))
	}
	return errs
}

// StateDir 返回运行状态目录.
//
// 三个平台统一为 ~/.local/state/rainmail, 与配置分离, 状态丢失只会导致重复提醒一次.
func StateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法确定用户主目录, 请用 repeat.state_file 指定状态文件路径: %w", err)
	}
	return filepath.Join(home, ".local", "state", "rainmail"), nil
}

// DefaultStatePath 返回未指定 state_file 时的状态文件路径.
func DefaultStatePath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// StatePath 返回状态文件路径, state_file 留空时使用 DefaultStatePath.
func (c *Config) StatePath() (string, error) {
	if c.Repeat.StateFile != "" {
		return c.Repeat.StateFile, nil
	}
	return DefaultStatePath()
}

// MaskSecrets 返回把敏感字段打码后的副本, 用于展示.
func (c *Config) MaskSecrets() *Config {
	out := *c

	out.Notify.Email.Password = maskValue(c.Notify.Email.Password)
	if len(c.Notify.Email.To) > 0 {
		out.Notify.Email.To = append([]string(nil), c.Notify.Email.To...)
	}

	providers := make(map[string]ProviderSettings, len(c.Weather.Providers))
	for name, p := range c.Weather.Providers {
		p.APIKey = maskValue(p.APIKey)
		if len(p.Headers) > 0 {
			headers := make(map[string]string, len(p.Headers))
			for k, v := range p.Headers {
				headers[k] = maskHeader(k, v)
			}
			p.Headers = headers
		}
		providers[name] = p
	}
	out.Weather.Providers = providers

	return &out
}

func maskValue(v string) string {
	if v == "" {
		return ""
	}
	return "******"
}

func maskHeader(key, value string) string {
	lk := strings.ToLower(key)
	for _, hint := range []string{"authorization", "token", "key", "secret", "password"} {
		if strings.Contains(lk, hint) {
			return maskValue(value)
		}
	}
	return value
}
