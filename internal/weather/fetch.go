package weather

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// userAgent 让接口方能够识别调用来源.
const userAgent = "rainmail/0.1 (+https://github.com/azazo1/rainmail)"

// maxResponseBytes 限制单个响应的读取量, 防止异常接口拖垮内存.
const maxResponseBytes = 4 << 20

// requestJSON 发起一次请求并返回响应体, 非 2xx 状态码会返回带片段的错误.
func requestJSON(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string) ([]byte, error) {
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", redactURL(endpoint), err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("读取 %s 的响应失败: %w", redactURL(endpoint), err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("接口 %s 返回 %s: %s", redactURL(endpoint), resp.Status, snippet(body))
	}
	return body, nil
}

// redactURL 把查询串中的密钥参数替换为 ***, 便于安全写日志.
func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := parsed.Query()
	for _, key := range []string{"key", "appid", "apikey", "api_key", "token", "access_key", "secret"} {
		if query.Has(key) {
			query.Set(key, "***")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func snippet(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 200 {
		return text[:200] + "..."
	}
	return text
}

// loadLocation 解析 IANA 时区名, 失败返回 nil.
func loadLocation(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" || name == "auto" {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}

// parseTime 依次尝试自定义布局, RFC3339, 常见格式与 Unix 时间戳.
func parseTime(raw string, layout string, loc *time.Location) (time.Time, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return time.Time{}, false
	}
	if loc == nil {
		loc = time.UTC
	}

	if number, err := strconv.ParseInt(text, 10, 64); err == nil {
		if number > 1e12 {
			return time.UnixMilli(number).In(loc), true
		}
		if number > 1e9 {
			return time.Unix(number, 0).In(loc), true
		}
	}

	layouts := make([]string, 0, 6)
	if layout != "" {
		layouts = append(layouts, layout)
	}
	layouts = append(layouts,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
	)

	for _, candidate := range layouts {
		if parsed, err := time.ParseInLocation(candidate, text, loc); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// parseIntLoose 解析可能是字符串或数字的整数字段.
func parseIntLoose(raw string) (int, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return int(value), true
}

// parseFloatLoose 解析可能是字符串或数字的浮点字段.
func parseFloatLoose(raw string) (float64, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
