package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Color constants for better output readability
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
)

// TestConfig represents test configuration
type TestConfig struct {
	ProxyHost string
	ProxyPort string
	Username  string
	Password  string
	Timeout   time.Duration
}

// TestResult represents test result
type TestResult struct {
	Name    string
	Success bool
	Message string
	IP      string
}

// HttpBinResponse represents httpbin.org response format
type HttpBinResponse struct {
	Origin string `json:"origin"`
}

// Get environment variable or default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Load configuration from environment variables
func loadConfig() TestConfig {
	return TestConfig{
		ProxyHost: getEnv("PROXY_HOST", "127.0.0.1"),
		ProxyPort: getEnv("PROXY_PORT", "8282"),
		Username:  os.Getenv("AUTH_USERNAME"),
		Password:  os.Getenv("AUTH_PASSWORD"),
		Timeout:   30 * time.Second,
	}
}

// Build proxy URL with authentication if provided
func (c *TestConfig) buildProxyURL() string {
	if c.Username != "" && c.Password != "" {
		return fmt.Sprintf("http://%s:%s@%s:%s", c.Username, c.Password, c.ProxyHost, c.ProxyPort)
	} else if c.Username != "" {
		return fmt.Sprintf("http://%s@%s:%s", c.Username, c.ProxyHost, c.ProxyPort)
	}
	return fmt.Sprintf("http://%s:%s", c.ProxyHost, c.ProxyPort)
}

// Check if proxy server is running and accessible
func checkProxyServer(host, port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Execute HTTP test through proxy
func testHTTP(proxyURL, testURL string, timeout time.Duration) TestResult {
	result := TestResult{Name: "HTTP", Success: false}

	// Parse proxy URL
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		result.Message = fmt.Sprintf("🚫 Proxy URL parse failed: %v", err)
		return result
	}

	// Create custom transport with proxy
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxy),
	}

	// Create HTTP client with proxy transport
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	// Send HTTP request through proxy
	resp, err := client.Get(testURL)
	if err != nil {
		result.Message = fmt.Sprintf("📡 Request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Message = fmt.Sprintf("📖 Response read failed: %v", err)
		return result
	}

	// Parse JSON response
	var httpbinResp HttpBinResponse
	if err := json.Unmarshal(body, &httpbinResp); err != nil {
		result.Message = fmt.Sprintf("🔧 Response format error: %v", err)
		return result
	}

	if httpbinResp.Origin != "" {
		result.Success = true
		result.Message = "🎯 Test successful"
		result.IP = httpbinResp.Origin
	} else {
		result.Message = "🔍 IP information not found in response"
	}

	return result
}

// Print colored text (cross-platform compatible)
func printColored(color, text string) {
	fmt.Print(text)
}

// Print test result with emoji indicators
func printResult(result TestResult) {
	if result.Success {
		fmt.Printf("✓ %s test: SUCCESS", result.Name)
		if result.IP != "" {
			fmt.Printf(" (IP: %s)", result.IP)
		}
		fmt.Println()
	} else {
		fmt.Printf("✗ %s test: FAILED - %s\n", result.Name, result.Message)
	}
}

// Show help information
func showHelp() {
	fmt.Println("ProxyFlow Connectivity Testing Tool")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go run test-proxy.go")
	fmt.Println("  or compile and run: ./test-proxy")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  PROXY_HOST        Proxy host address (default: 127.0.0.1)")
	fmt.Println("  PROXY_PORT        Proxy port (default: 8282)")
	fmt.Println("  AUTH_USERNAME     Authentication username")
	fmt.Println("  AUTH_PASSWORD     Authentication password")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  go run test-proxy.go")
	fmt.Println("  PROXY_PORT=8080 go run test-proxy.go")
	fmt.Println("  AUTH_USERNAME=admin AUTH_PASSWORD=123456 go run test-proxy.go")
}

func main() {
	// 检查命令行参数
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		showHelp()
		return
	}

	// 加载配置
	config := loadConfig()

	// 打印标题
	fmt.Println()
	fmt.Println("=== ProxyFlow Connectivity Test ===")
	fmt.Printf("Proxy Address: %s:%s\n", config.ProxyHost, config.ProxyPort)
	
	if config.Username != "" {
		fmt.Printf("Authentication: %s\n", config.Username)
	} else {
		fmt.Println("Authentication: None")
	}
	fmt.Println()

	// 检查代理服务器状态
	fmt.Print("Checking proxy server status: ")
	if !checkProxyServer(config.ProxyHost, config.ProxyPort) {
		fmt.Println("❌ Cannot connect to proxy server")
		fmt.Printf("🔧 Please ensure ProxyFlow service is running on port %s\n", config.ProxyPort)
		os.Exit(1)
	}
	fmt.Println("✅ Running")
	fmt.Println()

	// 构建代理URL
	proxyURL := config.buildProxyURL()

	// 测试URL
	httpTestURL := "http://httpbin.org/ip"
	httpsTestURL := "https://httpbin.org/ip"

	var results []TestResult
	
	// HTTP测试
	fmt.Println("🌐 Running HTTP test...")
	httpResult := testHTTP(proxyURL, httpTestURL, config.Timeout)
	results = append(results, httpResult)
	printResult(httpResult)
	fmt.Println()

	// HTTPS测试
	fmt.Println("🔒 Running HTTPS test...")
	httpsResult := testHTTP(proxyURL, httpsTestURL, config.Timeout)
	httpsResult.Name = "HTTPS" // Ensure correct test name
	results = append(results, httpsResult)
	printResult(httpsResult)
	fmt.Println()

	// 统计结果
	var passed, total int
	total = len(results)
	
	fmt.Println("📊 === Test Results Summary ===")
	for _, result := range results {
		if result.Success {
			passed++
			fmt.Printf("  %s: ✓", result.Name)
			if result.IP != "" {
				fmt.Printf(" (IP: %s)", result.IP)
			}
			fmt.Println()
		} else {
			fmt.Printf("  %s: ✗ %s\n", result.Name, result.Message)
		}
	}

	fmt.Println()
	fmt.Printf("🧪 Total tests: %d\n", total)
	fmt.Printf("✅ Passed: %d\n", passed)
	fmt.Printf("❌ Failed: %d\n", total-passed)

	if passed == total {
		fmt.Println()
		fmt.Println("🎉 All tests passed! ProxyFlow is working correctly")
		os.Exit(0)
	} else {
		fmt.Println()
		fmt.Println("⚠️  Some tests failed, please check ProxyFlow configuration")
		fmt.Println()
		fmt.Println("🔧 Troubleshooting steps:")
		fmt.Println("1. 🚀 Ensure ProxyFlow service is running")
		fmt.Println("2. 📝 Check if proxy.txt contains valid proxies")
		fmt.Println("3. 🔐 Verify proxy server authentication credentials")
		fmt.Printf("4. 🛡️  Ensure firewall allows port %s\n", config.ProxyPort)
		fmt.Println("5. 🌐 Check network connection and DNS resolution")
		os.Exit(1)
	}
}