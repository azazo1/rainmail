package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/azazo1/rainmail/internal/config"
	"github.com/azazo1/rainmail/internal/notify"
	"github.com/azazo1/rainmail/internal/state"
	"github.com/azazo1/rainmail/internal/weather"
)

// scriptedProvider 每次查询都依据当前时间生成一份包含降水的预报.
type scriptedProvider struct {
	now func() time.Time
	err error
}

func (p *scriptedProvider) Name() string { return "scripted" }

func (p *scriptedProvider) Hourly(context.Context, weather.Request) (*weather.Forecast, error) {
	if p.err != nil {
		return nil, p.err
	}
	now := p.now()
	loc := now.Location()
	start := now.Truncate(time.Hour).Add(time.Hour)

	hours := make([]weather.Hour, 0, 4)
	for i := 0; i < 4; i++ {
		hour := weather.Hour{
			Time:        start.Add(time.Duration(i) * time.Hour),
			Probability: 20,
			WeatherCode: 3,
		}
		if i < 2 {
			hour.PrecipitationMM = 1.5
			hour.WeatherCode = 61
			hour.Description = "小雨"
			hour.Probability = 80
		}
		hours = append(hours, hour)
	}

	return &weather.Forecast{
		Provider:   "scripted",
		CodeSystem: weather.CodeWMO,
		Location:   loc,
		Timezone:   loc.String(),
		FetchedAt:  now,
		Hours:      hours,
	}, nil
}

type countingNotifier struct {
	name string
	sent int
	err  error
}

func (n *countingNotifier) Name() string { return n.name }

func (n *countingNotifier) Send(context.Context, notify.Message) error {
	if n.err != nil {
		return n.err
	}
	n.sent++
	return nil
}

type serviceFixture struct {
	service  *Service
	store    *state.Store
	now      *time.Time
	provider *scriptedProvider
}

func newFixture(t *testing.T, at time.Time, notifiers ...notify.Notifier) *serviceFixture {
	t.Helper()

	cfg := config.Default()
	cfg.Location.Name = "测试城"
	cfg.Notify.Email.Enabled = false
	cfg.Notify.System.Enabled = true
	cfg.Repeat.Cooldown = config.Duration(time.Hour)

	current := at
	provider := &scriptedProvider{now: func() time.Time { return current }}

	store, err := state.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("打开状态文件失败: %v", err)
	}

	if notifiers == nil {
		notifiers = []notify.Notifier{}
	}

	service, err := New(Options{
		Config:    cfg,
		Provider:  provider,
		Store:     store,
		Notifiers: notifiers,
		Now:       func() time.Time { return current },
	})
	if err != nil {
		t.Fatalf("构造 Service 失败: %v", err)
	}

	return &serviceFixture{
		service:  service,
		store:    store,
		now:      &current,
		provider: provider,
	}
}

