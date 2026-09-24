package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomProviderParallelArrays(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("lat"); got != "31.2300" {
			t.Errorf("lat 占位符未替换, 实际为 %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"hourly": {
				"time": ["2026-09-24T18:00", "2026-09-24T19:00"],
				"precipitation": [0.4, 0],
				"precipitation_probability": [80, 20],
				"weather_code": [61, 3],
				"temperature_2m": [21.5, 20.8]
			}
		}`))
	}))
	defer server.Close()

	provider, err := New(Options{
		Provider:          "custom",
		URL:               server.URL + "/weather?lat={lat}&lon={lon}",
		TimePath:          "hourly.time",
		TimeFormat:        "2006-01-02T15:04",
		PrecipitationPath: "hourly.precipitation",
		ProbabilityPath:   "hourly.precipitation_probability",
		WeatherCodePath:   "hourly.weather_code",
		TemperaturePath:   "hourly.temperature_2m",
		CodeSystem:        "wmo",
	}, server.Client())
	if err != nil {
		t.Fatalf("构造 custom 接口失败: %v", err)
	}

	forecast, err := provider.Hourly(context.Background(), Request{
		Latitude: 31.23, Longitude: 121.47, Timezone: "Asia/Shanghai", Hours: 6,
	})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}

	if forecast.CodeSystem != CodeWMO {
		t.Fatalf("码表应为 wmo, 实际为 %s", forecast.CodeSystem)
	}
	if len(forecast.Hours) != 2 {
		t.Fatalf("应解析出 2 个小时, 实际为 %d", len(forecast.Hours))
	}

	first := forecast.Hours[0]
	if first.PrecipitationMM != 0.4 {
		t.Fatalf("降水量应为 0.4, 实际为 %v", first.PrecipitationMM)
	}
	if first.Probability != 80 {
		t.Fatalf("概率应为 80, 实际为 %d", first.Probability)
	}
	if first.WeatherCode != 61 {
		t.Fatalf("天气码应为 61, 实际为 %d", first.WeatherCode)
	}
	if !first.HasTemperature || first.TemperatureC != 21.5 {
		t.Fatalf("温度解析不正确: %+v", first)
	}
	if first.Description != "小雨" {
		t.Fatalf("天气描述应为小雨, 实际为 %q", first.Description)
	}
	if first.Time.Hour() != 18 {
		t.Fatalf("时间应为 18 点, 实际为 %s", first.Time)
	}
}

func TestCustomProviderItemArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"list": [
				{"dt": "2026-09-24 18:00:00", "rain": 1.2, "pop": 70},
				{"dt": "2026-09-24 21:00:00", "rain": 0, "pop": 10}
			]
		}`))
	}))
	defer server.Close()

	provider, err := New(Options{
		Provider:          "custom",
		URL:               server.URL,
		ItemsPath:         "list",
		TimePath:          "dt",
		TimeFormat:        "2006-01-02 15:04:05",
		PrecipitationPath: "rain",
		ProbabilityPath:   "pop",
	}, server.Client())
	if err != nil {
		t.Fatalf("构造 custom 接口失败: %v", err)
	}

	forecast, err := provider.Hourly(context.Background(), Request{
		Latitude: 31.23, Longitude: 121.47, Timezone: "Asia/Shanghai", Hours: 6,
	})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(forecast.Hours) != 2 {
		t.Fatalf("应解析出 2 个小时, 实际为 %d", len(forecast.Hours))
	}
	if forecast.Hours[0].PrecipitationMM != 1.2 {
		t.Fatalf("降水量应为 1.2, 实际为 %v", forecast.Hours[0].PrecipitationMM)
	}
	if forecast.Hours[0].Probability != 70 {
		t.Fatalf("概率应为 70, 实际为 %d", forecast.Hours[0].Probability)
	}
	if forecast.CodeSystem != CodeNone {
		t.Fatalf("未配置 code_system 时应为 none, 实际为 %s", forecast.CodeSystem)
	}
}

func TestCustomProviderRequiresConfiguration(t *testing.T) {
	if _, err := New(Options{Provider: "custom"}, nil); err == nil {
		t.Fatal("缺少 url 时应报错")
	}
	if _, err := New(Options{Provider: "custom", URL: "https://example.com"}, nil); err == nil {
		t.Fatal("缺少 time_path 时应报错")
	}
}

func TestProvidersRequireAPIKey(t *testing.T) {
	cases := []Options{
		{Provider: "qweather"},
		{Provider: "seniverse"},
		{Provider: "openweathermap"},
	}

	for _, opts := range cases {
		provider, err := New(opts, nil)
		if err != nil {
			t.Fatalf("构造 %s 失败: %v", opts.Provider, err)
		}
		if _, err := provider.Hourly(context.Background(), Request{Latitude: 1, Longitude: 1}); err == nil {
			t.Fatalf("%s 缺少 api_key 时应报错", opts.Provider)
		}
	}
}
