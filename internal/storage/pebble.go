package storage

import (
	"fmt"
	"sync"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
)

// PebbleStore 封装Pebble存储引擎
type PebbleStore struct {
	db   *pebble.DB
	path string
	mu   sync.Mutex // 仅保护 Close/Compact 等管理操作; Pebble 本身是线程安全的
}

// NewPebbleStore 创建新的Pebble存储实例
func NewPebbleStore(path string) (*PebbleStore, error) {
	cache := pebble.NewCache(512 << 20) // 512MB 块缓存
	defer cache.Unref()

	opts := &pebble.Options{
		Cache:        cache,
		MemTableSize: 128 << 20, // 128MB MemTable (合理大小减少 flush 频率)

		// L0 触发参数 — 适当放宽避免频繁 compaction
		L0CompactionThreshold: 4,
		L0StopWritesThreshold: 12,
		LBaseMaxBytes:         64 << 20, // 64MB

		// 并发 compaction
		MaxConcurrentCompactions: func() int { return 4 },
		MaxOpenFiles:             10000,

		// WAL 配置
		DisableWAL: false,
		WALDir:     "", // 与数据同目录

		// 分层配置 — 启用 Bloom Filter 提升读性能
		Levels: []pebble.LevelOptions{
			{TargetFileSize: 8 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},   // L0
			{TargetFileSize: 16 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},  // L1
			{TargetFileSize: 32 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},  // L2
			{TargetFileSize: 64 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},  // L3
			{TargetFileSize: 128 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression}, // L4
			{TargetFileSize: 256 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression}, // L5
			{TargetFileSize: 256 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression}, // L6
		},
	}

	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open pebble: %w", err)
	}

	return &PebbleStore{
		db:   db,
		path: path,
	}, nil
}

// Get 获取指定key的value
func (s *PebbleStore) Get(key []byte) ([]byte, error) {
	value, closer, err := s.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get key: %w", err)
	}
	defer closer.Close()

	// 复制数据
	data := make([]byte, len(value))
	copy(data, value)
	return data, nil
}

// Put 设置key-value对 (同步写入)
func (s *PebbleStore) Put(key, value []byte) error {
	err := s.db.Set(key, value, pebble.Sync)
	if err != nil {
		return fmt.Errorf("failed to put key: %w", err)
	}
	return nil
}

// PutNoSync 设置key-value对 (不强制sync，Raft日志已保证持久性)
func (s *PebbleStore) PutNoSync(key, value []byte) error {
	err := s.db.Set(key, value, pebble.NoSync)
	if err != nil {
		return fmt.Errorf("failed to put key: %w", err)
	}
	return nil
}

// PutBatch 批量写入多个key-value对 (高性能)
func (s *PebbleStore) PutBatch(kvs map[string]string) error {
	batch := s.db.NewBatch()
	for k, v := range kvs {
		if err := batch.Set([]byte(k), []byte(v), nil); err != nil {
			batch.Close()
			return fmt.Errorf("failed to set in batch: %w", err)
		}
	}
	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}
	return nil
}

// Delete 删除指定的key
func (s *PebbleStore) Delete(key []byte) error {
	err := s.db.Delete(key, pebble.Sync)
	if err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}
	return nil
}

// DeleteNoSync 删除指定的key (不强制sync)
func (s *PebbleStore) DeleteNoSync(key []byte) error {
	err := s.db.Delete(key, pebble.NoSync)
	if err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}
	return nil
}

// NewBatch 创建新的写批次
func (s *PebbleStore) NewBatch() *pebble.Batch {
	return s.db.NewBatch()
}

// WriteBatch 批量写入操作
func (s *PebbleStore) WriteBatch(batch *pebble.Batch) error {
	err := batch.Commit(pebble.Sync)
	if err != nil {
		return fmt.Errorf("failed to write batch: %w", err)
	}
	return nil
}

