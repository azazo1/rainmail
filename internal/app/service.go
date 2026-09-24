// Package app 编排天气查询, 降水判定, 提醒发送与状态更新.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/azazo1/rainmail/internal/config"
	"github.com/azazo1/rainmail/internal/notify"
	"github.com/azazo1/rainmail/internal/notify/email"
	"github.com/azazo1/rainmail/internal/notify/system"
	"github.com/azazo1/rainmail/internal/report"
	"github.com/azazo1/rainmail/internal/state"
	"github.com/azazo1/rainmail/internal/weather"
)

// 通道名, 与 notifier.Name() 保持一致.
const (
	ChannelEmail  = "email"
	ChannelSystem = "system"
)

// Service 是 rainmail 的核心流程.
type Service struct {
	cfg        *config.Config
	provider   weather.Provider
	store      *state.Store
	dispatcher *notify.Dispatcher
	quietHours []config.QuietWindow
	logger     *slog.Logger
	now        func() time.Time
}

// Options 描述构造 Service 所需的依赖, 便于测试注入替身.
type Options struct {
	Config   *config.Config
	Logger   *slog.Logger
	Provider weather.Provider
	Store    *state.Store
	// Notifiers 为 nil 时按配置自动构造; 传空切片表示不使用任何通道.
	Notifiers []notify.Notifier
	// Now 用于注入时间, 为 nil 时使用 time.Now.
	Now func() time.Time
}

