package benchmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 1. 原始存储引擎: Pebble vs LevelDB
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func BenchmarkPebble_SeqWrite_Sync(b *testing.B) {
	dir := b.TempDir()
	db, err := pebble.Open(dir, &pebble.Options{})
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	val := bytes.Repeat([]byte("v"), 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		if err := db.Set(key, val, pebble.Sync); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPebble_SeqWrite_NoSync(b *testing.B) {
	dir := b.TempDir()
	db, err := pebble.Open(dir, &pebble.Options{})
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	val := bytes.Repeat([]byte("v"), 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		if err := db.Set(key, val, pebble.NoSync); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPebble_BatchWrite(b *testing.B) {
	dir := b.TempDir()
	db, err := pebble.Open(dir, &pebble.Options{})
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	val := bytes.Repeat([]byte("v"), 100)
	batchSize := 50
	b.ResetTimer()
	for i := 0; i < b.N; i += batchSize {
		batch := db.NewBatch()
		n := minInt(batchSize, b.N-i)
		for j := 0; j < n; j++ {
			key := []byte(fmt.Sprintf("key_%010d", i+j))
			batch.Set(key, val, nil)
		}
		if err := batch.Commit(pebble.NoSync); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLevelDB_SeqWrite_Sync(b *testing.B) {
	dir := b.TempDir()
	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	val := bytes.Repeat([]byte("v"), 100)
	wo := &opt.WriteOptions{Sync: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		if err := db.Put(key, val, wo); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLevelDB_SeqWrite_NoSync(b *testing.B) {
	dir := b.TempDir()
	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	val := bytes.Repeat([]byte("v"), 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		if err := db.Put(key, val, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLevelDB_BatchWrite(b *testing.B) {
	dir := b.TempDir()
	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	val := bytes.Repeat([]byte("v"), 100)
	batchSize := 50
	b.ResetTimer()
	for i := 0; i < b.N; i += batchSize {
		batch := new(leveldb.Batch)
		n := minInt(batchSize, b.N-i)
		for j := 0; j < n; j++ {
			key := []byte(fmt.Sprintf("key_%010d", i+j))
			batch.Put(key, val)
		}
		if err := db.Write(batch, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 2. 并发写入
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func BenchmarkPebble_ConcurrentWrite(b *testing.B) {
	dir := b.TempDir()
	db, err := pebble.Open(dir, &pebble.Options{})
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	val := bytes.Repeat([]byte("v"), 100)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := []byte(fmt.Sprintf("key_%010d_%d", i, rand.Int63()))
			if err := db.Set(key, val, pebble.NoSync); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

func BenchmarkLevelDB_ConcurrentWrite(b *testing.B) {
	dir := b.TempDir()
	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	val := bytes.Repeat([]byte("v"), 100)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := []byte(fmt.Sprintf("key_%010d_%d", i, rand.Int63()))
			if err := db.Put(key, val, nil); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 3. 读取
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func BenchmarkPebble_SeqRead(b *testing.B) {
	dir := b.TempDir()
	db, err := pebble.Open(dir, &pebble.Options{})
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	val := bytes.Repeat([]byte("v"), 100)
	for i := 0; i < 100000; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		db.Set(key, val, pebble.NoSync)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i%100000))
		v, closer, err := db.Get(key)
		if err != nil {
			b.Fatal(err)
		}
		_ = v
		closer.Close()
	}
}

func BenchmarkLevelDB_SeqRead(b *testing.B) {
	dir := b.TempDir()
	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	val := bytes.Repeat([]byte("v"), 100)
	for i := 0; i < 100000; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		db.Put(key, val, nil)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i%100000))
		v, err := db.Get(key, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = v
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 4. 综合对比 (含 HooDB 集群端到端)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestHooDBvsLevelDB_Throughput(t *testing.T) {
	const numOps = 10000
	const valueSize = 100

	type result struct {
		name string
		ops  float64
	}
	var results []result

	// --- Pebble Sync ---
	t.Run("Pebble_Sync", func(t *testing.T) {
		dir := t.TempDir()
		db, err := pebble.Open(dir, &pebble.Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		val := bytes.Repeat([]byte("v"), valueSize)
		start := time.Now()
		for i := 0; i < numOps; i++ {
			key := []byte(fmt.Sprintf("key_%010d", i))
			if err := db.Set(key, val, pebble.Sync); err != nil {
				t.Fatal(err)
			}
		}
		dur := time.Since(start)
		ops := float64(numOps) / dur.Seconds()
		results = append(results, result{"Pebble (Sync)", ops})
		t.Logf("Pebble Sync: %d ops in %v = %.0f ops/sec", numOps, dur, ops)
	})

	// --- Pebble NoSync ---
	t.Run("Pebble_NoSync", func(t *testing.T) {
		dir := t.TempDir()
		db, err := pebble.Open(dir, &pebble.Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		val := bytes.Repeat([]byte("v"), valueSize)
		start := time.Now()
		for i := 0; i < numOps; i++ {
			key := []byte(fmt.Sprintf("key_%010d", i))
			if err := db.Set(key, val, pebble.NoSync); err != nil {
				t.Fatal(err)
			}
		}
		dur := time.Since(start)
		ops := float64(numOps) / dur.Seconds()
		results = append(results, result{"Pebble (NoSync)", ops})
		t.Logf("Pebble NoSync: %d ops in %v = %.0f ops/sec", numOps, dur, ops)
	})

	// --- LevelDB Sync ---
	t.Run("LevelDB_Sync", func(t *testing.T) {
		dir := t.TempDir()
		db, err := leveldb.OpenFile(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		val := bytes.Repeat([]byte("v"), valueSize)
		wo := &opt.WriteOptions{Sync: true}
		start := time.Now()
		for i := 0; i < numOps; i++ {
			key := []byte(fmt.Sprintf("key_%010d", i))
			if err := db.Put(key, val, wo); err != nil {
				t.Fatal(err)
			}
		}
		dur := time.Since(start)
		ops := float64(numOps) / dur.Seconds()
		results = append(results, result{"LevelDB (Sync)", ops})
		t.Logf("LevelDB Sync: %d ops in %v = %.0f ops/sec", numOps, dur, ops)
	})

	// --- LevelDB NoSync ---
	t.Run("LevelDB_NoSync", func(t *testing.T) {
		dir := t.TempDir()
		db, err := leveldb.OpenFile(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		val := bytes.Repeat([]byte("v"), valueSize)
		start := time.Now()
		for i := 0; i < numOps; i++ {
			key := []byte(fmt.Sprintf("key_%010d", i))
			if err := db.Put(key, val, nil); err != nil {
				t.Fatal(err)
			}
		}
		dur := time.Since(start)
		ops := float64(numOps) / dur.Seconds()
		results = append(results, result{"LevelDB (NoSync)", ops})
		t.Logf("LevelDB NoSync: %d ops in %v = %.0f ops/sec", numOps, dur, ops)
	})

	// --- HooDB API 并发写入 ---
	t.Run("HooDB_API_Concurrent", func(t *testing.T) {
		resp, err := http.Get("http://localhost:8001/health")
		if err != nil {
			t.Skipf("HooDB cluster not running, skipping: %v", err)
			return
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Skipf("HooDB cluster not healthy, skipping")
			return
		}

		concurrency := 50
		var success, failed int64
		start := time.Now()
		var wg sync.WaitGroup
		tasks := make(chan int, numOps)
		for i := 0; i < numOps; i++ {
			tasks <- i
		}
		close(tasks)

		for w := 0; w < concurrency; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				client := &http.Client{Timeout: 5 * time.Second}
				for i := range tasks {
					body := fmt.Sprintf(`{"value":"val_%d"}`, i)
					url := fmt.Sprintf("http://localhost:8001/kv/bench_%d", i)
					req, _ := http.NewRequest("PUT", url, bytes.NewBufferString(body))
					req.Header.Set("Content-Type", "application/json")
					resp, err := client.Do(req)
					if err != nil {
						atomic.AddInt64(&failed, 1)
						continue
					}
					resp.Body.Close()
					if resp.StatusCode == 200 {
						atomic.AddInt64(&success, 1)
					} else {
						atomic.AddInt64(&failed, 1)
					}
				}
			}()
		}
		wg.Wait()
		dur := time.Since(start)
		ops := float64(success) / dur.Seconds()
		results = append(results, result{"HooDB API (50并发)", ops})
		t.Logf("HooDB API: %d ops in %v = %.0f ops/sec (success=%d, failed=%d)",
			numOps, dur, ops, success, failed)
	})

	// --- HooDB Batch API ---
	t.Run("HooDB_API_Batch", func(t *testing.T) {
		resp, err := http.Get("http://localhost:8001/health")
		if err != nil {
			t.Skipf("HooDB cluster not running, skipping: %v", err)
			return
		}
		resp.Body.Close()

		batchSize := 100
		batches := numOps / batchSize
		var success, failed int64
		start := time.Now()
		client := &http.Client{Timeout: 5 * time.Second}

		for b := 0; b < batches; b++ {
			items := make(map[string]string, batchSize)
			for j := 0; j < batchSize; j++ {
				idx := b*batchSize + j
				items[fmt.Sprintf("batch_%d", idx)] = fmt.Sprintf("val_%d", idx)
			}
			body, _ := json.Marshal(map[string]interface{}{"items": items})
			req, _ := http.NewRequest("POST", "http://localhost:8001/kv/batch", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				atomic.AddInt64(&failed, int64(batchSize))
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == 200 {
				atomic.AddInt64(&success, int64(batchSize))
			} else {
				atomic.AddInt64(&failed, int64(batchSize))
			}
		}
		dur := time.Since(start)
		ops := float64(success) / dur.Seconds()
		results = append(results, result{"HooDB Batch API", ops})
		t.Logf("HooDB Batch: %d ops in %v = %.0f ops/sec (success=%d, failed=%d)",
			numOps, dur, ops, success, failed)
	})

	// 打印汇总
	t.Log("")
	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║          HooDB vs LevelDB 性能对比 (10,000 ops, 100B value)     ║")
	t.Log("╠══════════════════════════════════════════════════════════════════╣")
	for _, r := range results {
		bar := ""
		barLen := int(r.ops / 1000)
		if barLen > 40 {
			barLen = 40
		}
		for j := 0; j < barLen; j++ {
			bar += "█"
		}
		t.Logf("║  %-22s  %10.0f ops/sec  %s", r.name, r.ops, bar)
	}
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
}
