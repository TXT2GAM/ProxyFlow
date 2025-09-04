// Package pool 提供代理池管理功能。
//
// 本包实现了代理服务器池的管理，包括代理列表加载、解析、验证
// 和轮询分配等功能。支持从文件读取代理配置，自动解析代理URL
// 和认证信息，提供线程安全的代理轮询机制。
package pool

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rfym21/ProxyFlow/internal/models"
)

// Pool 代理池管理器。
//
// 通过API动态获取代理服务器连接信息，每次请求时获取一个新的随机代理。
// 提供线程安全的代理获取机制。
type Pool struct {
	apiURL     string        // 代理API端点URL
	httpClient *http.Client  // HTTP客户端
	mutex      sync.RWMutex  // 读写锁
}

// NewPool 创建新的代理池实例。
//
// 初始化用于从API动态获取代理的代理池。
//
// 参数：
//   - apiURL: 代理API端点URL
//
// 返回值：
//   - *Pool: 初始化完成的代理池实例
//   - error: 初始化错误，成功时为nil
func NewPool(apiURL string) (*Pool, error) {
	if apiURL == "" {
		return nil, fmt.Errorf("PROXY_API 配置不能为空")
	}

	pool := &Pool{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	log.Printf("代理池已初始化，API端点: %s", apiURL)
	return pool, nil
}

// fetchProxyFromAPI 从API获取代理。
//
// 向配置的API端点发送HTTP GET请求，获取一个随机代理URL。
// 解析返回的代理URL并返回代理信息结构。
//
// 返回值：
//   - *models.ProxyInfo: 从API获取的代理信息
//   - error: API请求或解析错误，成功时为nil
func (p *Pool) fetchProxyFromAPI() (*models.ProxyInfo, error) {
	p.mutex.RLock()
	apiURL := p.apiURL
	client := p.httpClient
	p.mutex.RUnlock()

	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("API请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取API响应失败: %v", err)
	}

	proxyURL := strings.TrimSpace(string(body))
	if proxyURL == "" {
		return nil, fmt.Errorf("API返回空的代理URL")
	}

	return p.parseProxy(proxyURL)
}

// parseProxy 解析代理字符串。
//
// 将代理URL字符串解析为ProxyInfo结构，提取协议、
// 主机地址和认证信息。仅支持HTTP和HTTPS协议。
//
// 参数：
//   - proxyStr: 代理URL字符串，格式为scheme://[user:pass@]host:port
//
// 返回值：
//   - *models.ProxyInfo: 解析后的代理信息结构
//   - error: 解析错误，成功时为nil  
func (p *Pool) parseProxy(proxyStr string) (*models.ProxyInfo, error) {
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil, fmt.Errorf("无效的代理URL: %v", err)
	}

	if proxyURL.Scheme != "http" && proxyURL.Scheme != "https" {
		return nil, fmt.Errorf("不支持的代理协议: %s", proxyURL.Scheme)
	}

	proxyInfo := &models.ProxyInfo{
		URL:  proxyURL,
		Host: proxyURL.Host,
	}

	// 提取认证信息
	if proxyURL.User != nil {
		proxyInfo.Username = proxyURL.User.Username()
		if password, ok := proxyURL.User.Password(); ok {
			proxyInfo.Password = password
		}
	}

	return proxyInfo, nil
}

// NextProxy 获取下一个代理服务器信息。
//
// 从API动态获取一个随机代理。每次调用都会向API请求一个新的代理。
// 提供线程安全的代理获取机制。
//
// 返回值：
//   - models.ProxyInfo: 从API获取的代理服务器信息
func (p *Pool) NextProxy() models.ProxyInfo {
	proxyInfo, err := p.fetchProxyFromAPI()
	if err != nil {
		log.Printf("从API获取代理失败: %v", err)
		return models.ProxyInfo{}
	}

	return *proxyInfo
}

// Size 获取代理池中的代理数量。
//
// 对于API模式，始终返回1，表示可以动态获取代理。
//
// 返回值：
//   - int: 始终返回1（表示API可用）
func (p *Pool) Size() int {
	return 1 // API动态模式，始终可用
}
