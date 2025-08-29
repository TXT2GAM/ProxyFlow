package config

import (
	"os"
	"strconv"
	"time"
)

// Config 应用配置结构
type Config struct {
	ProxyPort      string        // 代理服务监听端口
	ProxyFile      string        // 代理文件路径
	PoolSize       int           // 连接池大小
	RequestTimeout time.Duration // 请求超时时间
	AuthUsername   string        // 代理服务器认证用户名
	AuthPassword   string        // 代理服务器认证密码
}

// Load 加载配置从环境变量
func Load() *Config {
	return &Config{
		ProxyPort:      getEnv("PROXY_PORT", "8080"),
		ProxyFile:      getEnv("PROXY_FILE", "proxy.txt"),
		PoolSize:       getEnvInt("POOL_SIZE", 100),
		RequestTimeout: time.Duration(getEnvInt("REQUEST_TIMEOUT", 30)) * time.Second,
		AuthUsername:   getEnv("AUTH_USERNAME", ""),
		AuthPassword:   getEnv("AUTH_PASSWORD", ""),
	}
}

// getEnv 获取环境变量字符串值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt 获取环境变量整数值
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
