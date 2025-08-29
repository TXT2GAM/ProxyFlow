package auth

import (
	"encoding/base64"
	"fmt"
)

/**
 * EncodeBasicAuth 编码Basic认证字符串
 * @param {string} username - 用户名
 * @param {string} password - 密码
 * @returns {string} 编码后的Basic认证字符串
 */
func EncodeBasicAuth(username, password string) string {
	if username == "" {
		return ""
	}
	auth := fmt.Sprintf("%s:%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))
	return "Basic " + encoded
}

/**
 * DecodeBasicAuth 解码Basic认证字符串
 * @param {string} authHeader - 认证头部值
 * @returns {string, string, error} 用户名、密码和错误信息
 */
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
	for i := 0; i < len(credentials); i++ {
		if credentials[i] == ':' {
			return credentials[:i], credentials[i+1:], nil
		}
	}

	return "", "", fmt.Errorf("认证格式无效")
}