// New 构造 Service.
func New(opts Options) (*Service, error) {
	if opts.Config == nil {
		return nil, errors.New("构造 Service 需要配置")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	provider := opts.Provider
	if provider == nil {
		settings := opts.Config.Weather.Providers[opts.Config.Weather.Provider]
		client := &http.Client{Timeout: opts.Config.Weather.Timeout.Duration()}
		created, err := weather.New(weatherOptions(opts.Config.Weather.Provider, settings), client)
		if err != nil {
			return nil, err
		}
		provider = created
	}

	store := opts.Store
	if store == nil {
		statePath, err := opts.Config.StatePath()
		if err != nil {
			return nil, err
		}
		opened, err := state.Open(statePath)
		if err != nil {
			return nil, err
		}
		store = opened
	}

	notifiers := opts.Notifiers
	if notifiers == nil {
		built, err := BuildNotifiers(opts.Config, logger)
		if err != nil {
			return nil, err
		}
		notifiers = built
	}

	quietHours, err := config.ParseQuietWindows(opts.Config.Notify.QuietHours)
	if err != nil {
		return nil, err
	}

	return &Service{
		cfg:        opts.Config,
		provider:   provider,
		store:      store,
		dispatcher: notify.NewDispatcher(logger, notifiers...),
		quietHours: quietHours,
		logger:     logger,
		now:        now,
	}, nil
}

// BuildNotifiers 依据配置构造提醒通道.
func BuildNotifiers(cfg *config.Config, logger *slog.Logger) ([]notify.Notifier, error) {
	if logger == nil {
		logger = slog.Default()
	}

	var notifiers []notify.Notifier

	if cfg.Notify.Email.Enabled {
		channel, err := email.New(email.Config{
			Host:               cfg.Notify.Email.Host,
			Port:               cfg.Notify.Email.Port,
			Encryption:         cfg.Notify.Email.Encryption,
			Username:           cfg.Notify.Email.Username,
			Password:           cfg.Notify.Email.Password,
			From:               cfg.Notify.Email.From,
			FromName:           cfg.Notify.Email.FromName,
			To:                 cfg.Notify.Email.To,
			SubjectPrefix:      cfg.Notify.Email.SubjectPrefix,
			InsecureSkipVerify: cfg.Notify.Email.InsecureSkipVerify,
			Timeout:            cfg.Notify.Email.Timeout.Duration(),
		}, logger)
		if err != nil {
			return nil, fmt.Errorf("初始化邮件通道失败: %w", err)
		}
		notifiers = append(notifiers, channel)
	}

	if cfg.Notify.System.Enabled {
		channel := system.New(cfg.Notify.System.Sound, logger)
		if !channel.Available() {
			logger.Warn("当前平台缺少可用的桌面通知机制, 系统提醒会发送失败",
				"os", runtime.GOOS, "hint", desktopHint())
		}
		notifiers = append(notifiers, channel)
	}

	if len(notifiers) == 0 {
		return nil, errors.New("没有启用任何提醒通道, 请检查 notify.email 与 notify.system")
	}
	return notifiers, nil
}

// Provider 返回使用的天气接口.
func (s *Service) Provider() weather.Provider { return s.provider }

// Store 返回状态存储.
func (s *Service) Store() *state.Store { return s.store }

// Channels 返回已启用的提醒通道名.
func (s *Service) Channels() []string { return s.dispatcher.Names() }

// CheckOptions 控制单次检查的行为.
type CheckOptions struct {
	// DryRun 只查询与判定, 不发送提醒, 也不落盘提醒状态.
	DryRun bool
	// Force 忽略冷却时间强制发送.
	Force bool
	// Channels 限定发送通道, 为空表示全部已启用通道.
	Channels []string
}

// Result 是一次检查的完整结果.
type Result struct {
	Assessment weather.Assessment
	FetchedAt  time.Time
	Notified   bool
	// Skipped 说明本次没有发送提醒的原因, 为空表示提醒已尝试发送.
	Skipped  string
	Channels []notify.Result
	Duration time.Duration
}

// Err 汇总各通道的错误.
func (r Result) Err() error { return notify.Errors(r.Channels) }

// Check 执行一次完整的检查: 拉取预报, 判定降水, 去重后发送提醒.
func (s *Service) Check(ctx context.Context, opts CheckOptions) (*Result, error) {
	started := time.Now()
	now := s.now()

	s.logger.Info("开始检查降水",
		"location", s.cfg.Location.Name,
		"provider", s.provider.Name(),
		"lookahead_hours", s.cfg.Weather.LookaheadHours)

	request := weather.Request{
		LocationName: s.cfg.Location.Name,
		Latitude:     s.cfg.Location.Latitude,
		Longitude:    s.cfg.Location.Longitude,
		Timezone:     s.cfg.Location.Timezone,
		Hours:        s.cfg.Weather.LookaheadHours,
	}

	fetchStarted := time.Now()
	forecast, err := s.provider.Hourly(ctx, request)
	if err != nil {
		s.logger.Error("查询天气失败",
			"provider", s.provider.Name(),
			"error", err,
			"elapsed", time.Since(fetchStarted).Round(time.Millisecond))
		return nil, fmt.Errorf("查询天气失败: %w", err)
	}
	s.logger.Info("天气查询完成",
		"provider", forecast.Provider,
		"hours", len(forecast.Hours),
		"timezone", forecast.Timezone,
		"elapsed", time.Since(fetchStarted).Round(time.Millisecond))

	assessment := weather.Evaluate(forecast, now, weather.EvalOptions{
		LookaheadHours:     s.cfg.Weather.LookaheadHours,
		MinPrecipitationMM: s.cfg.Weather.MinPrecipitationMM,
		MinProbability:     s.cfg.Weather.MinProbability,
		IncludeSnow:        s.cfg.Weather.IncludeSnow,
		NotifyOnPossible:   s.cfg.Notify.NotifyOnPossible,
	})
	assessment.LocationName = s.cfg.Location.Name

	result := &Result{Assessment: assessment, FetchedAt: forecast.FetchedAt}
	defer func() { result.Duration = time.Since(started) }()

	if !assessment.HasSignal() {
		result.Skipped = "观察窗口内没有降水信号"
		s.logger.Info("未检测到降水",
			"window_start", assessment.WindowStart.Format(time.RFC3339),
			"window_end", assessment.WindowEnd.Format(time.RFC3339),
			"forecast_hours", len(forecast.Hours))
		s.recordChecked(now)
		return result, nil
	}

	s.logger.Info("检测到降水",
		"level", assessment.Level.String(),
		"kind", string(assessment.Kind),
		"start", assessment.Start.Format(time.RFC3339),
		"end", assessment.End.Format(time.RFC3339),
		"peak_precipitation_mm", assessment.PeakPrecipitationMM,
		"peak_probability", assessment.PeakProbability,
		"signals", len(assessment.Signals),
		"event_key", assessment.EventKey())

	if opts.DryRun {
		result.Skipped = "dry-run: 仅展示判定结果, 未发送提醒"
		return result, nil
	}

	channels, skipReason := s.resolveChannels(opts.Channels, now)
	if len(channels) == 0 {
		result.Skipped = skipReason
		s.logger.Info("没有可用的提醒通道, 本次不发送", "reason", skipReason)
		s.recordChecked(now)
		return result, nil
	}

	if reason := s.cooldownReason(now, assessment, opts.Force); reason != "" {
		result.Skipped = reason
		s.logger.Info("跳过重复提醒", "event_key", assessment.EventKey(), "reason", reason)
		s.recordChecked(now)
		return result, nil
	}

	message := report.Message(assessment)
	s.logger.Info("开始发送提醒",
		"channels", strings.Join(channels, ","),
		"title", message.Title,
		"level", string(message.Level))

	results := s.dispatcher.SendTo(ctx, channels, message)
	result.Channels = results
	result.Notified = len(notify.Succeeded(results)) > 0

	s.saveNotifyState(now, assessment, results)

	if err := notify.Errors(results); err != nil {
		s.logger.Error("部分提醒通道发送失败", "error", err)
	}
	return result, nil
}

// Run 常驻轮询, 直到 ctx 结束. interval 为 0 时使用配置中的间隔.
func (s *Service) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = s.cfg.Weather.CheckInterval.Duration()
	}
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	s.logger.Info("rainmail 进入常驻模式",
		"interval", interval,
		"channels", strings.Join(s.dispatcher.Names(), ","),
		"state_file", s.store.Path(),
		"lookahead_hours", s.cfg.Weather.LookaheadHours)

	// 启动后先检查一次, 避免等待一个完整间隔.
	s.checkOnce(ctx)

	for {
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.logger.Info("收到退出信号, 常驻模式结束")
			return nil
		case <-timer.C:
			s.checkOnce(ctx)
		}
	}
}

