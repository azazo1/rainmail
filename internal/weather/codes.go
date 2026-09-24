package weather

// IsRain 判断天气码是否代表降雨.
func IsRain(cs CodeSystem, code int) bool {
	if code < 0 {
		return false
	}
	switch cs {
	case CodeWMO:
		return isRainWMO(code)
	case CodeOWM:
		// 2xx 雷暴, 3xx 毛毛雨, 5xx 降雨.
		return code/100 == 2 || code/100 == 3 || code/100 == 5
	case CodeQWeather:
		return qweatherRain(code)
	case CodeSeniverse:
		return seniverseRain(code)
	default:
		return false
	}
}

// IsSnow 判断天气码是否代表降雪.
func IsSnow(cs CodeSystem, code int) bool {
	if code < 0 {
		return false
	}
	switch cs {
	case CodeWMO:
		return isSnowWMO(code)
	case CodeOWM:
		// 6xx 降雪.
		return code/100 == 6
	case CodeQWeather:
		return qweatherSnow(code)
	case CodeSeniverse:
		return seniverseSnow(code)
	default:
		return false
	}
}

// Describe 返回天气码的中文描述, 未知码返回空串.
func Describe(cs CodeSystem, code int) string {
	if code < 0 {
		return ""
	}
	switch cs {
	case CodeWMO:
		return describeWMO(code)
	case CodeOWM:
		return describeOWM(code)
	case CodeQWeather:
		return describeQWeather(code)
	case CodeSeniverse:
		return describeSeniverse(code)
	default:
		return ""
	}
}

// WMO 天气码取自 Open-Meteo 文档.
func isRainWMO(code int) bool {
	switch code {
	case 51, 53, 55, 56, 57, 61, 63, 65, 66, 67, 80, 81, 82, 95, 96, 99:
		return true
	default:
		return false
	}
}

func isSnowWMO(code int) bool {
	switch code {
	case 71, 73, 75, 77, 85, 86:
		return true
	default:
		return false
	}
}

func describeWMO(code int) string {
	switch code {
	case 0:
		return "晴"
	case 1:
		return "基本晴朗"
	case 2:
		return "局部多云"
	case 3:
		return "阴"
	case 45, 48:
		return "雾"
	case 51:
		return "小毛毛雨"
	case 53:
		return "毛毛雨"
	case 55:
		return "浓毛毛雨"
	case 56, 57:
		return "冻毛毛雨"
	case 61:
		return "小雨"
	case 63:
		return "中雨"
	case 65:
		return "大雨"
	case 66, 67:
		return "冻雨"
	case 71:
		return "小雪"
	case 73:
		return "中雪"
	case 75:
		return "大雪"
	case 77:
		return "米雪"
	case 80:
		return "小阵雨"
	case 81:
		return "阵雨"
	case 82:
		return "强阵雨"
	case 85, 86:
		return "阵雪"
	case 95:
		return "雷阵雨"
	case 96, 99:
		return "雷阵雨伴冰雹"
	default:
		return ""
	}
}

func describeOWM(code int) string {
	switch code {
	case 200, 201, 202, 210, 211, 212, 221, 230, 231, 232:
		return "雷雨"
	case 300, 301, 302, 310, 311, 312, 313, 314, 321:
		return "毛毛雨"
	case 500, 501, 502, 503, 504, 511, 520, 521, 522, 531:
		return "降雨"
	case 600, 601, 602, 611, 612, 613, 615, 616, 620, 621, 622:
		return "降雪"
	case 701, 711, 721, 731, 741, 751, 761, 762, 771, 781:
		return "雾霾或大风"
	case 800:
		return "晴"
	case 801, 802, 803, 804:
		return "多云"
	default:
		return ""
	}
}

