package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	content := `{
		"nodeId": "node1",
		"httpAddr": ":8001",
		"raftAddr": "127.0.0.1:9001",
		"dataDir": "./data/node1",
		"peers": ["127.0.0.1:9001", "127.0.0.1:9002"]
	}`

	tmpFile, err := os.CreateTemp("", "config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.NodeID != "node1" {
		t.Errorf("expected nodeId 'node1', got '%s'", cfg.NodeID)
	}
	if cfg.StoreDir() != "data/node1/store" {
		t.Errorf("expected store dir 'data/node1/store', got '%s'", cfg.StoreDir())
	}
	if cfg.RaftDir() != "data/node1/raft" {
		t.Errorf("expected raft dir 'data/node1/raft', got '%s'", cfg.RaftDir())
	}
}

func TestValidate_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"empty nodeId", Config{HTTPAddr: ":8001", RaftAddr: "127.0.0.1:9001", DataDir: "./data"}},
		{"empty httpAddr", Config{NodeID: "n1", RaftAddr: "127.0.0.1:9001", DataDir: "./data"}},
		{"empty raftAddr", Config{NodeID: "n1", HTTPAddr: ":8001", DataDir: "./data"}},
		{"empty dataDir", Config{NodeID: "n1", HTTPAddr: ":8001", RaftAddr: "127.0.0.1:9001"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestGetNoSync_Default(t *testing.T) {
	cfg := &Config{}
	if !cfg.GetNoSync() {
		t.Error("expected default NoSync to be true")
	}
}

func TestGetNoSync_Explicit(t *testing.T) {
	f := false
	cfg := &Config{NoSync: &f}
	if cfg.GetNoSync() {
		t.Error("expected explicit false NoSync")
	}

	tr := true
	cfg2 := &Config{NoSync: &tr}
	if !cfg2.GetNoSync() {
		t.Error("expected explicit true NoSync")
	}
}

func TestGetMaxKeySize_Default(t *testing.T) {
	cfg := &Config{}
	if cfg.GetMaxKeySize() != 1024 {
		t.Errorf("expected default MaxKeySize 1024, got %d", cfg.GetMaxKeySize())
	}
}

func TestGetMaxValueSize_Default(t *testing.T) {
	cfg := &Config{}
	if cfg.GetMaxValueSize() != 1<<20 {
		t.Errorf("expected default MaxValueSize 1MB, got %d", cfg.GetMaxValueSize())
	}
}

func TestGetMaxBatchSize_Default(t *testing.T) {
	cfg := &Config{}
	if cfg.GetMaxBatchSize() != 1000 {
		t.Errorf("expected default MaxBatchSize 1000, got %d", cfg.GetMaxBatchSize())
	}
}

func TestGetMaxKeySize_Custom(t *testing.T) {
	cfg := &Config{MaxKeySize: 256}
	if cfg.GetMaxKeySize() != 256 {
		t.Errorf("expected MaxKeySize 256, got %d", cfg.GetMaxKeySize())
	}
}

func TestLoadConfig_WithNewFields(t *testing.T) {
	content := `{
		"nodeId": "node1",
		"httpAddr": ":8001",
		"raftAddr": "127.0.0.1:9001",
		"dataDir": "./data/node1",
		"peers": ["127.0.0.1:9001"],
		"noSync": true,
		"apiKey": "my-secret",
		"maxKeySize": 512,
		"maxValueSize": 2048,
		"maxBatchSize": 500,
		"httpPeers": {
			"127.0.0.1:9001": "127.0.0.1:8001"
		}
	}`

	tmpFile, err := os.CreateTemp("", "config-new-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.GetNoSync() {
		t.Error("expected NoSync true")
	}
	if cfg.APIKey != "my-secret" {
		t.Errorf("expected APIKey 'my-secret', got '%s'", cfg.APIKey)
	}
	if cfg.GetMaxKeySize() != 512 {
		t.Errorf("expected MaxKeySize 512, got %d", cfg.GetMaxKeySize())
	}
	if cfg.GetMaxValueSize() != 2048 {
		t.Errorf("expected MaxValueSize 2048, got %d", cfg.GetMaxValueSize())
	}
	if cfg.GetMaxBatchSize() != 500 {
		t.Errorf("expected MaxBatchSize 500, got %d", cfg.GetMaxBatchSize())
	}
	if cfg.HTTPPeers["127.0.0.1:9001"] != "127.0.0.1:8001" {
		t.Error("expected HTTPPeers mapping")
	}
}
