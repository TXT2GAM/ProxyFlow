package models

import "net/url"

// ProxyInfo 代理信息结构
type ProxyInfo struct {
	URL      *url.URL // 代理URL
	Host     string   // 代理主机地址
	Username string   // 认证用户名
	Password string   // 认证密码
}