// 和风天气图标码: 3xx 为降雨, 4xx 为降雪.
func qweatherRain(code int) bool {
	switch {
	case code >= 300 && code <= 318:
		return true
	case code == 350, code == 351, code == 399:
		return true
	// 404 雨夹雪, 405 雨雪天气, 406 阵雨夹雪, 456 阵雨夹雪, 457 阵雪.
	case code == 404, code == 405, code == 406, code == 456:
		return true
	default:
		return false
	}
}

func qweatherSnow(code int) bool {
	switch {
	case code >= 400 && code <= 403:
		return true
	case code >= 407 && code <= 410:
		return true
	case code == 457, code == 499:
		return true
	// 雨夹雪同时计入降雨.
	case code == 404, code == 405, code == 406, code == 456:
		return true
	default:
		return false
	}
}

func describeQWeather(code int) string {
	switch code {
	case 300, 350:
		return "阵雨"
	case 301, 351:
		return "强阵雨"
	case 302:
		return "雷阵雨"
	case 303:
		return "强雷阵雨"
	case 304:
		return "雷阵雨伴冰雹"
	case 305:
		return "小雨"
	case 306:
		return "中雨"
	case 307:
		return "大雨"
	case 308:
		return "极端降雨"
	case 309:
		return "细雨"
	case 310:
		return "暴雨"
	case 311:
		return "大暴雨"
	case 312:
		return "特大暴雨"
	case 313:
		return "冻雨"
	case 314:
		return "小到中雨"
	case 315:
		return "中到大雨"
	case 316:
		return "大到暴雨"
	case 317:
		return "暴雨到大暴雨"
	case 318:
		return "大暴雨到特大暴雨"
	case 399:
		return "雨"
	case 400:
		return "小雪"
	case 401:
		return "中雪"
	case 402:
		return "大雪"
	case 403:
		return "暴雪"
	case 404:
		return "雨夹雪"
	case 405:
		return "雨雪天气"
	case 406:
		return "阵雨夹雪"
	case 407:
		return "阵雪"
	case 408:
		return "小到中雪"
	case 409:
		return "中到大雪"
	case 410:
		return "大到暴雪"
	case 456:
		return "阵雨夹雪"
	case 457:
		return "阵雪"
	case 499:
		return "雪"
	default:
		return ""
	}
}

// 心知天气的天气现象代码兼容字符串与数字两种形态, 这里统一按数字处理.
func seniverseRain(code int) bool {
	switch code {
	case 3, 4, 6, 7, 8, 9, 10, 11, 18, 21, 22, 23, 24, 25:
		return true
	// 5 雨夹雪.
	case 5:
		return true
	default:
		return false
	}
}

func seniverseSnow(code int) bool {
	switch code {
	case 12, 13, 14, 15, 16, 26, 27, 28:
		return true
	case 5:
		return true
	default:
		return false
	}
}

func describeSeniverse(code int) string {
	switch code {
	case 0:
		return "晴"
	case 1:
		return "多云"
	case 2:
		return "阴"
	case 3:
		return "阵雨"
	case 4:
		return "雷阵雨"
	case 5:
		return "雨夹雪"
	case 6:
		return "小雨"
	case 7:
		return "中雨"
	case 8:
		return "大雨"
	case 9:
		return "暴雨"
	case 10:
		return "大暴雨"
	case 11:
		return "特大暴雨"
	case 12:
		return "阵雪"
	case 13:
		return "小雪"
	case 14:
		return "中雪"
	case 15:
		return "大雪"
	case 16:
		return "暴雪"
	case 17:
		return "雾"
	case 18:
		return "冻雨"
	case 19:
		return "霾"
	case 20:
		return "沙尘暴"
	case 21:
		return "小到中雨"
	case 22:
		return "中到大雨"
	case 23:
		return "大到暴雨"
	case 24:
		return "暴雨到大暴雨"
	case 25:
		return "大暴雨到特大暴雨"
	case 26:
		return "小到中雪"
	case 27:
		return "中到大雪"
	case 28:
		return "大到暴雪"
	default:
		return ""
	}
}
