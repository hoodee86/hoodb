package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble"
	hraft "github.com/hashicorp/raft"
	"github.com/shauntso/hoodb/internal/config"
	"github.com/shauntso/hoodb/internal/raft"
	"github.com/shauntso/hoodb/internal/storage"
)

const (
	// maxBatchSize 写入批处理器的最大批量大小
	maxBatchSize = 200
)

// KVService 分布式KV服务
type KVService struct {
	store   *storage.PebbleStore
	raft    *raft.RaftNode
	logger  *log.Logger
	mu      sync.RWMutex
	batcher *writeBatcher
}

// NewKVService 创建新的KV服务
func NewKVService(cfg *config.Config) (*KVService, error) {
	// 创建 Pebble 存储引擎
	store, err := storage.NewPebbleStore(cfg.StoreDir())
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	kv := &KVService{
		store:  store,
		logger: log.New(log.Writer(), fmt.Sprintf("[%s] ", cfg.NodeID), log.LstdFlags),
	}

	// 创建 Raft 节点，KVService 实现了 FSM 接口
	raftNode, err := raft.NewRaftNode(&raft.Config{
		NodeID:   cfg.NodeID,
		BindAddr: cfg.RaftAddr,
		DataDir:  cfg.RaftDir(),
		FSM:      kv,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create raft node: %w", err)
	}

	kv.raft = raftNode

	// 创建写入批处理器 (合并并发写入提升吞吐量, 流水线架构)
	kv.batcher = newWriteBatcher(kv, maxBatchSize)

	// 如果是第一个节点且 peers 不为空，进行 Bootstrap
	// peers 为空表示这是要动态加入现有集群的新节点，不应自行 Bootstrap
	if cfg.NodeID == "node1" && len(cfg.Peers) > 0 {
		if err := raftNode.Bootstrap(cfg.Peers); err != nil {
			kv.logger.Printf("Bootstrap warning (may already be bootstrapped): %v", err)
		}
	}

	// 等待 Leader 选举
	if err := raftNode.WaitForLeader(30 * time.Second); err != nil {
		kv.logger.Printf("Warning: %v", err)
	}

	kv.logger.Printf("KV Service started, Leader: %s, IsLeader: %v",
		raftNode.GetLeader(), raftNode.IsLeader())

	return kv, nil
}

// Get 获取key的值
func (kv *KVService) Get(key string) (string, error) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	value, err := kv.store.Get([]byte(key))
	if err != nil {
		return "", fmt.Errorf("failed to get key: %w", err)
	}

	if value == nil {
		return "", fmt.Errorf("key not found")
	}

	return string(value), nil
}

// Set 设置key-value，通过Raft同步
func (kv *KVService) Set(key, value string) error {
	// 如果不是Leader，返回错误
	if !kv.raft.IsLeader() {
		return fmt.Errorf("not leader, leader is: %s", kv.raft.GetLeader())
	}

	// 创建命令
	cmd := &raft.Command{
		Op:    "set",
		Key:   key,
		Value: value,
	}

	// 通过Raft应用命令
	if err := kv.raft.ApplyCommand(cmd); err != nil {
		return fmt.Errorf("failed to apply command: %w", err)
	}

	return nil
}

// SetBatched 通过写入批处理器设置key-value (高性能，合并并发写入)
func (kv *KVService) SetBatched(key, value string) error {
	if !kv.raft.IsLeader() {
		return fmt.Errorf("not leader, leader is: %s", kv.raft.GetLeader())
	}
	return kv.batcher.Submit(key, value)
}

// BatchSet 批量设置多个key-value，单次Raft提交
func (kv *KVService) BatchSet(kvs map[string]string) error {
	if !kv.raft.IsLeader() {
		return fmt.Errorf("not leader, leader is: %s", kv.raft.GetLeader())
	}

	cmd := &raft.Command{
		Op:    "batch_set",
		Batch: kvs,
	}

	if err := kv.raft.ApplyCommand(cmd); err != nil {
		return fmt.Errorf("failed to apply batch command: %w", err)
	}

	return nil
}

