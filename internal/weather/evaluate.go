package weather

import (
	"fmt"
	"sort"
	"time"
)

// EvalOptions 描述降水判定阈值.
type EvalOptions struct {
	// LookaheadHours 是向后观察的小时数.
	LookaheadHours int
	// MinPrecipitationMM 是确定降水的小时降水量阈值.
	MinPrecipitationMM float64
	// MinProbability 是可能降水的概率阈值.
	MinProbability int
	// IncludeSnow 决定降雪是否算作降水事件.
	IncludeSnow bool
	// NotifyOnPossible 决定是否保留仅概率达标的信号.
	NotifyOnPossible bool
}

// Signal 是判定后有降水信号的小时.
type Signal struct {
	Time            time.Time
	Level           Level
	Kind            Kind
	PrecipitationMM float64
	Probability     int
	WeatherCode     int
	Description     string
}

// periodGap 是同一段降水中允许的最大信号间隔, 超过即视为两场降水.
const periodGap = 2 * time.Hour

// Period 是一段连续降水.
type Period struct {
	Start                time.Time
	End                  time.Time
	Level                Level
	Kind                 Kind
	PeakPrecipitationMM  float64
	PeakProbability      int
	TotalPrecipitationMM float64
	Signals              []Signal
}

// Span 返回时段的跨度, 与 Signals 数量不同, 因为段内可能存在无信号的整点.
func (p Period) Span() time.Duration { return p.End.Sub(p.Start) }

// Assessment 是一次降水判定的结果.
type Assessment struct {
	LocationName string
	Provider     string
	Location     *time.Location
	GeneratedAt  time.Time
	// WindowStart 与 WindowEnd 是判定覆盖的时间窗口.
	WindowStart time.Time
	WindowEnd   time.Time

	Level Level
	Kind  Kind
	// Start 与 End 覆盖全部信号, 单个时段的起止见 Periods.
	Start time.Time
	End   time.Time

	PeakPrecipitationMM  float64
	PeakProbability      int
	TotalPrecipitationMM float64
	Signals              []Signal
	// Periods 是按时间连续性切分出的降水时段.
	Periods []Period
}

// HasSignal 表示本次判定是否存在需要提醒的降水.
func (a Assessment) HasSignal() bool { return a.Level != LevelNone }

// EventKey 是用于去重的降水事件标识: 同一地点, 同一类型, 同一开始小时视为同一场降水.
func (a Assessment) EventKey() string {
	if !a.HasSignal() {
		return ""
	}
	return fmt.Sprintf("%s|%s|%s", a.LocationName, a.Kind, a.Start.UTC().Format("2006010215"))
}

// Summary 返回一句人话描述, 例如 "上海 未来 12 小时有雨".
func (a Assessment) Summary() string {
	if !a.HasSignal() {
		return fmt.Sprintf("%s 未来 %d 小时无降水", a.LocationName, int(a.WindowEnd.Sub(a.WindowStart).Hours()))
	}
	hours := int(a.WindowEnd.Sub(a.WindowStart).Hours())
	verb := "可能有降水"
	switch a.Level {
	case LevelCertain:
		verb = "有" + a.Kind.Chinese()
	case LevelPossible:
		verb = "可能" + a.Kind.Chinese()
	}
	return fmt.Sprintf("%s 未来 %d 小时%s", a.LocationName, hours, verb)
}

// Chinese 返回降水类型的中文名.
func (k Kind) Chinese() string {
	switch k {
	case KindRain:
		return "降雨"
	case KindSnow:
		return "降雪"
	case KindMixed:
		return "雨夹雪"
	case KindUnknown:
		return "降水"
	default:
		return "无降水"
	}
}

