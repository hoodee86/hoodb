package storage

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestPebbleBasicOperations(t *testing.T) {
	tmpDir := "./test_db_basic"
	defer os.RemoveAll(tmpDir)

	store, err := NewPebbleStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// 测试Put和Get
	key := []byte("test_key")
	value := []byte("test_value")

	err = store.Put(key, value)
	if err != nil {
		t.Fatalf("Failed to put: %v", err)
	}

	result, err := store.Get(key)
	if err != nil {
		t.Fatalf("Failed to get: %v", err)
	}

	if string(result) != string(value) {
		t.Fatalf("Expected %s, got %s", value, result)
	}

	// 测试Delete
	err = store.Delete(key)
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	result, err = store.Get(key)
	if err != nil {
		t.Fatalf("Failed to get after delete: %v", err)
	}

	if result != nil {
		t.Fatalf("Expected nil after delete, got %v", result)
	}

	t.Log("Basic operations test passed!")
}

func TestPebbleConcurrentWrites(t *testing.T) {
	tmpDir := "./test_db_concurrent"
	defer os.RemoveAll(tmpDir)

	store, err := NewPebbleStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// 并发写入测试
	numGoroutines := 10
	writesPerGoroutine := 100
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				key := []byte(fmt.Sprintf("key_%d_%d", id, j))
				value := []byte(fmt.Sprintf("value_%d_%d", id, j))
				if err := store.Put(key, value); err != nil {
					t.Errorf("Failed to put: %v", err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	// 验证所有数据
	for i := 0; i < numGoroutines; i++ {
		for j := 0; j < writesPerGoroutine; j++ {
			key := []byte(fmt.Sprintf("key_%d_%d", i, j))
			value := []byte(fmt.Sprintf("value_%d_%d", i, j))
			result, err := store.Get(key)
			if err != nil {
				t.Errorf("Failed to get key_%d_%d: %v", i, j, err)
			}
			if string(result) != string(value) {
				t.Errorf("Mismatch for key_%d_%d: expected %s, got %s", i, j, value, result)
			}
		}
	}

	totalOps := numGoroutines * writesPerGoroutine
	opsPerSec := float64(totalOps) / duration.Seconds()
	t.Logf("Concurrent writes: %d ops in %v (%.2f ops/sec)", totalOps, duration, opsPerSec)
}

func TestPebbleConcurrentReads(t *testing.T) {
	tmpDir := "./test_db_concurrent_reads"
	defer os.RemoveAll(tmpDir)

	store, err := NewPebbleStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// 先写入一些数据
	numKeys := 1000
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("key_%d", i))
		value := []byte(fmt.Sprintf("value_%d", i))
		if err := store.Put(key, value); err != nil {
			t.Fatalf("Failed to put: %v", err)
		}
	}

	// 并发读取测试
	numGoroutines := 20
	readsPerGoroutine := 500
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < readsPerGoroutine; j++ {
				key := []byte(fmt.Sprintf("key_%d", j%numKeys))
				_, err := store.Get(key)
				if err != nil {
					t.Errorf("Failed to get: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	totalOps := numGoroutines * readsPerGoroutine
	opsPerSec := float64(totalOps) / duration.Seconds()
	t.Logf("Concurrent reads: %d ops in %v (%.2f ops/sec)", totalOps, duration, opsPerSec)
}

func TestPebbleBatchOperations(t *testing.T) {
	tmpDir := "./test_db_batch"
	defer os.RemoveAll(tmpDir)

	store, err := NewPebbleStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// 批量写入测试
	batch := store.NewBatch()
	numKeys := 1000

	start := time.Now()

	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("batch_key_%d", i))
		value := []byte(fmt.Sprintf("batch_value_%d", i))
		if err := batch.Set(key, value, nil); err != nil {
			t.Fatalf("Failed to set in batch: %v", err)
		}
	}

	if err := store.WriteBatch(batch); err != nil {
		t.Fatalf("Failed to write batch: %v", err)
	}

	duration := time.Since(start)

	// 验证批量写入的数据
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("batch_key_%d", i))
		value := []byte(fmt.Sprintf("batch_value_%d", i))
		result, err := store.Get(key)
		if err != nil {
			t.Errorf("Failed to get batch_key_%d: %v", i, err)
		}
		if string(result) != string(value) {
			t.Errorf("Mismatch for batch_key_%d", i)
		}
	}

	opsPerSec := float64(numKeys) / duration.Seconds()
	t.Logf("Batch write: %d ops in %v (%.2f ops/sec)", numKeys, duration, opsPerSec)
}

func TestPebbleIterator(t *testing.T) {
	tmpDir := "./test_db_iterator"
	defer os.RemoveAll(tmpDir)

	store, err := NewPebbleStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// 写入有序数据
	numKeys := 100
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("key_%03d", i))
		value := []byte(fmt.Sprintf("value_%03d", i))
		if err := store.Put(key, value); err != nil {
			t.Fatalf("Failed to put: %v", err)
		}
	}

	// 测试迭代器
	iter, err := store.NewIterator()
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}
	defer iter.Close()

	count := 0
	for iter.First(); iter.Valid(); iter.Next() {
		count++
	}

	if err := iter.Error(); err != nil {
		t.Fatalf("Iterator error: %v", err)
	}

	if count != numKeys {
		t.Fatalf("Expected %d keys, got %d", numKeys, count)
	}

	t.Logf("Iterator test passed: counted %d keys", count)
}

func BenchmarkPebblePut(b *testing.B) {
	tmpDir := "./bench_db_put"
	defer os.RemoveAll(tmpDir)

	store, err := NewPebbleStore(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("bench_key_%d", i))
		value := []byte(fmt.Sprintf("bench_value_%d", i))
		if err := store.Put(key, value); err != nil {
			b.Fatalf("Failed to put: %v", err)
		}
	}
}

func BenchmarkPebbleGet(b *testing.B) {
	tmpDir := "./bench_db_get"
	defer os.RemoveAll(tmpDir)

	store, err := NewPebbleStore(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// 预先写入数据
	numKeys := 10000
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("bench_key_%d", i))
		value := []byte(fmt.Sprintf("bench_value_%d", i))
		if err := store.Put(key, value); err != nil {
			b.Fatalf("Failed to put: %v", err)
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("bench_key_%d", i%numKeys))
		_, err := store.Get(key)
		if err != nil {
			b.Fatalf("Failed to get: %v", err)
		}
	}
}
