package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config 节点配置
type Config struct {
	NodeID   string   `json:"nodeId"`
	HTTPAddr string   `json:"httpAddr"`
	RaftAddr string   `json:"raftAddr"`
	DataDir  string   `json:"dataDir"`
	Peers    []string `json:"peers"`
}

// Load 从 JSON 文件加载配置
func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Validate 校验配置合法性
func (c *Config) Validate() error {
	if strings.TrimSpace(c.NodeID) == "" {
		return fmt.Errorf("nodeId is required")
	}
	if strings.TrimSpace(c.HTTPAddr) == "" {
		return fmt.Errorf("httpAddr is required")
	}
	if strings.TrimSpace(c.RaftAddr) == "" {
		return fmt.Errorf("raftAddr is required")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return fmt.Errorf("dataDir is required")
	}
	return nil
}

// StoreDir 返回存储引擎数据目录
func (c *Config) StoreDir() string {
	return c.DataDir + "/store"
}

// RaftDir 返回 Raft 日志数据目录
func (c *Config) RaftDir() string {
	return c.DataDir + "/raft"
}
