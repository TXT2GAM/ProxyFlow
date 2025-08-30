// Package models 定义项目中使用的数据结构。
//
// 本包包含了代理服务器项目中使用的所有数据模型和结构定义，
// 主要用于表示代理信息、配置参数等核心数据类型。
package models

import "net/url"

// ProxyInfo 代理服务器信息结构。
//
// 存储单个代理服务器的连接信息，包括网络地址、
// 认证凭据和连接参数等。
type ProxyInfo struct {
	URL      *url.URL // 代理URL
	Host     string   // 代理主机地址
	Username string   // 认证用户名
	Password string   // 认证密码
}
