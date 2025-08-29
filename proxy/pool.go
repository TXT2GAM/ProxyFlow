package proxy

import (
	"bufio"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
)

/**
 * 代理信息结构
 */
type ProxyInfo struct {
	URL      *url.URL // 代理URL
	Host     string   // 代理主机地址
	Username string   // 认证用户名
	Password string   // 认证密码
}

/**
 * 代理池管理器
 */
type Pool struct {
	proxies []ProxyInfo // 代理列表
	current int64       // 当前索引（原子操作）
}

/**
 * 创建新的代理池
 * @param {string} filename - 代理文件路径
 * @returns {*Pool} 代理池实例
 * @returns {error} 错误信息
 */
func NewPool(filename string) (*Pool, error) {
	pool := &Pool{
		proxies: make([]ProxyInfo, 0),
		current: -1,
	}

	err := pool.loadProxies(filename)
	if err != nil {
		return nil, err
	}

	if len(pool.proxies) == 0 {
		return nil, fmt.Errorf("no valid proxies found in %s", filename)
	}

	log.Printf("Loaded %d proxies from %s", len(pool.proxies), filename)
	return pool, nil
}

/**
 * 从文件加载代理列表
 * @param {string} filename - 代理文件路径
 * @returns {error} 错误信息
 */
func (p *Pool) loadProxies(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open proxy file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		proxyInfo, err := p.parseProxy(line)
		if err != nil {
			log.Printf("Warning: invalid proxy at line %d: %v", lineNum, err)
			continue
		}

		p.proxies = append(p.proxies, *proxyInfo)
	}

	return scanner.Err()
}

/**
 * 解析代理字符串
 * @param {string} proxyStr - 代理字符串
 * @returns {*ProxyInfo} 代理信息
 * @returns {error} 错误信息
 */
func (p *Pool) parseProxy(proxyStr string) (*ProxyInfo, error) {
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %v", err)
	}

	if proxyURL.Scheme != "http" && proxyURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}

	proxyInfo := &ProxyInfo{
		URL:  proxyURL,
		Host: proxyURL.Host,
	}

	// 提取认证信息
	if proxyURL.User != nil {
		proxyInfo.Username = proxyURL.User.Username()
		if password, ok := proxyURL.User.Password(); ok {
			proxyInfo.Password = password
		}
		log.Printf("Parsed proxy: Host=%s, Username=%s, Password=%s",
			proxyInfo.Host, proxyInfo.Username, proxyInfo.Password)
	} else {
		log.Printf("No authentication info found in proxy URL: %s", proxyStr)
	}

	return proxyInfo, nil
}

/**
 * 获取下一个代理（轮询方式）
 * @returns {ProxyInfo} 代理信息
 */
func (p *Pool) NextProxy() ProxyInfo {
	if len(p.proxies) == 0 {
		return ProxyInfo{}
	}

	// 原子操作递增索引
	index := atomic.AddInt64(&p.current, 1)
	// 取模实现循环轮询
	actualIndex := index % int64(len(p.proxies))

	return p.proxies[actualIndex]
}

/**
 * 获取代理池大小
 * @returns {int} 代理数量
 */
func (p *Pool) Size() int {
	return len(p.proxies)
}
