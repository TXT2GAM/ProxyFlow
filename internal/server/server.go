package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rfym21/ProxyFlow/internal/auth"
	"github.com/rfym21/ProxyFlow/internal/client"
	"github.com/rfym21/ProxyFlow/internal/models"
	"github.com/rfym21/ProxyFlow/internal/pool"
)

// Server HTTP代理服务器
type Server struct {
	pool         *pool.Pool     // 代理池
	client       *client.Client // HTTP客户端
	timeout      time.Duration  // 请求超时时间
	authUsername string         // 认证用户名
	authPassword string         // 认证密码
}

// NewServer 创建新的代理服务器
func NewServer(proxyPool *pool.Pool, timeout time.Duration, authUsername, authPassword string) *Server {
	return &Server{
		pool:         proxyPool,
		client:       client.NewClient(proxyPool, timeout),
		timeout:      timeout,
		authUsername: authUsername,
		authPassword: authPassword,
	}
}

// Start 启动代理服务器
func (s *Server) Start(port string) error {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("代理服务器正在端口 %s 上启动", port)
	log.Printf("使用 %d 个代理进行轮询", s.pool.Size())

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("接受连接时出错: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// handleConnection 处理TCP连接
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	log.Printf("来自 %s 的新连接", conn.RemoteAddr())

	// 读取第一行来判断请求类型
	reader := bufio.NewReader(conn)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("读取第一行时出错: %v", err)
		return
	}

	log.Printf("第一行: %q", firstLine)

	if strings.HasPrefix(firstLine, "CONNECT ") {
		log.Printf("处理 CONNECT 请求")
		s.handleConnectTCP(conn, reader, firstLine)
	} else {
		log.Printf("处理 HTTP 请求")
		s.handleHTTPTCP(conn, reader, firstLine)
	}
}

// handleConnectTCP 处理TCP CONNECT请求
func (s *Server) handleConnectTCP(conn net.Conn, reader *bufio.Reader, firstLine string) {
	// 解析CONNECT请求
	parts := strings.Fields(firstLine)
	if len(parts) < 2 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	destAddr := strings.TrimSpace(parts[1])
	if !strings.Contains(destAddr, ":") {
		destAddr += ":443"
	}

	log.Printf("CONNECT 目标: %s", destAddr)

	// 读取剩余的请求头并检查认证
	var authHeader string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("读取请求头时出错: %v", err)
			return
		}

		log.Printf("请求头: %q", line)

		// 检查Proxy-Authorization头
		if strings.HasPrefix(strings.ToLower(line), "proxy-authorization:") {
			authHeader = strings.TrimSpace(line[len("proxy-authorization:"):])
		}

		// 空行表示请求头结束
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// 检查认证
	if !s.checkAuthTCP(conn, authHeader) {
		return
	}

	// 尝试通过代理连接
	var upstreamConn net.Conn
	var err error

	// 重试机制：尝试所有代理
	for i := 0; i < s.pool.Size(); i++ {
		proxy := s.pool.NextProxy()
		log.Printf("尝试通过代理连接: %s (用户: %s)", proxy.Host, proxy.Username)
		upstreamConn, err = s.connectThroughProxy(destAddr, proxy)
		if err == nil {
			log.Printf("成功通过代理连接: %s", proxy.Host)
			break
		}
		log.Printf("通过代理 %s 连接失败: %v", proxy.Host, err)
	}

	if err != nil {
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer upstreamConn.Close()

	// 发送200 Connection Established响应
	_, err = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		return
	}

	// 双向数据转发
	go s.copyData(upstreamConn, conn)
	s.copyData(conn, upstreamConn)
}

