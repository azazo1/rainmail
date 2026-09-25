package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// openWeatherMap 对接 OpenWeatherMap 5 天 / 3 小时预报接口.
// 文档: https://openweathermap.org/forecast5
//
// 该接口的降水粒度是 3 小时, 这里会平摊为 3 个逐小时点, 保证上层逻辑只需处理小时序列.
type openWeatherMap struct {
	baseURL  string
	apiKey   string
	language string
	units    string
	client   *http.Client
}

func newOpenWeatherMap(opts Options, client *http.Client) *openWeatherMap {
	return &openWeatherMap{
		baseURL:  orDefault(opts.BaseURL, "https://api.openweathermap.org/data/2.5/forecast"),
		apiKey:   opts.APIKey,
		language: orDefault(opts.Language, "zh_cn"),
		units:    orDefault(opts.Units, "metric"),
		client:   client,
	}
}

func (p *openWeatherMap) Name() string { return "openweathermap" }

func (p *openWeatherMap) Hourly(ctx context.Context, req Request) (*Forecast, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("openweathermap 需要配置 weather.providers.openweathermap.api_key")
	}

	query := url.Values{}
	query.Set("lat", strconv.FormatFloat(req.Latitude, 'f', 4, 64))
	query.Set("lon", strconv.FormatFloat(req.Longitude, 'f', 4, 64))
	query.Set("appid", p.apiKey)
	query.Set("units", p.units)
	query.Set("lang", p.language)

	body, err := requestJSON(ctx, p.client, http.MethodGet, p.baseURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var payload struct {
		City struct {
			Name     string `json:"name"`
			Timezone int    `json:"timezone"`
		} `json:"city"`
		List []struct {
			DT   int64 `json:"dt"`
			Main struct {
				Temp float64 `json:"temp"`
			} `json:"main"`
			Weather []struct {
				ID          int    `json:"id"`
				Description string `json:"description"`
			} `json:"weather"`
			Rain *struct {
				ThreeH float64 `json:"3h"`
			} `json:"rain"`
			Snow *struct {
				ThreeH float64 `json:"3h"`
			} `json:"snow"`
			Pop float64 `json:"pop"`
		} `json:"list"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析 openweathermap 响应失败: %w", err)
	}
	if len(payload.List) == 0 {
		return nil, fmt.Errorf("openweathermap 响应缺少预报数据")
	}

	loc := loadLocation(req.Timezone)
	if loc == nil {
		loc = time.FixedZone("OWM", payload.City.Timezone)
	}

	forecast := &Forecast{
		Provider:   p.Name(),
		CodeSystem: CodeOWM,
		Location:   loc,
		Timezone:   orDefault(req.Timezone, loc.String()),
		FetchedAt:  time.Now(),
	}

	for _, item := range payload.List {
		// rain 与 snow 是 3 小时累计量, 平摊到小时才能与阈值比较.
		var total float64
		if item.Rain != nil {
			total += item.Rain.ThreeH
		}
		if item.Snow != nil {
			total += item.Snow.ThreeH
		}

		code := -1
		description := ""
		if len(item.Weather) > 0 {
			code = item.Weather[0].ID
			description = item.Weather[0].Description
		}

		probability := int(math.Round(item.Pop * 100))
		if probability < 0 {
			probability = -1
		}

		slot := time.Unix(item.DT, 0).In(loc)
		for offset := 0; offset < 3; offset++ {
			forecast.Hours = append(forecast.Hours, Hour{
				Time:            slot.Add(time.Duration(offset) * time.Hour),
				PrecipitationMM: total / 3,
				Probability:     probability,
				WeatherCode:     code,
				TemperatureC:    item.Main.Temp,
				HasTemperature:  true,
				Description:     description,
			})
		}
	}

	return forecast, nil
}
