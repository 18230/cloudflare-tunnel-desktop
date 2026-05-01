package main

import (
	"regexp"
	"strings"
)

var jwtLikePattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)

// RedactSensitiveText 移除日志和错误文本中的常见敏感片段。
func RedactSensitiveText(value string) string {
	value = jwtLikePattern.ReplaceAllString(value, "[已隐藏 token]")
	value = strings.ReplaceAll(value, "Authorization: Bearer ", "Authorization: Bearer [已隐藏] ")
	return value
}