// Evaluate 依据阈值从预报中提取降水信号.
//
// now 之前的整点会被忽略, lookahead 窗口内的小时按阈值分类:
//   - 降水量达标, 或天气码本身表示降水: certain
//   - 仅降水概率达标: possible
func Evaluate(f *Forecast, now time.Time, opts EvalOptions) Assessment {
	if opts.LookaheadHours <= 0 {
		opts.LookaheadHours = 12
	}

	loc := f.Location
	if loc == nil {
		loc = time.UTC
	}

	windowStart := now.In(loc).Truncate(time.Hour)
	windowEnd := windowStart.Add(time.Duration(opts.LookaheadHours) * time.Hour)

	assessment := Assessment{
		Provider:    f.Provider,
		Location:    loc,
		GeneratedAt: now,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Level:       LevelNone,
		Kind:        KindNone,
	}

	hours := make([]Hour, len(f.Hours))
	copy(hours, f.Hours)
	sort.Slice(hours, func(i, j int) bool { return hours[i].Time.Before(hours[j].Time) })

	for _, hour := range hours {
		t := hour.Time.In(loc)
		if t.Before(windowStart) || !t.Before(windowEnd) {
			continue
		}

		level, kind := classify(hour, f.CodeSystem, opts)
		if level == LevelNone {
			continue
		}
		if level == LevelPossible && !opts.NotifyOnPossible {
			continue
		}

		description := hour.Description
		if description == "" {
			description = Describe(f.CodeSystem, hour.WeatherCode)
		}

		assessment.Signals = append(assessment.Signals, Signal{
			Time:            t,
			Level:           level,
			Kind:            kind,
			PrecipitationMM: hour.PrecipitationMM,
			Probability:     hour.Probability,
			WeatherCode:     hour.WeatherCode,
			Description:     description,
		})
	}

	if len(assessment.Signals) == 0 {
		return assessment
	}

	first := assessment.Signals[0]
	last := assessment.Signals[len(assessment.Signals)-1]

	assessment.Start = first.Time
	assessment.End = last.Time.Add(time.Hour)
	assessment.Level = LevelPossible
	assessment.PeakProbability = -1

	kindCount := map[Kind]int{}
	for _, signal := range assessment.Signals {
		if signal.Level > assessment.Level {
			assessment.Level = signal.Level
		}
		if signal.Kind != KindNone && signal.Kind != KindUnknown {
			kindCount[signal.Kind]++
		}
		if signal.PrecipitationMM > assessment.PeakPrecipitationMM {
			assessment.PeakPrecipitationMM = signal.PrecipitationMM
		}
		if signal.Probability > assessment.PeakProbability {
			assessment.PeakProbability = signal.Probability
		}
		assessment.TotalPrecipitationMM += signal.PrecipitationMM
	}
	assessment.Kind = dominantKind(kindCount)
	assessment.Periods = splitPeriods(assessment.Signals)

	return assessment
}

// splitPeriods 把信号按时间连续性切成若干降水时段.
func splitPeriods(signals []Signal) []Period {
	if len(signals) == 0 {
		return nil
	}

	periods := make([]Period, 0, 2)
	current := newPeriod(signals[0])

	for _, signal := range signals[1:] {
		previous := current.Signals[len(current.Signals)-1]
		if signal.Time.Sub(previous.Time) > periodGap {
			periods = append(periods, finalizePeriod(current))
			current = newPeriod(signal)
			continue
		}
		current.Signals = append(current.Signals, signal)
	}

	return append(periods, finalizePeriod(current))
}

func newPeriod(signal Signal) Period {
	return Period{
		Start:           signal.Time,
		End:             signal.Time.Add(time.Hour),
		Level:           signal.Level,
		Kind:            KindNone,
		PeakProbability: -1,
		Signals:         []Signal{signal},
	}
}

func finalizePeriod(period Period) Period {
	counts := map[Kind]int{}

	for _, signal := range period.Signals {
		if signal.Level > period.Level {
			period.Level = signal.Level
		}
		if signal.Kind != KindNone && signal.Kind != KindUnknown {
			counts[signal.Kind]++
		}
		if signal.PrecipitationMM > period.PeakPrecipitationMM {
			period.PeakPrecipitationMM = signal.PrecipitationMM
		}
		if signal.Probability > period.PeakProbability {
			period.PeakProbability = signal.Probability
		}
		period.TotalPrecipitationMM += signal.PrecipitationMM
	}

	period.Kind = dominantKind(counts)
	period.End = period.Signals[len(period.Signals)-1].Time.Add(time.Hour)
	return period
}

func classify(hour Hour, cs CodeSystem, opts EvalOptions) (Level, Kind) {
	rain := IsRain(cs, hour.WeatherCode)
	snow := IsSnow(cs, hour.WeatherCode)

	if snow && !opts.IncludeSnow && !rain {
		return LevelNone, KindNone
	}
	if snow && !opts.IncludeSnow {
		snow = false
	}

	wet := hour.PrecipitationMM > 0 && hour.PrecipitationMM >= opts.MinPrecipitationMM

	if wet || rain || snow {
		switch {
		case rain && snow:
			return LevelCertain, KindMixed
		case snow:
			return LevelCertain, KindSnow
		case rain:
			return LevelCertain, KindRain
		default:
			return LevelCertain, KindRain
		}
	}

	if opts.MinProbability > 0 && hour.Probability >= opts.MinProbability {
		return LevelPossible, KindUnknown
	}
	return LevelNone, KindNone
}

// dominantKind 选取出现次数最多的降水类型, 雨雪并存时记为 mixed.
func dominantKind(counts map[Kind]int) Kind {
	if len(counts) == 0 {
		return KindUnknown
	}
	if counts[KindRain] > 0 && counts[KindSnow] > 0 {
		return KindMixed
	}

	best := KindUnknown
	bestCount := -1
	for kind, count := range counts {
		if count > bestCount {
			best, bestCount = kind, count
		}
	}
	return best
}
