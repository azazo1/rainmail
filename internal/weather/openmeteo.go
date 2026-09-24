package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// openMeteo 对接 Open-Meteo 免费接口, 无需密钥.
// 文档: https://open-meteo.com/en/docs
type openMeteo struct {
	baseURL string
	client  *http.Client
}

func newOpenMeteo(opts Options, client *http.Client) *openMeteo {
	return &openMeteo{
		baseURL: orDefault(opts.BaseURL, "https://api.open-meteo.com/v1/forecast"),
		client:  client,
	}
}

func (p *openMeteo) Name() string { return "open-meteo" }

func (p *openMeteo) Hourly(ctx context.Context, req Request) (*Forecast, error) {
	hours := req.Hours
	if hours <= 0 {
		hours = 24
	}
	days := hours/24 + 2
	if days > 16 {
		days = 16
	}

	query := url.Values{}
	query.Set("latitude", strconv.FormatFloat(req.Latitude, 'f', 4, 64))
	query.Set("longitude", strconv.FormatFloat(req.Longitude, 'f', 4, 64))
	query.Set("hourly", "precipitation,precipitation_probability,weather_code,temperature_2m")
	query.Set("forecast_days", strconv.Itoa(days))
	query.Set("timezone", orDefault(req.Timezone, "auto"))

	body, err := requestJSON(ctx, p.client, http.MethodGet, p.baseURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Timezone string `json:"timezone"`
		Hourly   struct {
			Time          []string  `json:"time"`
			Precipitation []float64 `json:"precipitation"`
			Probability   []int     `json:"precipitation_probability"`
			WeatherCode   []int     `json:"weather_code"`
			Temperature   []float64 `json:"temperature_2m"`
		} `json:"hourly"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析 open-meteo 响应失败: %w", err)
	}
	if len(payload.Hourly.Time) == 0 {
		return nil, fmt.Errorf("open-meteo 响应缺少逐小时数据")
	}

	tzName := orDefault(req.Timezone, payload.Timezone)
	loc := loadLocation(tzName)
	if loc == nil {
		loc = time.UTC
	}

	forecast := &Forecast{
		Provider:   p.Name(),
		CodeSystem: CodeWMO,
		Location:   loc,
		Timezone:   tzName,
		FetchedAt:  time.Now(),
	}

	for i, raw := range payload.Hourly.Time {
		ts, ok := parseTime(raw, "2006-01-02T15:04", loc)
		if !ok {
			continue
		}
		hour := Hour{Time: ts, Probability: -1, WeatherCode: -1}
		if i < len(payload.Hourly.Precipitation) {
			hour.PrecipitationMM = payload.Hourly.Precipitation[i]
		}
		if i < len(payload.Hourly.Probability) {
			hour.Probability = payload.Hourly.Probability[i]
		}
		if i < len(payload.Hourly.WeatherCode) {
			hour.WeatherCode = payload.Hourly.WeatherCode[i]
		}
		if i < len(payload.Hourly.Temperature) {
			hour.TemperatureC = payload.Hourly.Temperature[i]
			hour.HasTemperature = true
		}
		hour.Description = Describe(CodeWMO, hour.WeatherCode)
		forecast.Hours = append(forecast.Hours, hour)
	}

	if len(forecast.Hours) == 0 {
		return nil, fmt.Errorf("open-meteo 响应没有可解析的逐小时数据")
	}
	return forecast, nil
}
