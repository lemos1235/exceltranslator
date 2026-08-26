// Package version 提供应用的版本信息。
//
// 版本号维护在仓库根目录的 VERSION 文件中，格式为 <语义化版本>+<发布号>，
// 例如 1.0.0+2。构建脚本会读取该文件并通过
// -ldflags "-X exceltranslator/pkg/version.Full=1.0.0+2" 注入到二进制中。
package version

import "strings"

// Full 完整版本号，形如 1.0.0+2；未经构建脚本注入时为 dev。
var Full = "dev"

// Short 返回不含发布号的版本号，例如 1.0.0。
func Short() string {
	if name, _, ok := strings.Cut(Full, "+"); ok {
		return name
	}
	return Full
}

// Build 返回发布号，例如 2；不存在时返回空字符串。
func Build() string {
	if _, build, ok := strings.Cut(Full, "+"); ok {
		return build
	}
	return ""
}
