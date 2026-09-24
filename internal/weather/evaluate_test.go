package weather

import (
	"testing"
	"time"
)

func testLocation() *time.Location {
	return time.FixedZone("CST", 8*3600)
}

func testNow() time.Time {
	return time.Date(2026, 9, 24, 10, 30, 0, 0, testLocation())
}

func at(hour int) time.Time {
	return time.Date(2026, 9, 24, hour, 0, 0, 0, testLocation())
}

func TestEvaluateFindsCertainRain(t *testing.T) {
	forecast := &Forecast{
		Provider:   "test",
		CodeSystem: CodeWMO,
		Location:   testLocation(),
		Hours: []Hour{
			{Time: at(9), PrecipitationMM: 0, Probability: 10, WeatherCode: 0},
			{Time: at(13), PrecipitationMM: 1.2, Probability: 70, WeatherCode: 61},
			{Time: at(14), PrecipitationMM: 3.4, Probability: 85, WeatherCode: 63},
			{Time: at(15), PrecipitationMM: 0, Probability: 20, WeatherCode: 3},
		},
	}

	assessment := Evaluate(forecast, testNow(), EvalOptions{
		LookaheadHours:     12,
		MinPrecipitationMM: 0.1,
		MinProbability:     60,
		NotifyOnPossible:   true,
	})

	if assessment.Level != LevelCertain {
		t.Fatalf("级别应为 certain, 实际为 %s", assessment.Level)
	}
	if assessment.Kind != KindRain {
		t.Fatalf("类型应为 rain, 实际为 %s", assessment.Kind)
	}
	if len(assessment.Signals) != 2 {
		t.Fatalf("信号数量应为 2, 实际为 %d", len(assessment.Signals))
	}
	if !assessment.Start.Equal(at(13)) {
		t.Fatalf("起始时间应为 13:00, 实际为 %s", assessment.Start)
	}
	if !assessment.End.Equal(at(15)) {
		t.Fatalf("结束时间应为 15:00, 实际为 %s", assessment.End)
	}
	if assessment.PeakPrecipitationMM != 3.4 {
		t.Fatalf("峰值降水应为 3.4, 实际为 %v", assessment.PeakPrecipitationMM)
	}
	if assessment.PeakProbability != 85 {
		t.Fatalf("峰值概率应为 85, 实际为 %d", assessment.PeakProbability)
	}
}

func TestEvaluateIgnoresPastHours(t *testing.T) {
	forecast := &Forecast{
		Provider:   "test",
		CodeSystem: CodeWMO,
		Location:   testLocation(),
		Hours: []Hour{
			{Time: at(8), PrecipitationMM: 5, Probability: 90, WeatherCode: 65},
			{Time: at(11), PrecipitationMM: 0, Probability: 5, WeatherCode: 0},
		},
	}

	assessment := Evaluate(forecast, testNow(), EvalOptions{
		LookaheadHours:     6,
		MinPrecipitationMM: 0.1,
		MinProbability:     60,
		NotifyOnPossible:   true,
	})

	if assessment.HasSignal() {
		t.Fatalf("过去小时的降水不应触发提醒, 实际信号数为 %d", len(assessment.Signals))
	}
}

func TestEvaluateProbabilityOnly(t *testing.T) {
	forecast := &Forecast{
		Provider:   "test",
		CodeSystem: CodeWMO,
		Location:   testLocation(),
		Hours: []Hour{
			{Time: at(11), PrecipitationMM: 0, Probability: 75, WeatherCode: 3},
		},
	}
	options := EvalOptions{
		LookaheadHours:     6,
		MinPrecipitationMM: 0.1,
		MinProbability:     60,
		NotifyOnPossible:   true,
	}

	assessment := Evaluate(forecast, testNow(), options)
	if assessment.Level != LevelPossible {
		t.Fatalf("级别应为 possible, 实际为 %s", assessment.Level)
	}
	if assessment.Kind != KindUnknown {
		t.Fatalf("仅概率达标时类型应为 unknown, 实际为 %s", assessment.Kind)
	}

	options.NotifyOnPossible = false
	assessment = Evaluate(forecast, testNow(), options)
	if assessment.HasSignal() {
		t.Fatal("关闭概率提醒后不应产生信号")
	}
}

