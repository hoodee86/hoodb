package main

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
	"github.com/syndtr/goleveldb/leveldb"
	levelopt "github.com/syndtr/goleveldb/leveldb/opt"
)

// ========== 测试参数 ==========

const (
	numOps      = 50000 // 每项测试的操作数
	valueSize   = 256   // value 大小(字节)
	batchSize   = 1000  // 批量写入每批数量
	concurrency = 8     // 并发线程数
	rwOps       = 20000 // 混合读写测试操作数
)

// ========== 数据结构 ==========

type BenchmarkResult struct {
	Engine       string
	Operation    string
	TotalOps     int
	Duration     time.Duration
	OpsPerSec    float64
	AvgLatencyUs float64 // 微秒
}

// ========== 工具函数 ==========

func generateValue(size int) []byte {
	value := make([]byte, size)
	rand.Read(value)
	return value
}

func makeKey(i int) []byte {
	return []byte(fmt.Sprintf("key_%010d", i))
}

func printResult(r BenchmarkResult) {
	fmt.Printf("  %-28s %8d ops  %10.0f ops/sec  avg=%.1f µs\n",
		r.Operation, r.TotalOps, r.OpsPerSec, r.AvgLatencyUs)
}

// ========== Pebble 优化配置 ==========

func openOptimizedPebble(path string) *pebble.DB {
	cache := pebble.NewCache(256 << 20) // 256MB

	opts := &pebble.Options{
		Cache:        cache,
		MemTableSize: 64 << 20, // 64MB

		L0CompactionThreshold: 4,
		L0StopWritesThreshold: 12,
		LBaseMaxBytes:         64 << 20,

		MaxConcurrentCompactions: func() int { return runtime.NumCPU() },
		MaxOpenFiles:             10000,
		DisableWAL:               false,

		Levels: []pebble.LevelOptions{
			{TargetFileSize: 8 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},
			{TargetFileSize: 16 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},
			{TargetFileSize: 32 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},
			{TargetFileSize: 64 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},
			{TargetFileSize: 128 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},
			{TargetFileSize: 256 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},
			{TargetFileSize: 256 << 20, FilterPolicy: bloom.FilterPolicy(10), Compression: pebble.SnappyCompression},
		},
	}

	db, err := pebble.Open(path, opts)
	cache.Unref()
	if err != nil {
		panic(fmt.Sprintf("failed to open pebble: %v", err))
	}
	return db
}

func openOptimizedLevelDB(path string) *leveldb.DB {
	opts := &levelopt.Options{
		BlockCacheCapacity:     256 << 20, // 256MB 与 Pebble 一致
		WriteBuffer:            64 << 20,  // 64MB 与 Pebble 一致
		CompactionTableSize:    8 << 20,
		OpenFilesCacheCapacity: 10000,
	}

	db, err := leveldb.OpenFile(path, opts)
	if err != nil {
		panic(fmt.Sprintf("failed to open leveldb: %v", err))
	}
	return db
}

// ========== Pebble 测试 ==========