// Delete 删除key，通过Raft同步
func (kv *KVService) Delete(key string) error {
	// 如果不是Leader，返回错误
	if !kv.raft.IsLeader() {
		return fmt.Errorf("not leader, leader is: %s", kv.raft.GetLeader())
	}

	// 创建命令
	cmd := &raft.Command{
		Op:  "delete",
		Key: key,
	}

	// 通过Raft应用命令
	if err := kv.raft.ApplyCommand(cmd); err != nil {
		return fmt.Errorf("failed to apply command: %w", err)
	}

	kv.logger.Printf("Delete key=%s", key)
	return nil
}

// AddNode 添加新节点到集群
func (kv *KVService) AddNode(nodeID, address string) error {
	if !kv.raft.IsLeader() {
		return fmt.Errorf("not leader, leader is: %s", kv.raft.GetLeader())
	}

	kv.logger.Printf("Adding node %s at %s to cluster", nodeID, address)
	if err := kv.raft.AddVoter(nodeID, address); err != nil {
		return fmt.Errorf("failed to add node: %w", err)
	}

	kv.logger.Printf("Node %s added successfully", nodeID)
	return nil
}

// RemoveNode 从集群移除节点
func (kv *KVService) RemoveNode(nodeID string) error {
	if !kv.raft.IsLeader() {
		return fmt.Errorf("not leader, leader is: %s", kv.raft.GetLeader())
	}

	kv.logger.Printf("Removing node %s from cluster", nodeID)
	if err := kv.raft.RemoveServer(nodeID); err != nil {
		return fmt.Errorf("failed to remove node: %w", err)
	}

	kv.logger.Printf("Node %s removed successfully", nodeID)
	return nil
}

// GetClusterConfig 获取集群配置信息
func (kv *KVService) GetClusterConfig() (map[string]interface{}, error) {
	config, err := kv.raft.GetConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	servers := make([]map[string]string, 0)
	for _, server := range config.Servers {
		servers = append(servers, map[string]string{
			"id":       string(server.ID),
			"address":  string(server.Address),
			"suffrage": server.Suffrage.String(),
		})
	}

	return map[string]interface{}{
		"servers": servers,
		"leader":  kv.raft.GetLeader(),
	}, nil
}

// BenchmarkResult 服务端基准测试结果
type BenchmarkResult struct {
	Count     int     `json:"count"`
	Success   int     `json:"success"`
	Failed    int     `json:"failed"`
	Millis    float64 `json:"duration_ms"`
	OpsPerSec float64 `json:"ops_per_sec"`
}

// RunBenchmark 在服务端直接执行写入基准测试
// 绕过 HTTP 连接数限制, 准确测量 Raft 写入吞吐量
func (kv *KVService) RunBenchmark(count, concurrency int) (*BenchmarkResult, error) {
	if !kv.raft.IsLeader() {
		return nil, fmt.Errorf("not leader, leader is: %s", kv.raft.GetLeader())
	}

	var success, failed int64
	var wg sync.WaitGroup

	// 使用 channel 分发任务
	tasks := make(chan int, count)
	for i := 0; i < count; i++ {
		tasks <- i
	}
	close(tasks)

	start := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range tasks {
				key := fmt.Sprintf("bench_%d_%d", start.UnixNano(), i)
				value := fmt.Sprintf("val_%d", i)
				if err := kv.batcher.Submit(key, value); err != nil {
					atomic.AddInt64(&failed, 1)
				} else {
					atomic.AddInt64(&success, 1)
				}
			}
		}()
	}

	wg.Wait()
	dur := time.Since(start)

	s := int(atomic.LoadInt64(&success))
	f := int(atomic.LoadInt64(&failed))
	ms := float64(dur.Nanoseconds()) / 1e6

	return &BenchmarkResult{
		Count:     count,
		Success:   s,
		Failed:    f,
		Millis:    ms,
		OpsPerSec: float64(s) / dur.Seconds(),
	}, nil
}

// ===== 实现raft.FSM接口 =====

