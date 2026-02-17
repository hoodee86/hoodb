package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config 节点配置
type Config struct {
	NodeID       string            `json:"nodeId"`
	HTTPAddr     string            `json:"httpAddr"`
	RaftAddr     string            `json:"raftAddr"`
	DataDir      string            `json:"dataDir"`
	Peers        []string          `json:"peers"`
	NoSync       *bool             `json:"noSync,omitempty"`       // BoltDB NoSync (default: true)
	APIKey       string            `json:"apiKey,omitempty"`       // API 认证密钥 (空 = 无认证)
	MaxKeySize   int               `json:"maxKeySize,omitempty"`   // 最大 Key 大小 (default: 1024 bytes)
	MaxValueSize int               `json:"maxValueSize,omitempty"` // 最大 Value 大小 (default: 1MB)
	MaxBatchSize int               `json:"maxBatchSize,omitempty"` // 最大批量条目数 (default: 1000)
	HTTPPeers    map[string]string `json:"httpPeers,omitempty"`    // Raft 地址 -> HTTP 地址映射
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
	return filepath.Join(c.DataDir, "store")
}

// RaftDir 返回 Raft 日志数据目录
func (c *Config) RaftDir() string {
	return filepath.Join(c.DataDir, "raft")
}

// GetNoSync 返回 NoSync 设置，默认 true 保持向后兼容
func (c *Config) GetNoSync() bool {
	if c.NoSync == nil {
		return true
	}
	return *c.NoSync
}

// GetMaxKeySize 返回最大 Key 大小，默认 1024 bytes
func (c *Config) GetMaxKeySize() int {
	if c.MaxKeySize <= 0 {
		return 1024
	}
	return c.MaxKeySize
}

// GetMaxValueSize 返回最大 Value 大小，默认 1MB
func (c *Config) GetMaxValueSize() int {
	if c.MaxValueSize <= 0 {
		return 1 << 20 // 1MB
	}
	return c.MaxValueSize
}

// GetMaxBatchSize 返回最大批量条目数，默认 1000
func (c *Config) GetMaxBatchSize() int {
	if c.MaxBatchSize <= 0 {
		return 1000
	}
	return c.MaxBatchSize
}
