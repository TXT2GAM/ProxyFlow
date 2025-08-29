package pool

import (
	"bufio"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync/atomic"

	"github.com/rfym21/ProxyFlow/internal/models"
)

// Pool 代理池管理器
type Pool struct {
	proxies []models.ProxyInfo // 代理列表
	current int64              // 当前索引（原子操作）
}

// NewPool 创建新的代理池
func NewPool(filename string) (*Pool, error) {
	pool := &Pool{
		proxies: make([]models.ProxyInfo, 0),
		current: 0,
	}

	if err := pool.loadProxies(filename); err != nil {
		return nil, err
	}

	if len(pool.proxies) == 0 {
		return nil, fmt.Errorf("在 %s 中未找到有效的代理", filename)
	}

	log.Printf("从 %s 加载了 %d 个代理", filename, len(pool.proxies))
	return pool, nil
}

// loadProxies 从文件加载代理列表
func (p *Pool) loadProxies(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
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
			log.Printf("警告: 第 %d 行代理配置无效: %v", lineNum, err)
			continue
		}

		p.proxies = append(p.proxies, *proxyInfo)
	}

	return scanner.Err()
}

// parseProxy 解析代理字符串
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
		log.Printf("解析代理: 主机=%s, 用户名=%s, 密码=%s",
			proxyInfo.Host, proxyInfo.Username, proxyInfo.Password)
	} else {
		log.Printf("代理URL中未找到认证信息: %s", proxyStr)
	}

	return proxyInfo, nil
}

// NextProxy 获取下一个代理（轮询方式）
func (p *Pool) NextProxy() models.ProxyInfo {
	if len(p.proxies) == 0 {
		return models.ProxyInfo{}
	}

	// 原子操作获取下一个索引
	index := atomic.AddInt64(&p.current, 1) % int64(len(p.proxies))
	return p.proxies[index]
}

// Size 获取代理池大小
func (p *Pool) Size() int {
	return len(p.proxies)
}
