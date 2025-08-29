package proxy

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

/**
 * HTTP代理服务器
 */
type Server struct {
	pool         *Pool         // 代理池
	client       *Client       // HTTP客户端
	timeout      time.Duration // 请求超时时间
	authUsername string        // 认证用户名
	authPassword string        // 认证密码
}

/**
 * 创建新的代理服务器
 * @param {*Pool} pool - 代理池
 * @param {time.Duration} timeout - 请求超时时间
 * @param {string} authUsername - 认证用户名
 * @param {string} authPassword - 认证密码
 * @returns {*Server} 服务器实例
 */
func NewServer(pool *Pool, timeout time.Duration, authUsername, authPassword string) *Server {
	return &Server{
		pool:         pool,
		client:       NewClient(pool, timeout),
		timeout:      timeout,
		authUsername: authUsername,
		authPassword: authPassword,
	}
}

/**
 * 启动代理服务器
 * @param {string} port - 监听端口
 * @returns {error} 错误信息
 */
func (s *Server) Start(port string) error {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("Proxy server starting on port %s", port)
	log.Printf("Using %d proxies in rotation", s.pool.Size())

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

/**
 * 处理TCP连接
 * @param {net.Conn} conn - TCP连接
 */
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	log.Printf("New connection from %s", conn.RemoteAddr())

	// 读取第一行来判断请求类型
	reader := bufio.NewReader(conn)
	firstLine, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("Error reading first line: %v", err)
		return
	}

	log.Printf("First line: %q", firstLine)

	if strings.HasPrefix(firstLine, "CONNECT ") {
		log.Printf("Handling CONNECT request")
		s.handleConnectTCP(conn, reader, firstLine)
	} else {
		log.Printf("Handling HTTP request")
		s.handleHTTPTCP(conn, reader, firstLine)
	}
}

/**
 * 处理TCP CONNECT请求
 * @param {net.Conn} conn - TCP连接
 * @param {*bufio.Reader} reader - 缓冲读取器
 * @param {string} firstLine - 第一行请求
 */
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

	log.Printf("CONNECT target: %s", destAddr)

	// 读取剩余的请求头并检查认证
	var authHeader string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error reading header: %v", err)
			return
		}

		log.Printf("Header: %q", line)

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
		log.Printf("Trying to connect through proxy: %s (user: %s)", proxy.Host, proxy.Username)
		upstreamConn, err = s.connectThroughProxy(destAddr, proxy)
		if err == nil {
			log.Printf("Successfully connected through proxy: %s", proxy.Host)
			break
		}
		log.Printf("Failed to connect through proxy %s: %v", proxy.Host, err)
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

/**
 * 处理TCP HTTP请求
 * @param {net.Conn} conn - TCP连接
 * @param {*bufio.Reader} reader - 缓冲读取器
 * @param {string} firstLine - 第一行请求
 */
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

	log.Printf("HTTP request: %s %s %s", method, url, version)

	// 读取请求头并检查认证
	headers := make(map[string]string)
	var authHeader string
	var contentLength int

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error reading header: %v", err)
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
			log.Printf("Error reading body: %v", err)
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			return
		}
	}

	// 创建HTTP请求
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("Error creating request: %v", err)
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
		log.Printf("HTTP request failed, trying next proxy: %v", err)
	}

	if err != nil {
		log.Printf("All proxies failed: %v", err)
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

/**
 * 通过代理连接到目标地址
 * @param {string} destAddr - 目标地址
 * @param {ProxyInfo} proxy - 代理信息
 * @returns {net.Conn} 连接
 * @returns {error} 错误信息
 */
func (s *Server) connectThroughProxy(destAddr string, proxy ProxyInfo) (net.Conn, error) {
	// 连接到代理服务器
	proxyConn, err := net.DialTimeout("tcp", proxy.Host, s.timeout)
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
		auth := fmt.Sprintf("%s:%s", proxy.Username, proxy.Password)
		encoded := base64.StdEncoding.EncodeToString([]byte(auth))
		authHeader := fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", encoded)
		connectReq += authHeader
		log.Printf("Adding proxy auth for %s: %s", proxy.Host, authHeader)
	}

	connectReq += "\r\n"

	// 发送CONNECT请求
	log.Printf("Sending CONNECT request to proxy %s:\n%s", proxy.Host, connectReq)
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
	log.Printf("Proxy response from %s: %s", proxy.Host, response)
	if !strings.Contains(response, "200") {
		proxyConn.Close()
		return nil, fmt.Errorf("proxy connection failed: %s", response)
	}

	return proxyConn, nil
}

/**
 * 数据复制
 * @param {io.Writer} dst - 目标写入器
 * @param {io.Reader} src - 源读取器
 */
func (s *Server) copyData(dst io.Writer, src io.Reader) {
	io.Copy(dst, src)
}

/**
 * 检查TCP连接的代理认证
 * @param {net.Conn} conn - TCP连接
 * @param {string} authHeader - 认证头内容
 * @returns {bool} 认证是否通过
 */
func (s *Server) checkAuthTCP(conn net.Conn, authHeader string) bool {
	// 如果没有设置认证，则跳过检查
	if s.authUsername == "" && s.authPassword == "" {
		log.Printf("No auth configured, skipping auth check")
		return true
	}

	log.Printf("Auth required: username=%s", s.authUsername)
	log.Printf("Auth header: %s", authHeader)

	// 检查是否有认证头
	if authHeader == "" {
		log.Printf("No Proxy-Authorization header found")
		s.sendAuthRequiredTCP(conn)
		return false
	}

	// 解析Basic认证
	if !strings.HasPrefix(authHeader, "Basic ") {
		s.sendAuthRequiredTCP(conn)
		return false
	}

	// 解码Base64
	encoded := authHeader[6:] // 去掉"Basic "前缀
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		s.sendAuthRequiredTCP(conn)
		return false
	}

	// 分割用户名和密码
	credentials := string(decoded)
	parts := strings.SplitN(credentials, ":", 2)
	if len(parts) != 2 {
		s.sendAuthRequiredTCP(conn)
		return false
	}

	username, password := parts[0], parts[1]

	// 验证用户名和密码
	if username != s.authUsername || password != s.authPassword {
		s.sendAuthRequiredTCP(conn)
		return false
	}

	return true
}

/**
 * 发送TCP认证要求响应
 * @param {net.Conn} conn - TCP连接
 */
func (s *Server) sendAuthRequiredTCP(conn net.Conn) {
	response := "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"ProxyFlow\"\r\n\r\n"
	conn.Write([]byte(response))
}
