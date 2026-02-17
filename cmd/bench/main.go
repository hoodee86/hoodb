package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// ─── 配置 ──────────────────────────────────────────────

var (
	hoodbURL    = flag.String("url", "http://localhost:8001", "HooDB leader URL")
	numOps      = flag.Int("n", 10000, "Number of operations per test")
	concurrency = flag.Int("c", 50, "Concurrency for HooDB API tests")
	valueSize   = flag.Int("vsize", 100, "Value size in bytes")
	batchSize   = flag.Int("batch", 100, "Batch size for batch write tests")
	skipHooDB   = flag.Bool("skip-hoodb", false, "Skip HooDB API tests")
)

// ─── Result ────────────────────────────────────────────

type benchResult struct {
	group     string
	name      string
	ops       int
	success   int
	failed    int
	dur       time.Duration
	opsPerS   float64
	valueSize int // bytes per operation
	mbPerS    float64
}

func (r benchResult) String() string {
	return fmt.Sprintf("%-38s  %10.1f MB/s  %10.0f ops/s  %8d ok  %6d fail  %v",
		r.name, r.mbPerS, r.opsPerS, r.success, r.failed, r.dur.Round(time.Millisecond))
}

var allResults []benchResult

func record(r benchResult) {
	r.opsPerS = float64(r.success) / r.dur.Seconds()
	if r.valueSize > 0 {
		totalBytes := float64(r.success*r.valueSize) / (1024 * 1024)
		r.mbPerS = totalBytes / r.dur.Seconds()
	}
	allResults = append(allResults, r)
	fmt.Printf("  ✓ %-36s %10.1f MB/s  (%v)\n", r.name, r.mbPerS, r.dur.Round(time.Millisecond))
}

// ─── Helpers ────────────────────────────────────────────

func randomValue(size int) string {
	b := make([]byte, size)
	for i := range b {
		b[i] = byte('a' + rand.Intn(26))
	}
	return string(b)
}

func httpClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 200,
			MaxIdleConns:        200,
			IdleConnTimeout:     30 * time.Second,
		},
	}
}

func checkHooDB(baseURL string) error {
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		return fmt.Errorf("cannot reach HooDB at %s: %w", baseURL, err)
	}
	resp.Body.Close()
	return nil
}

// ─── HooDB: 并发单条写入 ───────────────────────────────

