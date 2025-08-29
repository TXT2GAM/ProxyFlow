package proxy

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type proxyAuthTransport struct {
	base      http.RoundTripper
	proxyAuth string
}

func (t *proxyAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.base == nil {
		t.base = http.DefaultTransport
	}
	req2 := req.Clone(req.Context())
	if t.proxyAuth != "" {
		req2.Header.Set("Proxy-Authorization", t.proxyAuth)
	}
	return t.base.RoundTrip(req2)
}

/**
 * HTTP客户端连接池管理器
 */
type Client struct {
	pool       *Pool                   // 代理池
	clients    map[string]*http.Client // 每个代理的HTTP客户端
	clientsMux sync.RWMutex            // 客户端映射锁
	timeout    time.Duration           // 请求超时时间
}

/**
 * 创建新的HTTP客户端管理器
 * @param {*Pool} pool - 代理池
 * @param {time.Duration} timeout - 请求超时时间
 * @returns {*Client} 客户端实例
 */
func NewClient(pool *Pool, timeout time.Duration) *Client {
	return &Client{
		pool:    pool,
		clients: make(map[string]*http.Client),
		timeout: timeout,
	}
}

/**
 * 执行HTTP请求
 * @param {*http.Request} req - HTTP请求
 * @returns {*http.Response} HTTP响应
 * @returns {error} 错误信息
 */
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// 获取下一个代理
	proxy := c.pool.NextProxy()
	if proxy.Host == "" {
		return nil, fmt.Errorf("no proxy available")
	}

	// 获取或创建对应的HTTP客户端
	client := c.getClient(proxy)

	// 执行请求
	return client.Do(req)
}

/**
 * 获取或创建HTTP客户端
 * @param {ProxyInfo} proxy - 代理信息
 * @returns {*http.Client} HTTP客户端
 */
func (c *Client) getClient(proxy ProxyInfo) *http.Client {
	proxyKey := proxy.Host

	// 先尝试读锁获取现有客户端
	c.clientsMux.RLock()
	if client, exists := c.clients[proxyKey]; exists {
		c.clientsMux.RUnlock()
		return client
	}
	c.clientsMux.RUnlock()

	// 使用写锁创建新客户端
	c.clientsMux.Lock()
	defer c.clientsMux.Unlock()

	// 双重检查，防止并发创建
	if client, exists := c.clients[proxyKey]; exists {
		return client
	}

	// 创建新的HTTP客户端
	client := c.createClient(proxy)
	c.clients[proxyKey] = client

	return client
}

/**
 * 创建HTTP客户端
 * @param {ProxyInfo} proxy - 代理信息
 * @returns {*http.Client} HTTP客户端
 */
func (c *Client) createClient(proxy ProxyInfo) *http.Client {
	// 创建代理URL
	proxyURL := &url.URL{
		Scheme: proxy.URL.Scheme,
		Host:   proxy.Host,
	}

	// 添加认证信息
	if proxy.Username != "" {
		proxyURL.User = url.UserPassword(proxy.Username, proxy.Password)
	}

	// 创建传输层配置
	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}

	// 如果需要认证，包一层添加Proxy-Authorization
	var rt http.RoundTripper = transport
	if proxy.Username != "" {
		auth := fmt.Sprintf("%s:%s", proxy.Username, proxy.Password)
		encoded := base64.StdEncoding.EncodeToString([]byte(auth))
		rt = &proxyAuthTransport{base: transport, proxyAuth: "Basic " + encoded}
	}

	// 创建HTTP客户端
	return &http.Client{
		Transport: rt,
		Timeout:   c.timeout,
	}
}

/**
 * 清理客户端连接池
 */
func (c *Client) Close() {
	c.clientsMux.Lock()
	defer c.clientsMux.Unlock()

	for _, client := range c.clients {
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	c.clients = make(map[string]*http.Client)
}
