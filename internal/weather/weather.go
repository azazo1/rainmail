// Package weather 定义天气数据模型, 接口抽象与降水评估规则.
//
// 所有内置接口都会把各自的响应归一化成 Forecast(逐小时序列),
// 上层逻辑只依赖 Forecast, 因此新增接口只需要实现 Provider.
package weather

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Kind 是降水类型.
type Kind string

// 降水类型取值.
const (
	KindNone    Kind = "none"
	KindRain    Kind = "rain"
	KindSnow    Kind = "snow"
	KindMixed   Kind = "mixed"
	KindUnknown Kind = "unknown"
)

// Level 是降水信号的置信级别.
type Level int

// 置信级别取值.
const (
	LevelNone Level = iota
	LevelPossible
	LevelCertain
)

// String 返回级别名.
func (l Level) String() string {
	switch l {
	case LevelPossible:
		return "possible"
	case LevelCertain:
		return "certain"
	default:
		return "none"
	}
}

// CodeSystem 标识天气码的体系, 不同接口的码值含义不同.
type CodeSystem string

// 天气码体系取值.
const (
	CodeNone      CodeSystem = "none"
	CodeWMO       CodeSystem = "wmo"
	CodeOWM       CodeSystem = "owm"
	CodeQWeather  CodeSystem = "qweather"
	CodeSeniverse CodeSystem = "seniverse"
)

// Hour 是逐小时预报中的一个点.
type Hour struct {
	Time            time.Time
	PrecipitationMM float64
	// Probability 为降水概率百分数, -1 表示接口未提供.
	Probability int
	// WeatherCode 为接口原始天气码, -1 表示接口未提供.
	WeatherCode    int
	TemperatureC   float64
	HasTemperature bool
	// Description 为接口给出的自然语言描述, 可能为空.
	Description string
}

// Forecast 是一次查询的归一化结果.
type Forecast struct {
	Provider   string
	CodeSystem CodeSystem
	Location   *time.Location
	Timezone   string
	FetchedAt  time.Time
	Hours      []Hour
}

// Request 描述一次天气查询.
type Request struct {
	LocationName string
	Latitude     float64
	Longitude    float64
	Timezone     string
	Hours        int
}

// Provider 是天气接口抽象.
type Provider interface {
	// Name 返回接口标识, 与配置中的 weather.provider 对应.
	Name() string
	// Hourly 拉取逐小时预报.
	Hourly(ctx context.Context, req Request) (*Forecast, error)
}

// Options 是构造接口实现所需的连接参数, 由配置层转换而来.
type Options struct {
	Provider string
	BaseURL  string
	APIKey   string
	Language string
	Units    string

	// 以下字段供 custom 接口使用.
	URL               string
	Method            string
	Headers           map[string]string
	ItemsPath         string
	TimePath          string
	TimeFormat        string
	CodeSystem        string
	PrecipitationPath string
	ProbabilityPath   string
	WeatherCodePath   string
	TemperaturePath   string
}

// Names 列出内置接口标识.
func Names() []string {
	return []string{"open-meteo", "openweathermap", "qweather", "seniverse", "custom"}
}

// New 依据 opts.Provider 构造接口实现, client 为 nil 时使用默认客户端.
func New(opts Options, client *http.Client) (Provider, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	switch strings.ToLower(strings.TrimSpace(opts.Provider)) {
	case "open-meteo", "openmeteo":
		return newOpenMeteo(opts, client), nil
	case "openweathermap", "owm":
		return newOpenWeatherMap(opts, client), nil
	case "qweather", "heweather":
		return newQWeather(opts, client), nil
	case "seniverse", "thinkpage":
		return newSeniverse(opts, client), nil
	case "custom", "template":
		return newCustom(opts, client)
	case "":
		return nil, fmt.Errorf("未指定天气接口, 可选 %s", strings.Join(Names(), " / "))
	default:
		return nil, fmt.Errorf("未知天气接口 %q, 可选 %s", opts.Provider, strings.Join(Names(), " / "))
	}
}
