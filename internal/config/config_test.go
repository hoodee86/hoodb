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
	if cfg.StoreDir() != "./data/node1/store" {
		t.Errorf("expected store dir './data/node1/store', got '%s'", cfg.StoreDir())
	}
	if cfg.RaftDir() != "./data/node1/raft" {
		t.Errorf("expected raft dir './data/node1/raft', got '%s'", cfg.RaftDir())
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