// Apply 应用Raft日志到状态机
// 注意: Raft 保证 Apply 是串行调用的, 无需对 Pebble 写入加锁
// (Pebble 本身也是线程安全的), 仅 Snapshot/Restore 需要互斥
func (kv *KVService) Apply(log *hraft.Log) interface{} {
	var cmd raft.Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		kv.logger.Printf("Failed to unmarshal command: %v", err)
		return err
	}

	switch cmd.Op {
	case "set":
		// 使用NoSync: Raft日志已保证持久性，无需双重 sync
		err := kv.store.PutNoSync([]byte(cmd.Key), []byte(cmd.Value))
		if err != nil {
			kv.logger.Printf("Failed to put key: %v", err)
			return err
		}
	case "batch_set":
		// 批量写入: 单次批量提交
		if cmd.Batch != nil {
			if err := kv.store.PutBatch(cmd.Batch); err != nil {
				kv.logger.Printf("Failed to batch put: %v", err)
				return err
			}
		}
	case "delete":
		err := kv.store.DeleteNoSync([]byte(cmd.Key))
		if err != nil {
			kv.logger.Printf("Failed to delete key: %v", err)
			return err
		}
	default:
		kv.logger.Printf("Unknown command: %s", cmd.Op)
	}

	return nil
}

// Snapshot 创建快照
func (kv *KVService) Snapshot() (hraft.FSMSnapshot, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	// 创建快照
	snapshot := kv.store.GetSnapshot()

	return &fsmSnapshot{
		store:    kv.store,
		snapshot: snapshot,
	}, nil
}

// Restore 从快照恢复
func (kv *KVService) Restore(rc io.ReadCloser) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	defer rc.Close()

	// 读取快照数据
	decoder := json.NewDecoder(rc)
	var data map[string]string
	if err := decoder.Decode(&data); err != nil {
		return fmt.Errorf("failed to decode snapshot: %w", err)
	}

	// 先清除旧数据，确保状态机完全替换为快照状态
	if err := kv.store.ClearAll(); err != nil {
		kv.logger.Printf("Warning: failed to clear store before restore: %v", err)
	}

	// 使用批量写入恢复数据（NoSync，避免每条都 fsync 导致恢复极慢）
	if err := kv.store.PutBatch(data); err != nil {
		return fmt.Errorf("failed to restore batch: %w", err)
	}

	kv.logger.Printf("Restored %d keys from snapshot", len(data))
	return nil
}

// IsLeader 判断是否为Leader
func (kv *KVService) IsLeader() bool {
	return kv.raft.IsLeader()
}

// GetLeader 获取Leader地址
func (kv *KVService) GetLeader() string {
	return kv.raft.GetLeader()
}

// GetStats 获取服务统计信息
func (kv *KVService) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"isLeader":     kv.raft.IsLeader(),
		"leader":       kv.raft.GetLeader(),
		"state":        kv.raft.State().String(),
		"lastIndex":    kv.raft.LastIndex(),
		"appliedIndex": kv.raft.AppliedIndex(),
		"raftStats":    kv.raft.Stats(),
	}
}

// Close 关闭服务
func (kv *KVService) Close() error {
	if kv.batcher != nil {
		kv.batcher.Stop()
	}
	if err := kv.raft.Shutdown(); err != nil {
		kv.logger.Printf("Failed to shutdown raft: %v", err)
	}
	if err := kv.store.Close(); err != nil {
		kv.logger.Printf("Failed to close store: %v", err)
	}
	return nil
}

// fsmSnapshot 实现FSMSnapshot接口
type fsmSnapshot struct {
	store    *storage.PebbleStore
	snapshot *pebble.Snapshot
}

// Persist 持久化快照
func (f *fsmSnapshot) Persist(sink hraft.SnapshotSink) error {
	defer sink.Close()

	// 基于 Pebble 快照创建迭代器（而非 live DB），保证快照数据一致性
	iter, err := f.store.NewSnapshotIterator(f.snapshot)
	if err != nil {
		sink.Cancel()
		return fmt.Errorf("failed to create snapshot iterator: %w", err)
	}
	defer iter.Close()

	data := make(map[string]string)
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		value := make([]byte, len(iter.Value()))
		copy(value, iter.Value())
		data[key] = string(value)
	}

	if err := iter.Error(); err != nil {
		return fmt.Errorf("iterator error: %w", err)
	}

	// 序列化数据
	encoder := json.NewEncoder(sink)
	if err := encoder.Encode(data); err != nil {
		sink.Cancel()
		return fmt.Errorf("failed to encode snapshot: %w", err)
	}

	return nil
}

// Release 释放快照资源
func (f *fsmSnapshot) Release() {
	if f.snapshot != nil {
		f.store.ReleaseSnapshot(f.snapshot)
	}
}