// handleHTTPTCP 处理TCP HTTP请求
func (s *Server) handleHTTPTCP(conn net.Conn, reader *bufio.Reader, firstLine string) {
	// 解析HTTP请求行
	parts := strings.Fields(firstLine)
	if len(parts) < 3 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	method := parts[0]
	url := parts[1]
	version := parts[2]

	log.Printf("HTTP 请求: %s %s %s", method, url, version)

	// 读取请求头并检查认证
	headers := make(map[string]string)
	var authHeader string
	var contentLength int

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("读取请求头时出错: %v", err)
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break
		}

		// 解析头部
		if colonIndex := strings.Index(line, ":"); colonIndex > 0 {
			key := strings.TrimSpace(line[:colonIndex])
			value := strings.TrimSpace(line[colonIndex+1:])
			headers[strings.ToLower(key)] = value

			// 检查特殊头部
			if strings.ToLower(key) == "proxy-authorization" {
				authHeader = value
			} else if strings.ToLower(key) == "content-length" {
				if cl, err := fmt.Sscanf(value, "%d", &contentLength); cl != 1 || err != nil {
					contentLength = 0
				}
			}
		}
	}

	// 检查认证
	if !s.checkAuthTCP(conn, authHeader) {
		return
	}

	// 读取请求体
	var body []byte
	if contentLength > 0 {
		body = make([]byte, contentLength)
		_, err := io.ReadFull(reader, body)
		if err != nil {
			log.Printf("读取请求体时出错: %v", err)
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			return
		}
	}

	// 创建HTTP请求
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("创建请求时出错: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	// 设置请求头（排除代理相关头部）
	for key, value := range headers {
		if key != "proxy-authorization" && key != "proxy-connection" {
			req.Header.Set(key, value)
		}
	}

	// 通过代理发送请求
	var resp *http.Response
	for i := 0; i < s.pool.Size(); i++ {
		resp, err = s.client.Do(req)
		if err == nil {
			break
		}
		log.Printf("HTTP 请求失败，尝试下一个代理: %v", err)
	}

	if err != nil {
		log.Printf("所有代理都失败了: %v", err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer resp.Body.Close()

	// 发送响应状态行
	statusLine := fmt.Sprintf("HTTP/1.1 %d %s\r\n", resp.StatusCode, resp.Status[4:])
	conn.Write([]byte(statusLine))

	// 发送响应头
	for key, values := range resp.Header {
		for _, value := range values {
			headerLine := fmt.Sprintf("%s: %s\r\n", key, value)
			conn.Write([]byte(headerLine))
		}
	}

	// 发送空行分隔头部和正文
	conn.Write([]byte("\r\n"))

	// 发送响应体
	io.Copy(conn, resp.Body)
}

// connectThroughProxy 通过代理连接到目标地址
func (s *Server) connectThroughProxy(destAddr string, proxy models.ProxyInfo) (net.Conn, error) {
	// 连接到代理服务器
	proxyConn, err := net.Dial("tcp", proxy.Host)
	if err != nil {
		return nil, err
	}

	// 构建CONNECT请求
	// Host 头应该指向目标主机
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", destAddr, destAddr)
	connectReq += "Proxy-Connection: Keep-Alive\r\n"
	connectReq += "Content-Length: 0\r\n"

	// 添加代理认证
	if proxy.Username != "" {
		authValue := auth.EncodeBasicAuth(proxy.Username, proxy.Password)
		authHeader := fmt.Sprintf("Proxy-Authorization: %s\r\n", authValue)
		connectReq += authHeader
		log.Printf("为代理 %s 添加认证: %s", proxy.Host, authHeader)
	}

	connectReq += "\r\n"

	// 发送CONNECT请求
	log.Printf("向代理 %s 发送 CONNECT 请求:\n%s", proxy.Host, connectReq)
	_, err = proxyConn.Write([]byte(connectReq))
	if err != nil {
		proxyConn.Close()
		return nil, err
	}

	// 读取代理响应
	buffer := make([]byte, 1024)
	n, err := proxyConn.Read(buffer)
	if err != nil {
		proxyConn.Close()
		return nil, err
	}

	response := string(buffer[:n])
	log.Printf("来自代理 %s 的响应: %s", proxy.Host, response)
	if !strings.Contains(response, "200") {
		proxyConn.Close()
		return nil, fmt.Errorf("代理连接失败: %s", response)
	}

	return proxyConn, nil
}

// copyData 数据复制
func (s *Server) copyData(dst io.Writer, src io.Reader) {
	io.Copy(dst, src)
}

// checkAuthTCP 检查TCP连接的代理认证
func (s *Server) checkAuthTCP(conn net.Conn, authHeader string) bool {
	// 如果没有设置认证，则跳过检查
	if s.authUsername == "" && s.authPassword == "" {
		log.Printf("未配置认证，跳过认证检查")
		return true
	}

	log.Printf("需要认证: 用户名=%s", s.authUsername)
	log.Printf("认证头: %s", authHeader)

	// 检查是否有认证头
	if authHeader == "" {
		log.Printf("未找到 Proxy-Authorization 头")
		s.sendAuthRequiredTCP(conn)
		return false
	}

	// 解析Basic认证
	username, password, err := auth.DecodeBasicAuth(authHeader)
	if err != nil {
		log.Printf("认证解析失败: %v", err)
		s.sendAuthRequiredTCP(conn)
		return false
	}

	// 验证用户名和密码
	if username != s.authUsername || password != s.authPassword {
		log.Printf("认证失败: 用户名或密码错误")
		s.sendAuthRequiredTCP(conn)
		return false
	}

	return true
}

// sendAuthRequiredTCP 发送TCP认证要求响应
func (s *Server) sendAuthRequiredTCP(conn net.Conn) {
	response := "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"ProxyFlow\"\r\n\r\n"
	conn.Write([]byte(response))
}
