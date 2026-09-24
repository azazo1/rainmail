package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// seniverse 对接心知天气逐小时预报接口.
// 文档: https://seniverse.yuque.com/books/share/9c6b0f1b (天气现象代码表)
//
// 注意 location 参数的顺序是 "纬度:经度".
type seniverse struct {
	baseURL  string
	apiKey   string
	language string
	units    string
	client   *http.Client
}

func newSeniverse(opts Options, client *http.Client) *seniverse {
	return &seniverse{
		baseURL:  orDefault(opts.BaseURL, "https://api.seniverse.com/v3/weather/hourly.json"),
		apiKey:   opts.APIKey,
		language: orDefault(opts.Language, "zh-Hans"),
		units:    orDefault(opts.Units, "c"),
		client:   client,
	}
}

func (p *seniverse) Name() string { return "seniverse" }

func (p *seniverse) Hourly(ctx context.Context, req Request) (*Forecast, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("seniverse 需要配置 weather.providers.seniverse.api_key")
	}

	query := url.Values{}
	query.Set("key", p.apiKey)
	query.Set("location", fmt.Sprintf("%.4f:%.4f", req.Latitude, req.Longitude))
	query.Set("language", p.language)
	query.Set("unit", p.units)

	body, err := requestJSON(ctx, p.client, http.MethodGet, p.baseURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Results []struct {
			Location struct {
				Name     string `json:"name"`
				Timezone string `json:"timezone"`
			} `json:"location"`
			Hourly []struct {
				Time          string `json:"time"`
				Text          string `json:"text"`
				Code          string `json:"code"`
				Temperature   string `json:"temperature"`
				Precipitation string `json:"precipitation"`
			} `json:"hourly"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析心知天气响应失败: %w", err)
	}
	if len(payload.Results) == 0 || len(payload.Results[0].Hourly) == 0 {
		return nil, fmt.Errorf("心知天气响应缺少逐小时数据, 请检查 api_key 与 location 参数")
	}

	result := payload.Results[0]
	tzName := orDefault(req.Timezone, result.Location.Timezone)
	loc := loadLocation(tzName)
	if loc == nil {
		loc = time.UTC
	}

	forecast := &Forecast{
		Provider:   p.Name(),
		CodeSystem: CodeSeniverse,
		Location:   loc,
		Timezone:   orDefault(tzName, "UTC"),
		FetchedAt:  time.Now(),
	}

	for _, item := range result.Hourly {
		ts, ok := parseTime(item.Time, time.RFC3339, loc)
		if !ok {
			continue
		}
		hour := Hour{Time: ts, Probability: -1, WeatherCode: -1, Description: item.Text}
		if value, ok := parseFloatLoose(item.Precipitation); ok {
			hour.PrecipitationMM = value
		}
		if value, ok := parseIntLoose(item.Code); ok {
			hour.WeatherCode = value
		}
		if value, ok := parseFloatLoose(item.Temperature); ok {
			hour.TemperatureC = value
			hour.HasTemperature = true
		}
		forecast.Hours = append(forecast.Hours, hour)
	}

	if len(forecast.Hours) == 0 {
		return nil, fmt.Errorf("心知天气响应没有可解析的逐小时数据")
	}
	return forecast, nil
}