func benchmarkPebble() []BenchmarkResult {
	var results []BenchmarkResult
	dbPath := "./bench_pebble"
	os.RemoveAll(dbPath)
	defer os.RemoveAll(dbPath)

	db := openOptimizedPebble(dbPath)
	defer db.Close()

	fmt.Println("\n━━━ Pebble 性能测试 ━━━")

	// 1. 顺序写入 (NoSync)
	{
		start := time.Now()
		for i := 0; i < numOps; i++ {
			key := makeKey(i)
			value := generateValue(valueSize)
			if err := db.Set(key, value, pebble.NoSync); err != nil {
				panic(err)
			}
		}
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "Pebble", Operation: "Seq Write (NoSync)",
			TotalOps: numOps, Duration: dur,
			OpsPerSec:    float64(numOps) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(numOps),
		}
		results = append(results, r)
		printResult(r)
	}

	// 2. 顺序写入 (Sync)
	{
		syncPath := "./bench_pebble_sync"
		os.RemoveAll(syncPath)
		defer os.RemoveAll(syncPath)
		syncDB := openOptimizedPebble(syncPath)

		ops := 2000
		start := time.Now()
		for i := 0; i < ops; i++ {
			key := makeKey(i)
			value := generateValue(valueSize)
			if err := syncDB.Set(key, value, pebble.Sync); err != nil {
				panic(err)
			}
		}
		dur := time.Since(start)
		syncDB.Close()
		r := BenchmarkResult{
			Engine: "Pebble", Operation: "Seq Write (Sync)",
			TotalOps: ops, Duration: dur,
			OpsPerSec:    float64(ops) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(ops),
		}
		results = append(results, r)
		printResult(r)
	}

	// 3. 顺序读取
	{
		start := time.Now()
		for i := 0; i < numOps; i++ {
			key := makeKey(i)
			_, closer, err := db.Get(key)
			if err != nil && err != pebble.ErrNotFound {
				panic(err)
			}
			if closer != nil {
				closer.Close()
			}
		}
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "Pebble", Operation: "Sequential Read",
			TotalOps: numOps, Duration: dur,
			OpsPerSec:    float64(numOps) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(numOps),
		}
		results = append(results, r)
		printResult(r)
	}

	// 4. 随机读取
	{
		start := time.Now()
		for i := 0; i < numOps; i++ {
			key := makeKey(rand.Intn(numOps))
			_, closer, err := db.Get(key)
			if err != nil && err != pebble.ErrNotFound {
				panic(err)
			}
			if closer != nil {
				closer.Close()
			}
		}
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "Pebble", Operation: "Random Read",
			TotalOps: numOps, Duration: dur,
			OpsPerSec:    float64(numOps) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(numOps),
		}
		results = append(results, r)
		printResult(r)
	}

	// 5. 批量写入 (NoSync)
	{
		start := time.Now()
		totalBatches := numOps / batchSize
		for b := 0; b < totalBatches; b++ {
			batch := db.NewBatch()
			for i := 0; i < batchSize; i++ {
				key := []byte(fmt.Sprintf("batch_%010d", b*batchSize+i))
				value := generateValue(valueSize)
				if err := batch.Set(key, value, nil); err != nil {
					panic(err)
				}
			}
			if err := batch.Commit(pebble.NoSync); err != nil {
				panic(err)
			}
		}
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "Pebble", Operation: "Batch Write (NoSync)",
			TotalOps: numOps, Duration: dur,
			OpsPerSec:    float64(numOps) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(totalBatches),
		}
		results = append(results, r)
		printResult(r)
	}

	// 6. 批量写入 (Sync)
	{
		start := time.Now()
		totalBatches := numOps / batchSize
		for b := 0; b < totalBatches; b++ {
			batch := db.NewBatch()
			for i := 0; i < batchSize; i++ {
				key := []byte(fmt.Sprintf("batchs_%010d", b*batchSize+i))
				value := generateValue(valueSize)
				if err := batch.Set(key, value, nil); err != nil {
					panic(err)
				}
			}
			if err := batch.Commit(pebble.Sync); err != nil {
				panic(err)
			}
		}
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "Pebble", Operation: "Batch Write (Sync)",
			TotalOps: numOps, Duration: dur,
			OpsPerSec:    float64(numOps) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(totalBatches),
		}
		results = append(results, r)
		printResult(r)
	}

	// 7. 并发写入 (NoSync)
	{
		var totalDone atomic.Int64
		opsPerThread := numOps / concurrency
		start := time.Now()
		var wg sync.WaitGroup
		for t := 0; t < concurrency; t++ {
			wg.Add(1)
			go func(tid int) {
				defer wg.Done()
				for i := 0; i < opsPerThread; i++ {
					key := []byte(fmt.Sprintf("conc_%d_%010d", tid, i))
					value := generateValue(valueSize)
					if err := db.Set(key, value, pebble.NoSync); err != nil {
						panic(err)
					}
					totalDone.Add(1)
				}
			}(t)
		}
		wg.Wait()
		dur := time.Since(start)
		total := int(totalDone.Load())
		r := BenchmarkResult{
			Engine: "Pebble", Operation: fmt.Sprintf("Concurrent Write x%d", concurrency),
			TotalOps: total, Duration: dur,
			OpsPerSec:    float64(total) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(total),
		}
		results = append(results, r)
		printResult(r)
	}

	// 8. 并发读取
	{
		var totalDone atomic.Int64
		opsPerThread := numOps / concurrency
		start := time.Now()
		var wg sync.WaitGroup
		for t := 0; t < concurrency; t++ {
			wg.Add(1)
			go func(tid int) {
				defer wg.Done()
				for i := 0; i < opsPerThread; i++ {
					key := makeKey(rand.Intn(numOps))
					_, closer, err := db.Get(key)
					if err != nil && err != pebble.ErrNotFound {
						panic(err)
					}
					if closer != nil {
						closer.Close()
					}
					totalDone.Add(1)
				}
			}(t)
		}
		wg.Wait()
		dur := time.Since(start)
		total := int(totalDone.Load())
		r := BenchmarkResult{
			Engine: "Pebble", Operation: fmt.Sprintf("Concurrent Read x%d", concurrency),
			TotalOps: total, Duration: dur,
			OpsPerSec:    float64(total) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(total),
		}
		results = append(results, r)
		printResult(r)
	}

	// 9. 混合读写 (7R:3W, NoSync)
	{
		var totalDone atomic.Int64
		start := time.Now()
		var wg sync.WaitGroup
		for t := 0; t < concurrency; t++ {
			wg.Add(1)
			go func(tid int) {
				defer wg.Done()
				opsEach := rwOps / concurrency
				for i := 0; i < opsEach; i++ {
					if rand.Float32() < 0.7 {
						key := makeKey(rand.Intn(numOps))
						_, closer, err := db.Get(key)
						if err != nil && err != pebble.ErrNotFound {
							panic(err)
						}
						if closer != nil {
							closer.Close()
						}
					} else {
						key := []byte(fmt.Sprintf("mix_%d_%010d", tid, i))
						value := generateValue(valueSize)
						if err := db.Set(key, value, pebble.NoSync); err != nil {
							panic(err)
						}
					}
					totalDone.Add(1)
				}
			}(t)
		}
		wg.Wait()
		dur := time.Since(start)
		total := int(totalDone.Load())
		r := BenchmarkResult{
			Engine: "Pebble", Operation: "Mixed R/W 7:3",
			TotalOps: total, Duration: dur,
			OpsPerSec:    float64(total) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(total),
		}
		results = append(results, r)
		printResult(r)
	}

	// 10. 迭代器扫描
	{
		start := time.Now()
		iter, err := db.NewIter(nil)
		if err != nil {
			panic(err)
		}
		count := 0
		for iter.First(); iter.Valid() && count < numOps; iter.Next() {
			_ = iter.Key()
			_ = iter.Value()
			count++
		}
		iter.Close()
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "Pebble", Operation: "Iterator Scan",
			TotalOps: count, Duration: dur,
			OpsPerSec:    float64(count) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(count),
		}
		results = append(results, r)
		printResult(r)
	}

	return results
}

