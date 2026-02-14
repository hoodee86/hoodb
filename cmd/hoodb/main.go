package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	httpapi "github.com/shauntso/hoodb/api/http"
	"github.com/shauntso/hoodb/internal/service"
)

// Config 配置结构
type Config struct {
	NodeID   string   `json:"nodeId"`
	HTTPAddr string   `json:"httpAddr"`
	RaftAddr string   `json:"raftAddr"`
	DataDir  string   `json:"dataDir"`
	Peers    []string `json:"peers"`
}

func main() {
	// 解析命令行参数
	configFile := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	if *configFile == "" {
		log.Fatal("Config file is required. Use -config flag")
	}

	// 读取配置文件
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 创建KV服务
	log.Printf("Starting HooDB node: %s", config.NodeID)
	log.Printf("HTTP address: %s", config.HTTPAddr)
	log.Printf("Raft address: %s", config.RaftAddr)
	log.Printf("Data directory: %s", config.DataDir)
	log.Printf("Peers: %v", config.Peers)

	kvService, err := service.NewKVService(
		config.DataDir,
		config.NodeID,
		config.RaftAddr,
		config.Peers,
	)
	if err != nil {
		log.Fatalf("Failed to create KV service: %v", err)
	}
	defer kvService.Close()

	// 创建HTTP服务器
	handler := httpapi.NewHandler(kvService)
	router := handler.SetupRoutes()

	// 启动HTTP服务器
	go func() {
		log.Printf("HTTP server listening on %s", config.HTTPAddr)
		if err := router.Run(config.HTTPAddr); err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
}

// loadConfig 加载配置文件
func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return &config, nil
}
