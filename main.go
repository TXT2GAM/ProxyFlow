package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"proxyflow/config"
	"proxyflow/proxy"

	"github.com/joho/godotenv"
)

/**
 * 程序入口点
 */
func main() {
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	// 加载配置
	cfg := config.Load()
	log.Printf("Starting ProxyFlow with config: Port=%s, ProxyFile=%s, PoolSize=%d",
		cfg.ProxyPort, cfg.ProxyFile, cfg.PoolSize)

	// 创建代理池
	pool, err := proxy.NewPool(cfg.ProxyFile)
	if err != nil {
		log.Fatalf("Failed to create proxy pool: %v", err)
	}

	// 创建代理服务器
	server := proxy.NewServer(pool, cfg.RequestTimeout, cfg.AuthUsername, cfg.AuthPassword)

	// 设置优雅关闭
	setupGracefulShutdown()

	// 启动服务器
	log.Printf("ProxyFlow is ready to handle requests")
	if err := server.Start(cfg.ProxyPort); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

/**
 * 设置优雅关闭处理
 */
func setupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("Shutting down ProxyFlow...")
		os.Exit(0)
	}()
}
