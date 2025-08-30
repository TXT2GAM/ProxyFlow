// Package auth 提供HTTP Basic认证处理功能。
//
// 本包实现了HTTP Basic认证的编码和解码功能，用于处理代理服务器
// 的认证逻辑。支持标准的Base64编码格式，提供安全的认证字符串
// 生成和解析功能。
package auth

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// EncodeBasicAuth 编码HTTP Basic认证字符串。
//
// 将用户名和密码编码为HTTP Basic认证格式的字符串。
// 如果用户名为空，则返回空字符串表示不需要认证。
//
// 参数：
//   - username: 认证用户名
//   - password: 认证密码
//
// 返回值：
//   - string: 编码后的Basic认证字符串，格式为"Basic <base64>"
func EncodeBasicAuth(username, password string) string {
	if username == "" {
		return ""
	}
	auth := fmt.Sprintf("%s:%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))
	return "Basic " + encoded
}

// DecodeBasicAuth 解码HTTP Basic认证字符串。
//
// 解析HTTP Basic认证头，提取用户名和密码信息。
// 支持标准的"Basic <base64>"格式。
//
// 参数：
//   - authHeader: HTTP认证头字符串
//
// 返回值：
//   - string: 解析出的用户名
//   - string: 解析出的密码
//   - error: 解析错误，如果成功则为nil
func DecodeBasicAuth(authHeader string) (string, string, error) {
	if authHeader == "" {
		return "", "", fmt.Errorf("认证头为空")
	}

	// 检查Basic前缀
	const basicPrefix = "Basic "
	if len(authHeader) < len(basicPrefix) || authHeader[:len(basicPrefix)] != basicPrefix {
		return "", "", fmt.Errorf("不是Basic认证")
	}

	// 解码Base64
	encoded := authHeader[len(basicPrefix):]
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", fmt.Errorf("Base64解码失败: %v", err)
	}

	// 分割用户名和密码
	credentials := string(decoded)
	parts := strings.SplitN(credentials, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("认证格式无效")
	}
	
	return parts[0], parts[1], nil
}
