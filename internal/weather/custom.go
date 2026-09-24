package weather

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// customProvider 用 gjson 路径把任意 JSON 接口映射为逐小时预报.
//
// 支持两种响应形态:
//   - 并行数组: {"hourly": {"time": [...], "rain": [...]}}  -> 各 path 从响应根写起
//   - 对象数组: {"list": [{"dt": ..., "rain": ...}, ...]}    -> 配置 items_path, 各 path 相对元素书写
type customProvider struct {
	opts   Options
	client *http.Client
}

func newCustom(opts Options, client *http.Client) (*customProvider, error) {
	if strings.TrimSpace(opts.URL) == "" {
		return nil, fmt.Errorf("custom 接口需要配置 weather.providers.custom.url")
	}
	if strings.TrimSpace(opts.TimePath) == "" {
		return nil, fmt.Errorf("custom 接口需要配置 weather.providers.custom.time_path")
	}
	return &customProvider{opts: opts, client: client}, nil
}

func (p *customProvider) Name() string { return "custom" }

func (p *customProvider) Hourly(ctx context.Context, req Request) (*Forecast, error) {
	endpoint := strings.NewReplacer(
		"{lat}", strconv.FormatFloat(req.Latitude, 'f', 4, 64),
		"{lon}", strconv.FormatFloat(req.Longitude, 'f', 4, 64),
		"{timezone}", req.Timezone,
		"{hours}", strconv.Itoa(req.Hours),
	).Replace(p.opts.URL)

	body, err := requestJSON(ctx, p.client, orDefault(p.opts.Method, http.MethodGet), endpoint, p.opts.Headers)
	if err != nil {
		return nil, err
	}
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("custom 接口返回的内容不是合法 JSON")
	}

	loc := loadLocation(req.Timezone)
	if loc == nil {
		loc = time.UTC
	}

	forecast := &Forecast{
		Provider:   p.Name(),
		CodeSystem: parseCodeSystem(p.opts.CodeSystem),
		Location:   loc,
		Timezone:   orDefault(req.Timezone, "UTC"),
		FetchedAt:  time.Now(),
	}

	if p.opts.ItemsPath != "" {
		items := gjson.GetBytes(body, p.opts.ItemsPath).Array()
		if len(items) == 0 {
			return nil, fmt.Errorf("custom 接口的 items_path %q 没有匹配到数组", p.opts.ItemsPath)
		}
		for _, item := range items {
			if hour, ok := p.buildHour(item.Get, loc); ok {
				forecast.Hours = append(forecast.Hours, hour)
			}
		}
	} else {
		times := gjson.GetBytes(body, p.opts.TimePath).Array()
		if len(times) == 0 {
			return nil, fmt.Errorf("custom 接口的 time_path %q 没有匹配到数组", p.opts.TimePath)
		}
		for i := range times {
			index := strconv.Itoa(i)
			lookup := func(path string) gjson.Result {
				return gjson.GetBytes(body, path+"."+index)
			}
			if hour, ok := p.buildHour(lookup, loc); ok {
				forecast.Hours = append(forecast.Hours, hour)
			}
		}
	}

	if len(forecast.Hours) == 0 {
		return nil, fmt.Errorf("custom 接口没有解析出任何逐小时数据, 请检查各 path 配置")
	}
	return forecast, nil
}

// buildHour 借助 lookup 取值函数构造一个小时记录, 便于两种响应形态复用.
func (p *customProvider) buildHour(lookup func(path string) gjson.Result, loc *time.Location) (Hour, bool) {
	rawTime := lookup(p.opts.TimePath).String()
	ts, ok := parseTime(rawTime, p.opts.TimeFormat, loc)
	if !ok {
		return Hour{}, false
	}

	hour := Hour{Time: ts, Probability: -1, WeatherCode: -1}

	if path := p.opts.PrecipitationPath; path != "" {
		if result := lookup(path); result.Exists() {
			hour.PrecipitationMM = result.Float()
		}
	}
	if path := p.opts.ProbabilityPath; path != "" {
		if result := lookup(path); result.Exists() {
			hour.Probability = int(result.Float())
		}
	}
	if path := p.opts.WeatherCodePath; path != "" {
		if result := lookup(path); result.Exists() {
			hour.WeatherCode = int(result.Float())
			hour.Description = Describe(parseCodeSystem(p.opts.CodeSystem), hour.WeatherCode)
		}
	}
	if path := p.opts.TemperaturePath; path != "" {
		if result := lookup(path); result.Exists() {
			hour.TemperatureC = result.Float()
			hour.HasTemperature = true
		}
	}
	return hour, true
}

func parseCodeSystem(name string) CodeSystem {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "wmo", "open-meteo", "openmeteo":
		return CodeWMO
	case "owm", "openweathermap":
		return CodeOWM
	case "qweather", "heweather":
		return CodeQWeather
	case "seniverse", "thinkpage":
		return CodeSeniverse
	default:
		return CodeNone
	}
}
