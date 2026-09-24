package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// qWeather 对接和风天气逐小时预报接口.
// 文档: https://dev.qweather.com/docs/api/weather/weather-hourly-forecast/
//
// 注意 location 参数的顺序是 "经度,纬度".
type qWeather struct {
	baseURL  string
	apiKey   string
	language string
	client   *http.Client
}

func newQWeather(opts Options, client *http.Client) *qWeather {
	return &qWeather{
		baseURL:  orDefault(opts.BaseURL, "https://devapi.qweather.com/v7/weather/24h"),
		apiKey:   opts.APIKey,
		language: orDefault(opts.Language, "zh"),
		client:   client,
	}
}

func (p *qWeather) Name() string { return "qweather" }

func (p *qWeather) Hourly(ctx context.Context, req Request) (*Forecast, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("qweather 需要配置 weather.providers.qweather.api_key")
	}

	query := url.Values{}
	query.Set("location", fmt.Sprintf("%.4f,%.4f", req.Longitude, req.Latitude))
	query.Set("key", p.apiKey)
	query.Set("lang", p.language)

	body, err := requestJSON(ctx, p.client, http.MethodGet, p.baseURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Code   string `json:"code"`
		Hourly []struct {
			FxTime string `json:"fxTime"`
			Temp   string `json:"temp"`
			Icon   string `json:"icon"`
			Text   string `json:"text"`
			Pop    string `json:"pop"`
			Precip string `json:"precip"`
		} `json:"hourly"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析和风天气响应失败: %w", err)
	}
	if payload.Code != "" && payload.Code != "200" {
		return nil, fmt.Errorf("和风天气返回状态码 %s, 请检查 api_key 与接口地址", payload.Code)
	}
	if len(payload.Hourly) == 0 {
		return nil, fmt.Errorf("和风天气响应缺少逐小时数据")
	}

	tzName := req.Timezone
	loc := loadLocation(tzName)
	if loc == nil {
		loc = time.UTC
	}

	forecast := &Forecast{
		Provider:   p.Name(),
		CodeSystem: CodeQWeather,
		Location:   loc,
		Timezone:   orDefault(tzName, "UTC"),
		FetchedAt:  time.Now(),
	}

	for _, item := range payload.Hourly {
		ts, ok := parseTime(item.FxTime, time.RFC3339, loc)
		if !ok {
			continue
		}
		hour := Hour{Time: ts, Probability: -1, WeatherCode: -1, Description: item.Text}
		if value, ok := parseFloatLoose(item.Precip); ok {
			hour.PrecipitationMM = value
		}
		if value, ok := parseIntLoose(item.Pop); ok {
			hour.Probability = value
		}
		if value, ok := parseIntLoose(item.Icon); ok {
			hour.WeatherCode = value
		}
		if value, ok := parseFloatLoose(item.Temp); ok {
			hour.TemperatureC = value
			hour.HasTemperature = true
		}
		forecast.Hours = append(forecast.Hours, hour)
	}

	if len(forecast.Hours) == 0 {
		return nil, fmt.Errorf("和风天气响应没有可解析的逐小时数据")
	}
	return forecast, nil
}