func (s *Service) checkOnce(ctx context.Context) {
	result, err := s.Check(ctx, CheckOptions{})
	if err != nil {
		s.logger.Error("本次检查失败", "error", err)
		return
	}
	if result.Skipped != "" {
		s.logger.Debug("本次检查未发送提醒", "reason", result.Skipped)
	}
}

// resolveChannels 计算本次实际使用的通道.
func (s *Service) resolveChannels(requested []string, now time.Time) ([]string, string) {
	channels := requested
	if len(channels) == 0 {
		channels = s.dispatcher.Names()
	}
	if !config.InQuietHours(s.quietHours, now) {
		return channels, ""
	}

	if s.cfg.Notify.QuietHoursSkipEmail {
		return nil, fmt.Sprintf("当前处于免打扰时段 %v, 已跳过全部提醒", s.cfg.Notify.QuietHours)
	}
	kept := make([]string, 0, len(channels))
	for _, name := range channels {
		if name != ChannelSystem {
			kept = append(kept, name)
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Sprintf("当前处于免打扰时段 %v, 且只启用了系统通知", s.cfg.Notify.QuietHours)
	}
	s.logger.Info("免打扰时段, 仅保留非系统通道", "channels", strings.Join(kept, ","))
	return kept, ""
}

// cooldownReason 判断是否因为冷却时间而跳过提醒, 空串表示可以发送.
func (s *Service) cooldownReason(now time.Time, assessment weather.Assessment, force bool) string {
	if force {
		return ""
	}

	current := s.store.Snapshot()
	if assessment.EventKey() != current.LastEventKey || current.LastNotifiedAt.IsZero() {
		return ""
	}

	cooldown := s.cfg.Repeat.Cooldown.Duration()
	if cooldown <= 0 {
		return ""
	}
	elapsed := now.Sub(current.LastNotifiedAt)
	if elapsed >= cooldown {
		return ""
	}
	loc := assessment.Location
	if loc == nil {
		loc = time.UTC
	}
	return fmt.Sprintf("同一场降水已于 %s 提醒过, 距冷却结束还有 %s",
		current.LastNotifiedAt.In(loc).Format("2006-01-02 15:04"),
		(cooldown-elapsed).Round(time.Minute))
}

func (s *Service) recordChecked(now time.Time) {
	if err := s.store.Update(func(st *state.State) { st.LastCheckedAt = now }); err != nil {
		s.logger.Error("更新状态失败", "error", err)
	}
}

func (s *Service) saveNotifyState(now time.Time, assessment weather.Assessment, results []notify.Result) {
	succeeded := notify.Succeeded(results)
	err := s.store.Update(func(st *state.State) {
		st.LastCheckedAt = now
		st.LastLevel = assessment.Level.String()
		if len(succeeded) > 0 {
			st.LastNotifiedAt = now
			st.LastEventKey = assessment.EventKey()
			st.LastSummary = assessment.Summary()
		}
		st.History = append(st.History, state.Record{
			At:       now,
			EventKey: assessment.EventKey(),
			Level:    assessment.Level.String(),
			Kind:     string(assessment.Kind),
			Summary:  assessment.Summary(),
			Channels: succeeded,
			Errors:   errorTexts(results),
		})
	})
	if err != nil {
		s.logger.Error("保存状态失败", "error", err)
	}
}

func errorTexts(results []notify.Result) []string {
	var out []string
	for _, r := range results {
		if r.Err != nil {
			out = append(out, r.Name+": "+r.Err.Error())
		}
	}
	return out
}

func weatherOptions(name string, settings config.ProviderSettings) weather.Options {
	return weather.Options{
		Provider:          name,
		BaseURL:           settings.BaseURL,
		APIKey:            settings.APIKey,
		Language:          settings.Language,
		Units:             settings.Units,
		URL:               settings.URL,
		Method:            settings.Method,
		Headers:           settings.Headers,
		ItemsPath:         settings.ItemsPath,
		TimePath:          settings.TimePath,
		TimeFormat:        settings.TimeFormat,
		CodeSystem:        settings.CodeSystem,
		PrecipitationPath: settings.PrecipitationPath,
		ProbabilityPath:   settings.ProbabilityPath,
		WeatherCodePath:   settings.WeatherCodePath,
		TemperaturePath:   settings.TemperaturePath,
	}
}

func desktopHint() string {
	switch runtime.GOOS {
	case "linux":
		return "请安装 libnotify-bin 以提供 notify-send"
	case "windows":
		return "请确认 PowerShell 可用"
	case "darwin":
		return "请确认 osascript 可用"
	default:
		return "当前系统暂不支持桌面通知, 可改用邮件通道"
	}
}