func baseTime() time.Time {
	return time.Date(2026, 9, 24, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
}

func TestCheckNotifiesOnceThenRespectsCooldown(t *testing.T) {
	fixture := newFixture(t, baseTime())
	email := &countingNotifier{name: ChannelEmail}
	system := &countingNotifier{name: ChannelSystem}
	fixture.service.dispatcher = notify.NewDispatcher(nil, email, system)

	first, err := fixture.service.Check(context.Background(), CheckOptions{})
	if err != nil {
		t.Fatalf("首次检查失败: %v", err)
	}
	if !first.Notified {
		t.Fatal("首次检查应发出提醒")
	}
	if email.sent != 1 || system.sent != 1 {
		t.Fatalf("两个通道各应发送一次, 实际为 %d / %d", email.sent, system.sent)
	}

	second, err := fixture.service.Check(context.Background(), CheckOptions{})
	if err != nil {
		t.Fatalf("第二次检查失败: %v", err)
	}
	if second.Notified {
		t.Fatal("冷却期内不应重复提醒")
	}
	if !strings.Contains(second.Skipped, "冷却") {
		t.Fatalf("跳过原因应说明冷却, 实际为 %q", second.Skipped)
	}
	if email.sent != 1 || system.sent != 1 {
		t.Fatalf("冷却期内不应再次发送, 实际为 %d / %d", email.sent, system.sent)
	}
}

func TestCheckNotifiesAgainAfterCooldown(t *testing.T) {
	fixture := newFixture(t, baseTime())
	email := &countingNotifier{name: ChannelEmail}
	fixture.service.dispatcher = notify.NewDispatcher(nil, email)

	if _, err := fixture.service.Check(context.Background(), CheckOptions{}); err != nil {
		t.Fatalf("首次检查失败: %v", err)
	}

	// 冷却时间为 1 小时, 推进 2 小时且起始小时改变后应重新提醒.
	*fixture.now = baseTime().Add(2 * time.Hour)
	result, err := fixture.service.Check(context.Background(), CheckOptions{})
	if err != nil {
		t.Fatalf("第二次检查失败: %v", err)
	}
	if !result.Notified {
		t.Fatalf("冷却结束后应重新提醒, 跳过原因: %s", result.Skipped)
	}
	if email.sent != 2 {
		t.Fatalf("邮件通道应发送两次, 实际为 %d", email.sent)
	}
}

func TestCheckDryRunDoesNotSend(t *testing.T) {
	fixture := newFixture(t, baseTime())
	email := &countingNotifier{name: ChannelEmail}
	fixture.service.dispatcher = notify.NewDispatcher(nil, email)

	result, err := fixture.service.Check(context.Background(), CheckOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run 检查失败: %v", err)
	}
	if result.Notified || email.sent != 0 {
		t.Fatal("dry-run 不应发送提醒")
	}
	if !result.Assessment.HasSignal() {
		t.Fatal("dry-run 仍应给出判定结果")
	}
}

func TestCheckQuietHoursKeepsEmailOnly(t *testing.T) {
	night := time.Date(2026, 9, 24, 3, 0, 0, 0, time.FixedZone("CST", 8*3600))
	fixture := newFixture(t, night)
	email := &countingNotifier{name: ChannelEmail}
	system := &countingNotifier{name: ChannelSystem}
	fixture.service.dispatcher = notify.NewDispatcher(nil, email, system)
	fixture.service.quietHours = []config.QuietWindow{mustQuietWindow(t, "23:00-07:00")}

	result, err := fixture.service.Check(context.Background(), CheckOptions{})
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if !result.Notified {
		t.Fatalf("免打扰时段仍应发送邮件, 跳过原因: %s", result.Skipped)
	}
	if email.sent != 1 {
		t.Fatalf("邮件通道应发送一次, 实际为 %d", email.sent)
	}
	if system.sent != 0 {
		t.Fatalf("免打扰时段不应发送系统通知, 实际为 %d", system.sent)
	}
}

func TestCheckQuietHoursCanSkipEverything(t *testing.T) {
	night := time.Date(2026, 9, 24, 3, 0, 0, 0, time.FixedZone("CST", 8*3600))
	fixture := newFixture(t, night)
	email := &countingNotifier{name: ChannelEmail}
	fixture.service.dispatcher = notify.NewDispatcher(nil, email)
	fixture.service.quietHours = []config.QuietWindow{mustQuietWindow(t, "23:00-07:00")}
	fixture.service.cfg.Notify.QuietHoursSkipEmail = true

	result, err := fixture.service.Check(context.Background(), CheckOptions{})
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if result.Notified || email.sent != 0 {
		t.Fatal("配置跳过邮件后不应发送任何提醒")
	}
	if !strings.Contains(result.Skipped, "免打扰") {
		t.Fatalf("跳过原因应说明免打扰, 实际为 %q", result.Skipped)
	}
}

func TestCheckKeepsEventPendingWhenAllChannelsFail(t *testing.T) {
	fixture := newFixture(t, baseTime())
	failing := &countingNotifier{name: ChannelEmail, err: errors.New("smtp 不可用")}
	fixture.service.dispatcher = notify.NewDispatcher(nil, failing)

	first, err := fixture.service.Check(context.Background(), CheckOptions{})
	if err != nil {
		t.Fatalf("检查不应因为通道失败而报错: %v", err)
	}
	if first.Notified {
		t.Fatal("通道全部失败时不应标记为已提醒")
	}
	if first.Err() == nil {
		t.Fatal("通道失败应体现在结果中")
	}
	if key := fixture.store.Snapshot().LastEventKey; key != "" {
		t.Fatalf("通道全部失败时不应记录 event key, 实际为 %q", key)
	}

	// 下一次检查仍然应该继续尝试, 而不是被冷却规则挡住.
	second, err := fixture.service.Check(context.Background(), CheckOptions{})
	if err != nil {
		t.Fatalf("第二次检查失败: %v", err)
	}
	if second.Skipped != "" {
		t.Fatalf("通道失败后不应进入冷却, 跳过原因: %s", second.Skipped)
	}
}

func TestCheckReportsProviderFailure(t *testing.T) {
	fixture := newFixture(t, baseTime())
	fixture.provider.err = errors.New("网络不可达")

	if _, err := fixture.service.Check(context.Background(), CheckOptions{}); err == nil {
		t.Fatal("天气接口失败时应返回错误")
	}
}

func mustQuietWindow(t *testing.T, raw string) config.QuietWindow {
	t.Helper()
	window, err := config.ParseQuietWindow(raw)
	if err != nil {
		t.Fatalf("解析免打扰时段失败: %v", err)
	}
	return window
}