func TestEvaluateSnowNeedsOptIn(t *testing.T) {
	forecast := &Forecast{
		Provider:   "test",
		CodeSystem: CodeWMO,
		Location:   testLocation(),
		Hours: []Hour{
			{Time: at(11), PrecipitationMM: 0.6, Probability: 80, WeatherCode: 71},
		},
	}

	assessment := Evaluate(forecast, testNow(), EvalOptions{
		LookaheadHours:     6,
		MinPrecipitationMM: 0.1,
		MinProbability:     60,
		IncludeSnow:        false,
		NotifyOnPossible:   true,
	})
	if assessment.HasSignal() {
		t.Fatal("未开启 include_snow 时降雪不应触发提醒")
	}

	assessment = Evaluate(forecast, testNow(), EvalOptions{
		LookaheadHours:     6,
		MinPrecipitationMM: 0.1,
		MinProbability:     60,
		IncludeSnow:        true,
		NotifyOnPossible:   true,
	})
	if assessment.Level != LevelCertain || assessment.Kind != KindSnow {
		t.Fatalf("开启 include_snow 后应为确定降雪, 实际为 %s / %s", assessment.Level, assessment.Kind)
	}
}

func TestEventKeyDistinguishesRains(t *testing.T) {
	first := Assessment{LocationName: "上海", Kind: KindRain, Level: LevelCertain, Start: at(13)}
	sameRain := Assessment{LocationName: "上海", Kind: KindRain, Level: LevelCertain, Start: at(13)}
	laterRain := Assessment{LocationName: "上海", Kind: KindRain, Level: LevelCertain, Start: at(18)}

	if first.EventKey() != sameRain.EventKey() {
		t.Fatal("同一场降水的 event key 应保持一致")
	}
	if first.EventKey() == laterRain.EventKey() {
		t.Fatal("不同时段的降水应有不同的 event key")
	}
	if (Assessment{Level: LevelNone}).EventKey() != "" {
		t.Fatal("无信号时 event key 应为空")
	}
}

func TestPeriodsSplitByGap(t *testing.T) {
	forecast := &Forecast{
		Provider:   "test",
		CodeSystem: CodeWMO,
		Location:   testLocation(),
		Hours: []Hour{
			{Time: at(11), PrecipitationMM: 1.0, Probability: 80, WeatherCode: 61},
			{Time: at(12), PrecipitationMM: 0.5, Probability: 75, WeatherCode: 61},
			// 与上一个信号相隔 4 小时, 应另起一段.
			{Time: at(16), PrecipitationMM: 2.0, Probability: 90, WeatherCode: 63},
			// 与上一个信号相隔 2 小时, 仍属于同一段.
			{Time: at(18), PrecipitationMM: 0.4, Probability: 70, WeatherCode: 61},
		},
	}

	assessment := Evaluate(forecast, testNow(), EvalOptions{
		LookaheadHours:     12,
		MinPrecipitationMM: 0.1,
		MinProbability:     60,
		NotifyOnPossible:   true,
	})

	if len(assessment.Periods) != 2 {
		t.Fatalf("应切分出 2 段降水, 实际为 %d", len(assessment.Periods))
	}

	first := assessment.Periods[0]
	if len(first.Signals) != 2 {
		t.Fatalf("第 1 段应包含 2 个信号, 实际为 %d", len(first.Signals))
	}
	if !first.Start.Equal(at(11)) || !first.End.Equal(at(13)) {
		t.Fatalf("第 1 段应覆盖 11:00 到 13:00, 实际为 %s ~ %s", first.Start, first.End)
	}
	if first.TotalPrecipitationMM != 1.5 {
		t.Fatalf("第 1 段累计降水应为 1.5, 实际为 %v", first.TotalPrecipitationMM)
	}

	second := assessment.Periods[1]
	if len(second.Signals) != 2 {
		t.Fatalf("第 2 段应包含 2 个信号, 实际为 %d", len(second.Signals))
	}
	if !second.Start.Equal(at(16)) || !second.End.Equal(at(19)) {
		t.Fatalf("第 2 段应覆盖 16:00 到 19:00, 实际为 %s ~ %s", second.Start, second.End)
	}
	if second.PeakPrecipitationMM != 2.0 {
		t.Fatalf("第 2 段峰值应为 2.0, 实际为 %v", second.PeakPrecipitationMM)
	}
}
