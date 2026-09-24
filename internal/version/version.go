// Package version 保存构建期注入的版本信息.
package version

import (
	"fmt"
	"runtime"
)

// 以下变量由构建脚本通过 -ldflags "-X <pkg>.Version=..." 注入,
// 直接 go run / go build 时保留默认值.
var (
	Version = "0.1.0"
	Commit  = "unknown"
	Date    = "unknown"
)

// String 返回人类可读的完整版本描述.
func String() string {
	return fmt.Sprintf("rainmail %s (commit %s, built %s, %s)", Version, Commit, Date, runtime.Version())
}