// ========== LevelDB 测试 ==========

func benchmarkLevelDB() []BenchmarkResult {
	var results []BenchmarkResult
	dbPath := "./bench_leveldb"
	os.RemoveAll(dbPath)
	defer os.RemoveAll(dbPath)

	db := openOptimizedLevelDB(dbPath)
	defer db.Close()

	fmt.Println("\n━━━ LevelDB 性能测试 ━━━")

	woNoSync := &levelopt.WriteOptions{Sync: false}
	woSync := &levelopt.WriteOptions{Sync: true}

	// 1. 顺序写入 (NoSync)
	{
		start := time.Now()
		for i := 0; i < numOps; i++ {
			key := makeKey(i)
			value := generateValue(valueSize)
			if err := db.Put(key, value, woNoSync); err != nil {
				panic(err)
			}
		}
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "LevelDB", Operation: "Seq Write (NoSync)",
			TotalOps: numOps, Duration: dur,
			OpsPerSec:    float64(numOps) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(numOps),
		}
		results = append(results, r)
		printResult(r)
	}

	// 2. 顺序写入 (Sync)
	{
		syncPath := "./bench_leveldb_sync"
		os.RemoveAll(syncPath)
		defer os.RemoveAll(syncPath)
		syncDB := openOptimizedLevelDB(syncPath)

		ops := 2000
		start := time.Now()
		for i := 0; i < ops; i++ {
			key := makeKey(i)
			value := generateValue(valueSize)
			if err := syncDB.Put(key, value, woSync); err != nil {
				panic(err)
			}
		}
		dur := time.Since(start)
		syncDB.Close()
		r := BenchmarkResult{
			Engine: "LevelDB", Operation: "Seq Write (Sync)",
			TotalOps: ops, Duration: dur,
			OpsPerSec:    float64(ops) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(ops),
		}
		results = append(results, r)
		printResult(r)
	}

	// 3. 顺序读取
	{
		start := time.Now()
		for i := 0; i < numOps; i++ {
			key := makeKey(i)
			_, err := db.Get(key, nil)
			if err != nil && err != leveldb.ErrNotFound {
				panic(err)
			}
		}
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "LevelDB", Operation: "Sequential Read",
			TotalOps: numOps, Duration: dur,
			OpsPerSec:    float64(numOps) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(numOps),
		}
		results = append(results, r)
		printResult(r)
	}

	// 4. 随机读取
	{
		start := time.Now()
		for i := 0; i < numOps; i++ {
			key := makeKey(rand.Intn(numOps))
			_, err := db.Get(key, nil)
			if err != nil && err != leveldb.ErrNotFound {
				panic(err)
			}
		}
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "LevelDB", Operation: "Random Read",
			TotalOps: numOps, Duration: dur,
			OpsPerSec:    float64(numOps) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(numOps),
		}
		results = append(results, r)
		printResult(r)
	}

	// 5. 批量写入 (NoSync)
	{
		start := time.Now()
		totalBatches := numOps / batchSize
		for b := 0; b < totalBatches; b++ {
			batch := new(leveldb.Batch)
			for i := 0; i < batchSize; i++ {
				key := []byte(fmt.Sprintf("batch_%010d", b*batchSize+i))
				value := generateValue(valueSize)
				batch.Put(key, value)
			}
			if err := db.Write(batch, woNoSync); err != nil {
				panic(err)
			}
		}
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "LevelDB", Operation: "Batch Write (NoSync)",
			TotalOps: numOps, Duration: dur,
			OpsPerSec:    float64(numOps) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(totalBatches),
		}
		results = append(results, r)
		printResult(r)
	}

	// 6. 批量写入 (Sync)
	{
		start := time.Now()
		totalBatches := numOps / batchSize
		for b := 0; b < totalBatches; b++ {
			batch := new(leveldb.Batch)
			for i := 0; i < batchSize; i++ {
				key := []byte(fmt.Sprintf("batchs_%010d", b*batchSize+i))
				value := generateValue(valueSize)
				batch.Put(key, value)
			}
			if err := db.Write(batch, woSync); err != nil {
				panic(err)
			}
		}
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "LevelDB", Operation: "Batch Write (Sync)",
			TotalOps: numOps, Duration: dur,
			OpsPerSec:    float64(numOps) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(totalBatches),
		}
		results = append(results, r)
		printResult(r)
	}

	// 7. 并发写入 (NoSync)
	{
		var totalDone atomic.Int64
		opsPerThread := numOps / concurrency
		start := time.Now()
		var wg sync.WaitGroup
		for t := 0; t < concurrency; t++ {
			wg.Add(1)
			go func(tid int) {
				defer wg.Done()
				for i := 0; i < opsPerThread; i++ {
					key := []byte(fmt.Sprintf("conc_%d_%010d", tid, i))
					value := generateValue(valueSize)
					if err := db.Put(key, value, woNoSync); err != nil {
						panic(err)
					}
					totalDone.Add(1)
				}
			}(t)
		}
		wg.Wait()
		dur := time.Since(start)
		total := int(totalDone.Load())
		r := BenchmarkResult{
			Engine: "LevelDB", Operation: fmt.Sprintf("Concurrent Write x%d", concurrency),
			TotalOps: total, Duration: dur,
			OpsPerSec:    float64(total) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(total),
		}
		results = append(results, r)
		printResult(r)
	}

	// 8. 并发读取
	{
		var totalDone atomic.Int64
		opsPerThread := numOps / concurrency
		start := time.Now()
		var wg sync.WaitGroup
		for t := 0; t < concurrency; t++ {
			wg.Add(1)
			go func(tid int) {
				defer wg.Done()
				for i := 0; i < opsPerThread; i++ {
					key := makeKey(rand.Intn(numOps))
					_, err := db.Get(key, nil)
					if err != nil && err != leveldb.ErrNotFound {
						panic(err)
					}
					totalDone.Add(1)
				}
			}(t)
		}
		wg.Wait()
		dur := time.Since(start)
		total := int(totalDone.Load())
		r := BenchmarkResult{
			Engine: "LevelDB", Operation: fmt.Sprintf("Concurrent Read x%d", concurrency),
			TotalOps: total, Duration: dur,
			OpsPerSec:    float64(total) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(total),
		}
		results = append(results, r)
		printResult(r)
	}

	// 9. 混合读写 (7R:3W, NoSync)
	{
		var totalDone atomic.Int64
		start := time.Now()
		var wg sync.WaitGroup
		for t := 0; t < concurrency; t++ {
			wg.Add(1)
			go func(tid int) {
				defer wg.Done()
				opsEach := rwOps / concurrency
				for i := 0; i < opsEach; i++ {
					if rand.Float32() < 0.7 {
						key := makeKey(rand.Intn(numOps))
						_, err := db.Get(key, nil)
						if err != nil && err != leveldb.ErrNotFound {
							panic(err)
						}
					} else {
						key := []byte(fmt.Sprintf("mix_%d_%010d", tid, i))
						value := generateValue(valueSize)
						if err := db.Put(key, value, woNoSync); err != nil {
							panic(err)
						}
					}
					totalDone.Add(1)
				}
			}(t)
		}
		wg.Wait()
		dur := time.Since(start)
		total := int(totalDone.Load())
		r := BenchmarkResult{
			Engine: "LevelDB", Operation: "Mixed R/W 7:3",
			TotalOps: total, Duration: dur,
			OpsPerSec:    float64(total) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(total),
		}
		results = append(results, r)
		printResult(r)
	}

	// 10. 迭代器扫描
	{
		start := time.Now()
		iter := db.NewIterator(nil, nil)
		count := 0
		for iter.Next() && count < numOps {
			_ = iter.Key()
			_ = iter.Value()
			count++
		}
		iter.Release()
		dur := time.Since(start)
		r := BenchmarkResult{
			Engine: "LevelDB", Operation: "Iterator Scan",
			TotalOps: count, Duration: dur,
			OpsPerSec:    float64(count) / dur.Seconds(),
			AvgLatencyUs: dur.Seconds() * 1e6 / float64(count),
		}
		results = append(results, r)
		printResult(r)
	}

	return results
}

