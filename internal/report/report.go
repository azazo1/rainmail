// Package report 把降水评估结果渲染成提醒消息.
package report

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/azazo1/rainmail/internal/notify"
	"github.com/azazo1/rainmail/internal/weather"
)

const timeLayout = "2006-01-02 15:04"

// Message 渲染一条提醒消息, 同时给出纯文本与 HTML 两种正文.
func Message(a weather.Assessment) notify.Message {
	return notify.Message{
		Title: a.Summary(),
		Text:  Text(a),
		HTML:  HTML(a),
		Level: level(a),
		Tags:  map[string]string{"event_key": a.EventKey()},
	}
}

func level(a weather.Assessment) notify.Level {
	if a.Level == weather.LevelCertain {
		return notify.LevelWarning
	}
	return notify.LevelInfo
}

// Text 渲染纯文本正文.
func Text(a weather.Assessment) string {
	var b strings.Builder

	b.WriteString("rainmail 降水提醒\n")
	b.WriteString(strings.Repeat("=", 32) + "\n")
	fmt.Fprintf(&b, "地点: %s\n", a.LocationName)
	fmt.Fprintf(&b, "结论: %s\n", a.Summary())
	fmt.Fprintf(&b, "数据来源: %s\n", a.Provider)

	if a.HasSignal() {
		fmt.Fprintf(&b, "降水类型: %s\n", a.Kind.Chinese())
		fmt.Fprintf(&b, "置信度: %s\n", levelText(a.Level))
		fmt.Fprintf(&b, "整体跨度: %s ~ %s\n", formatTime(a.Start, a.Location), formatTime(a.End, a.Location))
		fmt.Fprintf(&b, "降水时段数: %d\n", len(a.Periods))
		fmt.Fprintf(&b, "峰值小时降水: %.1f mm\n", a.PeakPrecipitationMM)
		fmt.Fprintf(&b, "累计降水: %.1f mm\n", a.TotalPrecipitationMM)
		fmt.Fprintf(&b, "最大降水概率: %s\n", probabilityText(a.PeakProbability))

		for i, period := range a.Periods {
			fmt.Fprintf(&b, "\n[%d] %s ~ %s  %s%s  峰值 %.1f mm  概率 %s  累计 %.1f mm\n",
				i+1,
				formatTime(period.Start, a.Location),
				formatTime(period.End, a.Location),
				levelText(period.Level),
				kindSuffix(period.Kind),
				period.PeakPrecipitationMM,
				probabilityText(period.PeakProbability),
				period.TotalPrecipitationMM)
			for _, signal := range period.Signals {
				fmt.Fprintf(&b, "      %s  %5.1f mm  %6s  %s  %s\n",
					formatTime(signal.Time, a.Location),
					signal.PrecipitationMM,
					probabilityText(signal.Probability),
					levelText(signal.Level),
					orUnknown(signal.Description))
			}
		}
	} else {
		fmt.Fprintf(&b, "观察窗口: %s ~ %s\n", formatTime(a.WindowStart, a.Location), formatTime(a.WindowEnd, a.Location))
	}

	fmt.Fprintf(&b, "\n生成时间: %s\n", formatTime(a.GeneratedAt, a.Location))
	return b.String()
}

// HTML 渲染 HTML 正文, 供支持富文本的邮件客户端展示.
func HTML(a weather.Assessment) string {
	var b strings.Builder

	b.WriteString(`<div style="font-family:-apple-system,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;`)
	b.WriteString(`font-size:14px;line-height:1.7;color:#1f2328">`)
	fmt.Fprintf(&b, `<h2 style="font-size:18px;margin:0 0 12px">%s</h2>`, html.EscapeString(a.Summary()))

	fmt.Fprintf(&b, `<p style="margin:0 0 6px">地点: <strong>%s</strong>, 数据来源: %s</p>`,
		html.EscapeString(a.LocationName), html.EscapeString(a.Provider))

	if a.HasSignal() {
		fmt.Fprintf(&b, `<p style="margin:0 0 6px">整体跨度: %s ~ %s, 共 %d 段降水</p>`,
			formatTime(a.Start, a.Location), formatTime(a.End, a.Location), len(a.Periods))
		fmt.Fprintf(&b, `<p style="margin:0 0 6px">峰值小时降水: <strong>%.1f mm</strong>, 累计: %.1f mm, 最大概率: %s</p>`,
			a.PeakPrecipitationMM, a.TotalPrecipitationMM, probabilityText(a.PeakProbability))

		for i, period := range a.Periods {
			fmt.Fprintf(&b, `<h3 style="font-size:15px;margin:16px 0 6px">[%d] %s ~ %s · %s%s · 峰值 %.1f mm · 概率 %s</h3>`,
				i+1,
				formatTime(period.Start, a.Location),
				formatTime(period.End, a.Location),
				levelText(period.Level),
				kindSuffix(period.Kind),
				period.PeakPrecipitationMM,
				probabilityText(period.PeakProbability))

			b.WriteString(`<table cellpadding="6" cellspacing="0" style="border-collapse:collapse;font-size:13px">`)
			b.WriteString(`<thead><tr style="background:#f2f3f5">`)
			b.WriteString(`<th align="left">时间</th><th align="right">降水</th><th align="right">概率</th><th align="left">级别</th><th align="left">天气</th>`)
			b.WriteString(`</tr></thead><tbody>`)
			for _, signal := range period.Signals {
				fmt.Fprintf(&b, `<tr style="border-top:1px solid #e5e7eb">`+
					`<td>%s</td><td align="right">%.1f mm</td><td align="right">%s</td><td>%s</td><td>%s</td></tr>`,
					formatTime(signal.Time, a.Location),
					signal.PrecipitationMM,
					probabilityText(signal.Probability),
					levelText(signal.Level),
					html.EscapeString(orUnknown(signal.Description)))
			}
			b.WriteString(`</tbody></table>`)
		}
	} else {
		fmt.Fprintf(&b, `<p style="margin:0 0 6px">观察窗口: %s ~ %s, 窗口内没有降水信号.</p>`,
			formatTime(a.WindowStart, a.Location), formatTime(a.WindowEnd, a.Location))
	}

	fmt.Fprintf(&b, `<p style="margin:14px 0 0;color:#6b7280;font-size:12px">生成时间: %s</p>`,
		formatTime(a.GeneratedAt, a.Location))
	b.WriteString(`</div>`)

	return b.String()
}

func formatTime(t time.Time, loc *time.Location) string {
	if t.IsZero() {
		return "未知"
	}
	if loc != nil {
		t = t.In(loc)
	}
	return t.Format(timeLayout)
}

func probabilityText(p int) string {
	if p < 0 {
		return "未知"
	}
	return fmt.Sprintf("%d%%", p)
}

func levelText(l weather.Level) string {
	switch l {
	case weather.LevelCertain:
		return "确定"
	case weather.LevelPossible:
		return "可能"
	default:
		return "无"
	}
}

// kindSuffix 在降水类型明确时给出 " 降雨" 这样的后缀.
func kindSuffix(kind weather.Kind) string {
	switch kind {
	case weather.KindRain, weather.KindSnow, weather.KindMixed:
		return " " + kind.Chinese()
	default:
		return ""
	}
}

func orUnknown(text string) string {
	if strings.TrimSpace(text) == "" {
		return "未知"
	}
	return text
}