// GetSnapshot 获取数据库快照
func (s *PebbleStore) GetSnapshot() *pebble.Snapshot {
	return s.db.NewSnapshot()
}

// ReleaseSnapshot 释放快照
func (s *PebbleStore) ReleaseSnapshot(snapshot *pebble.Snapshot) {
	snapshot.Close()
}

// NewIterator 创建迭代器 (基于 live DB)
func (s *PebbleStore) NewIterator() (*pebble.Iterator, error) {
	return s.db.NewIter(nil)
}

// NewSnapshotIterator 基于快照创建迭代器（用于 FSM Snapshot，保证一致性读）
func (s *PebbleStore) NewSnapshotIterator(snap *pebble.Snapshot) (*pebble.Iterator, error) {
	return snap.NewIter(nil)
}

// KeyValue 键值对
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ScanPrefix 按前缀扫描 Key，支持游标分页
// prefix: 搜索前缀, limit: 最大返回条数, cursor: 起始游标 (上次返回的 nextCursor)
// 返回: 结果列表, 下一页游标 (空字符串表示没有更多数据), 错误
func (s *PebbleStore) ScanPrefix(prefix []byte, limit int, cursor []byte) ([]KeyValue, string, error) {
	iterOpts := &pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpperBound(prefix),
	}
	iter, err := s.db.NewIter(iterOpts)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create prefix iterator: %w", err)
	}
	defer iter.Close()

	var seekKey []byte
	if len(cursor) > 0 {
		seekKey = cursor
	} else {
		seekKey = prefix
	}

	var results []KeyValue
	for iter.SeekGE(seekKey); iter.Valid() && len(results) < limit; iter.Next() {
		// Direct string conversion without intermediate byte slice allocation
		// This is safe because string() creates a copy internally
		results = append(results, KeyValue{
			Key:   string(iter.Key()),
			Value: string(iter.Value()),
		})
	}

	var nextCursor string
	if iter.Valid() {
		// 还有更多数据
		// Direct string conversion - no need for intermediate copy
		nextCursor = string(iter.Key())
	}

	if err := iter.Error(); err != nil {
		return nil, "", fmt.Errorf("iterator error: %w", err)
	}

	return results, nextCursor, nil
}

// prefixUpperBound 计算前缀的上界 (用于限定迭代器范围)
func prefixUpperBound(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil // 无前缀 = 扫描所有 key
	}
	upper := make([]byte, len(prefix))
	copy(upper, prefix)
	for i := len(upper) - 1; i >= 0; i-- {
		if upper[i] < 0xff {
			upper[i]++
			return upper[:i+1]
		}
	}
	return nil // 前缀全是 0xff
}

// Close 关闭数据库
func (s *PebbleStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// GetPath 返回数据库路径
func (s *PebbleStore) GetPath() string {
	return s.path
}

// ClearAll 清除所有数据（用于快照恢复前清空旧状态）
func (s *PebbleStore) ClearAll() error {
	iter, err := s.db.NewIter(nil)
	if err != nil {
		return fmt.Errorf("failed to create iterator for clear: %w", err)
	}
	defer iter.Close()

	batch := s.db.NewBatch()
	
	for iter.First(); iter.Valid(); iter.Next() {
		// Delete directly using iter.Key() - no need to copy since batch copies internally
		if err := batch.Delete(iter.Key(), nil); err != nil {
			batch.Close()
			return fmt.Errorf("failed to delete key in clear: %w", err)
		}
	}
	if err := iter.Error(); err != nil {
		batch.Close()
		return fmt.Errorf("iterator error during clear: %w", err)
	}
	// Commit the batch - no need to Close() after successful Commit()
	if err := batch.Commit(pebble.NoSync); err != nil {
		batch.Close()
		return fmt.Errorf("failed to commit clear batch: %w", err)
	}
	return nil
}

// Compact 手动触发压缩
func (s *PebbleStore) Compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Compact(nil, []byte("\xff\xff\xff\xff"), true)
}
