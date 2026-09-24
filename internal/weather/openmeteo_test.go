package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenMeteoParsesHourly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("hourly") == "" {
			t.Error("请求缺少 hourly 参数")
		}
		if query.Get("timezone") != "Asia/Shanghai" {
			t.Errorf("时区参数应为 Asia/Shanghai, 实际为 %q", query.Get("timezone"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"timezone": "Asia/Shanghai",
			"hourly": {
				"time": ["2026-09-24T18:00", "2026-09-24T19:00"],
				"precipitation": [1.8, 0],
				"precipitation_probability": [75, 5],
				"weather_code": [63, 1],
				"temperature_2m": [22.4, 22.0]
			}
		}`))
	}))
	defer server.Close()

	provider := newOpenMeteo(Options{BaseURL: server.URL}, server.Client())

	forecast, err := provider.Hourly(context.Background(), Request{
		Latitude: 31.2304, Longitude: 121.4737, Timezone: "Asia/Shanghai", Hours: 12,
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
	if first.PrecipitationMM != 1.8 || first.Probability != 75 || first.WeatherCode != 63 {
		t.Fatalf("首个小时解析不正确: %+v", first)
	}
	if first.Description != "中雨" {
		t.Fatalf("天气描述应为中雨, 实际为 %q", first.Description)
	}
	if forecast.Location == nil || forecast.Location.String() != "Asia/Shanghai" {
		t.Fatalf("时区未正确解析: %+v", forecast.Location)
	}
	if hour := first.Time.In(forecast.Location).Hour(); hour != 18 {
		t.Fatalf("时间应为 18 点, 实际为 %d", hour)
	}
}

func TestOpenMeteoRejectsBadPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"hourly": {"time": []}}`))
	}))
	defer server.Close()

	provider := newOpenMeteo(Options{BaseURL: server.URL}, server.Client())
	if _, err := provider.Hourly(context.Background(), Request{Latitude: 1, Longitude: 1}); err == nil {
		t.Fatal("缺少逐小时数据时应报错")
	}
}

func TestRedactURLCoversSecrets(t *testing.T) {
	got := redactURL("https://api.example.com/v1?lat=1&key=secret-value&appid=another")
	for _, leaked := range []string{"secret-value", "another"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("密钥不应出现在日志地址中: %s", got)
		}
	}
}
