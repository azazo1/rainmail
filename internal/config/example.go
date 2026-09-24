package config

import _ "embed"

// exampleTOML 是嵌入二进制的示例配置模板, 与源码一起分发.
//
//go:embed example.toml
var exampleTOML string

// ExampleTemplate 返回带注释的示例配置文本, 供 rainmail config init 使用.
func ExampleTemplate() string { return exampleTOML }
