package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	httpapi "github.com/shauntso/hoodb/api/http"
	"github.com/shauntso/hoodb/internal/config"
	"github.com/shauntso/hoodb/internal/service"
)

func main() {
	configFile := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	if *configFile == "" {
		log.Fatal("Config file is required. Use -config flag")
	}

	// 加载并校验配置
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting HooDB node: %s", cfg.NodeID)
	log.Printf("HTTP address: %s", cfg.HTTPAddr)
	log.Printf("Raft address: %s", cfg.RaftAddr)
	log.Printf("Data directory: %s", cfg.DataDir)
	log.Printf("Peers: %v", cfg.Peers)

	// 创建 KV 服务
	kvService, err := service.NewKVService(cfg)
	if err != nil {
		log.Fatalf("Failed to create KV service: %v", err)
	}
	defer kvService.Close()

	// 创建并启动 HTTP 服务器
	handler := httpapi.NewHandler(kvService)
	router := handler.SetupRoutes()

	go func() {
		log.Printf("HTTP server listening on %s", cfg.HTTPAddr)
		if err := router.Run(cfg.HTTPAddr); err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
}
