package client

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/rfym21/ProxyFlow/internal/auth"
	"github.com/rfym21/ProxyFlow/internal/models"
	"github.com/rfym21/ProxyFlow/internal/pool"
)

// proxyAuthTransport 代理认证传输层
type proxyAuthTransport struct {
	base      http.RoundTripper
	proxyAuth string
}

func (t *proxyAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Proxy-Authorization", t.proxyAuth)
	return t.base.RoundTrip(req)
}

// Client HTTP客户端连接池管理器
type Client struct {
	pool       *pool.Pool              // 代理池
	clients    map[string]*http.Client // 每个代理的HTTP客户端
	clientsMux sync.RWMutex            // 客户端映射锁
	timeout    time.Duration           // 请求超时时间
}

// NewClient 创建新的HTTP客户端管理器
func NewClient(proxyPool *pool.Pool, timeout time.Duration) *Client {
	return &Client{
		pool:    proxyPool,
		clients: make(map[string]*http.Client),
		timeout: timeout,
	}
}

// Do 执行HTTP请求
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// 获取下一个代理
	proxy := c.pool.NextProxy()
	if proxy.Host == "" {
		return nil, fmt.Errorf("没有可用的代理")
	}

	// 获取或创建对应的HTTP客户端
	client := c.getClient(proxy)

	// 执行请求
	return client.Do(req)
}

// getClient 获取或创建HTTP客户端
func (c *Client) getClient(proxy models.ProxyInfo) *http.Client {
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

// createClient 创建HTTP客户端
func (c *Client) createClient(proxy models.ProxyInfo) *http.Client {
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
		authValue := auth.EncodeBasicAuth(proxy.Username, proxy.Password)
		rt = &proxyAuthTransport{base: transport, proxyAuth: authValue}
	}

	// 创建HTTP客户端
	return &http.Client{
		Transport: rt,
		Timeout:   c.timeout,
	}
}

// Close 清理客户端连接池
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