func benchHooDBWriteConcurrent(baseURL string, n, conc, vsize int) {
	val := randomValue(vsize)
	var success, failed int64
	var wg sync.WaitGroup
	tasks := make(chan int, n)
	for i := 0; i < n; i++ {
		tasks <- i
	}
	close(tasks)

	start := time.Now()
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := httpClient()
			for i := range tasks {
				body := fmt.Sprintf(`{"value":"%s"}`, val)
				url := fmt.Sprintf("%s/kv/bench_w_%d", baseURL, i)
				req, _ := http.NewRequest("PUT", url, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := c.Do(req)
				if err != nil {
					atomic.AddInt64(&failed, 1)
					continue
				}
				io.Copy(io.Discard, resp.Body)
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

	record(benchResult{
		group: "HooDB API", name: fmt.Sprintf("HooDB API Write (%d 并发)", conc),
		ops: n, success: int(success), failed: int(failed), dur: dur, valueSize: vsize,
	})
}

// ─── HooDB: 批量写入 ───────────────────────────────────

func benchHooDBWriteBatch(baseURL string, n, bsize, vsize int) {
	val := randomValue(vsize)
	batches := n / bsize
	if batches == 0 {
		batches = 1
	}
	var success, failed int64

	start := time.Now()
	c := httpClient()
	for b := 0; b < batches; b++ {
		items := make(map[string]string, bsize)
		for j := 0; j < bsize; j++ {
			idx := b*bsize + j
			items[fmt.Sprintf("bench_b_%d", idx)] = val
		}
		body, _ := json.Marshal(map[string]interface{}{"items": items})
		req, _ := http.NewRequest("POST", baseURL+"/kv/batch", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			atomic.AddInt64(&failed, int64(bsize))
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			atomic.AddInt64(&success, int64(bsize))
		} else {
			atomic.AddInt64(&failed, int64(bsize))
		}
	}
	dur := time.Since(start)

	record(benchResult{
		group: "HooDB API", name: fmt.Sprintf("HooDB API BatchWrite (batch=%d)", bsize),
		ops: n, success: int(success), failed: int(failed), dur: dur, valueSize: vsize,
	})
}

// ─── HooDB: 并发批量写入 ───────────────────────────────

func benchHooDBWriteBatchConcurrent(baseURL string, n, bsize, conc, vsize int) {
	val := randomValue(vsize)
	batches := n / bsize
	if batches == 0 {
		batches = 1
	}
	var success, failed int64
	var wg sync.WaitGroup
	tasks := make(chan int, batches)
	for b := 0; b < batches; b++ {
		tasks <- b
	}
	close(tasks)

	start := time.Now()
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := httpClient()
			for b := range tasks {
				items := make(map[string]string, bsize)
				for j := 0; j < bsize; j++ {
					idx := b*bsize + j
					items[fmt.Sprintf("bench_bc_%d", idx)] = val
				}
				body, _ := json.Marshal(map[string]interface{}{"items": items})
				req, _ := http.NewRequest("POST", baseURL+"/kv/batch", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := c.Do(req)
				if err != nil {
					atomic.AddInt64(&failed, int64(bsize))
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode == 200 {
					atomic.AddInt64(&success, int64(bsize))
				} else {
					atomic.AddInt64(&failed, int64(bsize))
				}
			}
		}()
	}
	wg.Wait()
	dur := time.Since(start)

	record(benchResult{
		group: "HooDB API", name: fmt.Sprintf("HooDB API BatchWrite (%d并发, batch=%d)", conc, bsize),
		ops: n, success: int(success), failed: int(failed), dur: dur, valueSize: vsize,
	})
}

// ─── HooDB: 并发读取 ───────────────────────────────────

func benchHooDBReadConcurrent(baseURL string, n, conc int) {
	var success, failed int64
	var wg sync.WaitGroup
	tasks := make(chan int, n)
	for i := 0; i < n; i++ {
		tasks <- i
	}
	close(tasks)

	start := time.Now()
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := httpClient()
			for i := range tasks {
				url := fmt.Sprintf("%s/kv/bench_w_%d", baseURL, i%n)
				resp, err := c.Get(url)
				if err != nil {
					atomic.AddInt64(&failed, 1)
					continue
				}
				io.Copy(io.Discard, resp.Body)
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

	record(benchResult{
		group: "HooDB API", name: fmt.Sprintf("HooDB API Read (%d 并发)", conc),
		ops: n, success: int(success), failed: int(failed), dur: dur, valueSize: *valueSize,
	})
}

// ─── HooDB: 服务端基准测试 API ─────────────────────────

func benchHooDBServerSide(baseURL string, n, conc int) {
	body, _ := json.Marshal(map[string]int{"count": n, "concurrency": conc})
	c := httpClient()
	c.Timeout = 120 * time.Second

	start := time.Now()
	req, _ := http.NewRequest("POST", baseURL+"/benchmark", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		fmt.Printf("  ✗ HooDB Server-side benchmark failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Count   int     `json:"count"`
		Success int     `json:"success"`
		Failed  int     `json:"failed"`
		Millis  float64 `json:"duration_ms"`
		OpsPerS float64 `json:"ops_per_sec"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Printf("  ✗ Failed to parse response: %v\n", err)
		return
	}
	dur := time.Since(start)

	record(benchResult{
		group: "HooDB API", name: fmt.Sprintf("HooDB Server-side (%d 并发)", conc),
		ops: n, success: result.Success, failed: result.Failed, dur: dur, valueSize: *valueSize,
	})
}

// ─── LevelDB: 顺序写入 (Sync) ─────────────────────────

func benchLevelDBWriteSync(n, vsize int) {
	dir, _ := os.MkdirTemp("", "bench-leveldb-sync-*")
	defer os.RemoveAll(dir)

	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		fmt.Printf("  ✗ LevelDB open failed: %v\n", err)
		return
	}
	defer db.Close()

	val := []byte(randomValue(vsize))
	wo := &opt.WriteOptions{Sync: true}
	success := 0

	start := time.Now()
	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		if err := db.Put(key, val, wo); err == nil {
			success++
		}
	}
	dur := time.Since(start)

	record(benchResult{
		group: "LevelDB", name: "LevelDB SeqWrite (Sync)",
		ops: n, success: success, failed: n - success, dur: dur, valueSize: vsize,
	})
}

// ─── LevelDB: 顺序写入 (NoSync) ───────────────────────

func benchLevelDBWriteNoSync(n, vsize int) {
	dir, _ := os.MkdirTemp("", "bench-leveldb-nosync-*")
	defer os.RemoveAll(dir)

	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		fmt.Printf("  ✗ LevelDB open failed: %v\n", err)
		return
	}
	defer db.Close()

	val := []byte(randomValue(vsize))
	success := 0

	start := time.Now()
	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		if err := db.Put(key, val, nil); err == nil {
			success++
		}
	}
	dur := time.Since(start)

	record(benchResult{
		group: "LevelDB", name: "LevelDB SeqWrite (NoSync)",
		ops: n, success: success, failed: n - success, dur: dur, valueSize: vsize,
	})
}

// ─── LevelDB: 批量写入 ────────────────────────────────

func benchLevelDBWriteBatch(n, bsize, vsize int) {
	dir, _ := os.MkdirTemp("", "bench-leveldb-batch-*")
	defer os.RemoveAll(dir)

	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		fmt.Printf("  ✗ LevelDB open failed: %v\n", err)
		return
	}
	defer db.Close()

	val := []byte(randomValue(vsize))
	success := 0
	batches := n / bsize

	start := time.Now()
	for b := 0; b < batches; b++ {
		batch := new(leveldb.Batch)
		for j := 0; j < bsize; j++ {
			idx := b*bsize + j
			key := []byte(fmt.Sprintf("key_%010d", idx))
			batch.Put(key, val)
		}
		if err := db.Write(batch, nil); err == nil {
			success += bsize
		}
	}
	dur := time.Since(start)

	record(benchResult{
		group: "LevelDB", name: fmt.Sprintf("LevelDB BatchWrite (batch=%d)", bsize),
		ops: n, success: success, failed: n - success, dur: dur, valueSize: vsize,
	})
}

// ─── LevelDB: 并发写入 ────────────────────────────────

func benchLevelDBWriteConcurrent(n, conc, vsize int) {
	dir, _ := os.MkdirTemp("", "bench-leveldb-conc-*")
	defer os.RemoveAll(dir)

	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		fmt.Printf("  ✗ LevelDB open failed: %v\n", err)
		return
	}
	defer db.Close()

	val := []byte(randomValue(vsize))
	var success, failed int64
	var wg sync.WaitGroup
	tasks := make(chan int, n)
	for i := 0; i < n; i++ {
		tasks <- i
	}
	close(tasks)

	start := time.Now()
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range tasks {
				key := []byte(fmt.Sprintf("key_%010d", i))
				if err := db.Put(key, val, nil); err != nil {
					atomic.AddInt64(&failed, 1)
				} else {
					atomic.AddInt64(&success, 1)
				}
			}
		}()
	}
	wg.Wait()
	dur := time.Since(start)

	record(benchResult{
		group: "LevelDB", name: fmt.Sprintf("LevelDB ConcurrentWrite (%d 并发)", conc),
		ops: n, success: int(success), failed: int(failed), dur: dur, valueSize: vsize,
	})
}

// ─── LevelDB: 顺序读取 ────────────────────────────────

func benchLevelDBRead(n, vsize int) {
	dir, _ := os.MkdirTemp("", "bench-leveldb-read-*")
	defer os.RemoveAll(dir)

	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		fmt.Printf("  ✗ LevelDB open failed: %v\n", err)
		return
	}
	defer db.Close()

	val := []byte(randomValue(vsize))
	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		db.Put(key, val, nil)
	}

	success := 0
	start := time.Now()
	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		if _, err := db.Get(key, nil); err == nil {
			success++
		}
	}
	dur := time.Since(start)

	record(benchResult{
		group: "LevelDB", name: "LevelDB SeqRead",
		ops: n, success: success, failed: n - success, dur: dur, valueSize: vsize,
	})
}

// ─── LevelDB: 并发读取 ────────────────────────────────

func benchLevelDBReadConcurrent(n, conc, vsize int) {
	dir, _ := os.MkdirTemp("", "bench-leveldb-cread-*")
	defer os.RemoveAll(dir)

	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		fmt.Printf("  ✗ LevelDB open failed: %v\n", err)
		return
	}
	defer db.Close()

	val := []byte(randomValue(vsize))
	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key_%010d", i))
		db.Put(key, val, nil)
	}

	var success, failed int64
	var wg sync.WaitGroup
	tasks := make(chan int, n)
	for i := 0; i < n; i++ {
		tasks <- i
	}
	close(tasks)

	start := time.Now()
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range tasks {
				key := []byte(fmt.Sprintf("key_%010d", i))
				if _, err := db.Get(key, nil); err == nil {
					atomic.AddInt64(&success, 1)
				} else {
					atomic.AddInt64(&failed, 1)
				}
			}
		}()
	}
	wg.Wait()
	dur := time.Since(start)

	record(benchResult{
		group: "LevelDB", name: fmt.Sprintf("LevelDB ConcurrentRead (%d 并发)", conc),
		ops: n, success: int(success), failed: int(failed), dur: dur, valueSize: vsize,
	})
}

// ─── 输出报告 ──────────────────────────────────────────

func printReport() {
	sep := strings.Repeat("=", 100)
	thin := strings.Repeat("-", 100)

	fmt.Println()
	fmt.Println("+" + sep + "+")
	fmt.Printf("|  %-98s|\n", fmt.Sprintf("HooDB vs LevelDB Throughput (n=%d, value=%dB, concurrency=%d)", *numOps, *valueSize, *concurrency))
	fmt.Println("+" + sep + "+")

	groups := []string{}
	grouped := map[string][]benchResult{}
	for _, r := range allResults {
		if _, ok := grouped[r.group]; !ok {
			groups = append(groups, r.group)
		}
		grouped[r.group] = append(grouped[r.group], r)
	}

	for gi, g := range groups {
		if gi > 0 {
			fmt.Println("|  " + thin[:96] + "  |")
		}
		fmt.Printf("|  %-98s|\n", "["+g+"]")
		for _, r := range grouped[g] {
			barLen := int(r.mbPerS / 50)
			if barLen > 30 {
				barLen = 30
			}
			bar := strings.Repeat("#", barLen)
			fmt.Printf("|  %-38s %10.1f MB/s %10.0f ops/s  %-25s |\n", r.name, r.mbPerS, r.opsPerS, bar)
		}
	}
	fmt.Println("+" + sep + "+")

	// Write ranking by MB/s
	fmt.Println()
	fmt.Println("Write Throughput Ranking (MB/s):")
	fmt.Println(strings.Repeat("-", 70))
	writeResults := []benchResult{}
	for _, r := range allResults {
		if strings.Contains(strings.ToLower(r.name), "write") {
			writeResults = append(writeResults, r)
		}
	}
	sort.Slice(writeResults, func(i, j int) bool {
		return writeResults[i].mbPerS > writeResults[j].mbPerS
	})
	for i, r := range writeResults {
		fmt.Printf("  #%d  %-38s %10.1f MB/s  %10.0f ops/s\n", i+1, r.name, r.mbPerS, r.opsPerS)
	}

	// Read ranking by MB/s
	fmt.Println()
	fmt.Println("Read Throughput Ranking (MB/s):")
	fmt.Println(strings.Repeat("-", 70))
	readResults := []benchResult{}
	for _, r := range allResults {
		if strings.Contains(strings.ToLower(r.name), "read") {
			readResults = append(readResults, r)
		}
	}
	sort.Slice(readResults, func(i, j int) bool {
		return readResults[i].mbPerS > readResults[j].mbPerS
	})
	for i, r := range readResults {
		fmt.Printf("  #%d  %-38s %10.1f MB/s  %10.0f ops/s\n", i+1, r.name, r.mbPerS, r.opsPerS)
	}
}

// ─── main ──────────────────────────────────────────────

func main() {
	flag.Parse()

	fmt.Println("+============================================================+")
	fmt.Println("|       HooDB vs LevelDB  Benchmark Tool                     |")
	fmt.Println("+============================================================+")
	fmt.Printf("|  Ops: %-10d  Concurrency: %-6d  Value: %-6dB          |\n", *numOps, *concurrency, *valueSize)
	fmt.Println("+============================================================+")
	fmt.Println()

	// ===== HooDB API Tests =====
	if !*skipHooDB {
		if err := checkHooDB(*hoodbURL); err != nil {
			fmt.Printf("WARNING: HooDB unreachable: %v\n", err)
			fmt.Println("   Ensure cluster is running, or use -skip-hoodb to skip")
			fmt.Println()
			*skipHooDB = true
		}
	}

	if !*skipHooDB {
		fmt.Println("--- HooDB API Write Tests ---")
		benchHooDBWriteConcurrent(*hoodbURL, *numOps, *concurrency, *valueSize)
		benchHooDBWriteBatch(*hoodbURL, *numOps, *batchSize, *valueSize)
		benchHooDBWriteBatchConcurrent(*hoodbURL, *numOps, *batchSize, *concurrency, *valueSize)
		benchHooDBServerSide(*hoodbURL, *numOps, *concurrency)
		fmt.Println()

		fmt.Println("--- HooDB API Read Tests ---")
		benchHooDBReadConcurrent(*hoodbURL, *numOps, *concurrency)
		fmt.Println()
	}

	// ===== LevelDB Local Tests =====
	fmt.Println("--- LevelDB Local Write Tests ---")
	benchLevelDBWriteSync(*numOps, *valueSize)
	benchLevelDBWriteNoSync(*numOps, *valueSize)
	benchLevelDBWriteBatch(*numOps, *batchSize, *valueSize)
	benchLevelDBWriteConcurrent(*numOps, *concurrency, *valueSize)
	fmt.Println()

	fmt.Println("--- LevelDB Local Read Tests ---")
	benchLevelDBRead(*numOps, *valueSize)
	benchLevelDBReadConcurrent(*numOps, *concurrency, *valueSize)
	fmt.Println()

	// ===== Summary =====
	printReport()
}