// ========== 对比输出 ==========

func printComparison(pebbleResults, levelResults []BenchmarkResult) {
	// 按 operation 名做匹配
	opMap := map[string][2]*BenchmarkResult{}
	var ops []string
	for i := range pebbleResults {
		r := &pebbleResults[i]
		entry := opMap[r.Operation]
		entry[0] = r
		opMap[r.Operation] = entry
		ops = append(ops, r.Operation)
	}
	for i := range levelResults {
		r := &levelResults[i]
		entry := opMap[r.Operation]
		entry[1] = r
		opMap[r.Operation] = entry
	}

	fmt.Println("\n" + strings.Repeat("═", 100))
	fmt.Println("  性能对比总结  (Pebble vs LevelDB)")
	fmt.Println(strings.Repeat("═", 100))
	fmt.Printf("\n  %-28s %14s %14s %16s\n", "测试项", "Pebble", "LevelDB", "差异")
	fmt.Println("  " + strings.Repeat("─", 80))

	for _, op := range ops {
		entry := opMap[op]
		p := entry[0]
		l := entry[1]
		if p == nil || l == nil {
			continue
		}
		ratio := p.OpsPerSec / l.OpsPerSec
		bar := ""
		if ratio >= 1.0 {
			bar = fmt.Sprintf("Pebble +%.0f%%", (ratio-1)*100)
		} else {
			bar = fmt.Sprintf("LevelDB +%.0f%%", (1/ratio-1)*100)
		}
		fmt.Printf("  %-28s %11.0f/s %11.0f/s   %-16s\n",
			op, p.OpsPerSec, l.OpsPerSec, bar)
	}
	fmt.Println("  " + strings.Repeat("─", 80))

	fmt.Println("\n  说明:")
	fmt.Println("  • NoSync = 写入 OS 缓存, 无 fsync (适合 Raft 日志保证持久性)")
	fmt.Println("  • Sync   = 每次 fsync 到磁盘 (最高持久性 — 公平对比)")
	fmt.Println("  • 之前的测试 Pebble 用 Sync 而 LevelDB 用 NoSync, 对比不公平")
	fmt.Printf("  • 参数: %d ops, value=%dB, batch=%d, concurrency=%d\n",
		numOps, valueSize, batchSize, concurrency)
}

func main() {
	fmt.Println("╔════════════════════════════════════════════════════╗")
	fmt.Println("║     存储引擎性能对比测试 (Pebble vs LevelDB)          ║")
	fmt.Println("╠════════════════════════════════════════════════════╣")
	fmt.Printf("║  操作数: %d   值大小: %d B   并发: %d            ║\n", numOps, valueSize, concurrency)
	fmt.Printf("║  Go: %s   CPU: %d cores                      ║\n", runtime.Version(), runtime.NumCPU())
	fmt.Println("╚════════════════════════════════════════════════════╝")

	pebbleResults := benchmarkPebble()
	levelResults := benchmarkLevelDB()

	printComparison(pebbleResults, levelResults)

	fmt.Println("\n测试完成！")
}
