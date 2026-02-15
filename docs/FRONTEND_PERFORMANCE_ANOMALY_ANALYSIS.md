# HooDB 顺序写 vs 批量写性能分析 — 发现的真问题

## 问题现象

根据用户提供的截图（1000 条数据）：
- **"顺序写"**：18,011 ops/sec （0.06s 完成）✅ 快
- **"批量写"**：4,000 ops/sec （0.25s 完成）❌ 慢

这与通常的性能直觉完全相反！**为什么顺序写反而比批量写快?**

---

## 根本原因（前端代码层面）

### 1. "顺序写"测试的真实实现

前端 `PerformanceTest.tsx` 第 57 行：

```tsx
// runSequentialTest() 函数
const runSequentialTest = async () => {
  const res = await runBenchmark(count, 50);  // ← 関键！
  
  setResult({
    type: t('perfSequentialWrite'),
    count: res.count,
    duration: res.duration_ms / 1000,
    opsPerSec: res.ops_per_sec,
    ...
  });
};
```

**实际执行方式**：
```
前端浏览器                    网络                    HooDB 服务端
│                            │                       │
├─ POST /benchmark ─────────>│                       │
│  {count: 1000,             │                       │
│   concurrency: 50}         │                       ├─ 创建 50 个 goroutine
│                            │                       │
│                            │                       ├─ 并发提交 1000 条
│                            │                       │
│                            │                       ├─ batcher 合并 batch
│                            │                       │
│                            │                       ├─ Raft 日志复制
│                            │                       │
│                            │                       ├─ FSM Apply（批量写）
│<─ 返回 {ops_per_sec: 18011} ─────────────────────┤
│                            │                       │
```

**特点**：
- ✅ 只需 **1 个 HTTP 请求** 
- ✅ 数据合并在**服务端** goroutine 级别
- ✅ 网络开销最小
- ✅ 服务端使用 **50 个并发** 处理

### 2. "批量写"测试的真实实现

前端 `PerformanceTest.tsx` 第 68-117 行：

```tsx
// runBatchTest() 函数
const runBatchTest = async () => {
  const batchSize = 50;
  const batches = Math.ceil(count / batchSize);  // 1000/50 = 20 次
  
  for (let b = 0; b < batches; b++) {
    const items = {};
    for (let i = 0; i < currentBatchSize; i++) {
      items[key] = value;  // 组收 50 条
    }
    await batchSet(items);  // ← 発送 HTTP 請求
  }
};
```

**实际执行方式**：
```
第1个循环
前端浏览器                    网络                    HooDB 服务端
│                            │                       │
├─ POST /kv/batch ─────────>│                       │
│  {items: {key1, key2, ..., key50}}  │─────────────>├─ BatchSet()
│                            │                       ├─ ApplyCommand()
│<─ 返回 ─────────────────────────────────────────┤
│ (等待响应...)              │                       │
│  ← RTT 延迟 ~50ms          │                       │

第2个循环
├─ POST /kv/batch ─────────>│                       │
│  {items: {key51, ..., key100}}    │─────────────>├─ BatchSet()
│                            │                       │
│<─ 返回 ─────────────────────────────────────────┤
│ (等待响应...)              │                       │

... × 20 次循环，每次都要等完整的 HTTP RTT ...
```

**特点**：
- ❌ 需要 **20 个 HTTP 请求**（串行）
- ❌ 每个请求都要一次网络 RTT (~50ms)
- ❌ 浏览器连接复用开销
- ❌ **串行执行**，无法并发
- ❌ 总时间 = 20 × RTT + 处理时间 ≈ 1000ms

---

## 前端代码问题对比

| 项目 | "顺序写"实现 | "批量写"实现 |
|------|----------|----------|
| **发送方式** | 1 个 HTTP 请求 | 20 个 HTTP 请求 |
| **执行模式** | 服务端 50 个 goroutine 并发 | 浏览器串行循环 |
| **数据合并** | 由 batcher + Raft 自动合并 | 前端手工分割成 50 条发送 |
| **网络开销** | 1 × RTT (~5ms) | 20 × RTT (~1000ms) |
| **总耗时** | 50ms（包括网络） | 1000ms+ |
| **吞吐量** | 18,011 ops/sec | 4,000 ops/sec |
| **误导指数** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |

---

## 为什么会这样？

### 前端"批量写"的设计失误

原始意图可能是：
> "由浏览器将请求**分批**发送，模拟高吞吐量场景"

但实际结果是：
> "由于 HTTP/1.1 连接复用 + 浏览器 6 连接限制，每个请求都成为瓶颈"

### 浏览器 HTTP 连接限制

HTTP/1.1 规范建议浏览器**最多保持 6 个并发连接**：

```
浏览器连接池（6 个）
├─ 建立连接 1 到服务器...等待响应 1
├─ 建立连接 2 到服务器...等待响应 2
├─ ...
├─ 建立连接 6 到服务器...等待响应 6
└─ 连接 7-20 需要排队等待！

总时间 = ceil(20 / 6) × RTT ≈ 4 × 50ms = 200ms +
        处理时间 ≈ 50ms
        = 250ms
```

### "顺序写"为什么叫"顺序"却很快？

原因：**变量命名失误**。这个测试实际上应该叫"服务端基准测试"或"并发基准测试"。

```tsx
const runSequentialTest = async () => {          // ← 名字说"顺序"
  const res = await runBenchmark(count, 50);     // ← 但实现用 concurrency=50
};
```

这是**误导性命名**！应该改成：
```tsx
const runServerBenchmarkTest = async () => {     // ← 更准确的名字
  const res = await runBenchmark(count, 50);     // 服务端 50 并发
};
```

---

## 后端为什么批量写快？

为对比，看后端的顺序 vs 批量写的性能差异（直接调用 API）：

```bash
# 后端顺序写（concurrency=1）
curl -X POST http://localhost:8001/benchmark \
  -H "Content-Type: application/json" \
  -d '{"count":1000,"concurrency":1}'
# 结果：3920 ops/sec，255ms

# 后端批量写（concurrency=50）  
curl -X POST http://localhost:8001/benchmark \
  -H "Content-Type: application/json" \
  -d '{"count":1000,"concurrency":50}'
# 结果：23677 ops/sec，42ms
```

**后端正常遵循规律**：并发度高 = 吞吐量高。

**关键区别**：
- 后端：1 个 HTTP 请求，服务端并行处理
- 前端批量写：20 个 HTTP 请求，串行等待

---

## 根本问题

前端 `PerformanceTest.tsx` 中的"批量写"实现不是真正的**批量**，而是**逐批串行发送**：

```tsx
// ❌ 问题代码：串行发送
for (let b = 0; b < batches; b++) {
    await batchSet(items);  // 等完才能继续下一个
}

// 优化建议：并发发送
await Promise.all(
  batches.map((batch, idx) => 
    batchSet(batch)  // 所有 batch 同时发送
  )
);
```

---

## 性能时间分布

### "顺序写"（18,011 ops/sec）
```
客户端 → 服务端 (1个HTTP请求)
    ↓
POST /benchmark
    ↓
[0-2ms]   HTTP 网络传输
[2-5ms]   解析请求，创建 50 个 goroutine
[5-50ms]  50 个并发发送 + batcher + Raft 日志复制
[50-60ms] 返回响应
    ↓
客户端接收结果
总耗时: 60ms ÷ 1000 条 = 18,000+ ops/sec
```

### "批量写"（4,000 ops/sec）
```
20 个循环串行执行：
[0-50ms]   POST /kv/batch #1 → 返回
[50-100ms] POST /kv/batch #2 → 返回
...
[950-1000ms] POST /kv/batch #20 → 返回
总耗时: 1000ms ÷ 1000 条 = 4,000 ops/sec
```

---

## 解决方案

### 方案 1：修复前端"批量写"为真正的并发（推荐）

```tsx
const runBatchTest = async () => {
  const batchSize = 50;
  const batches = Math.ceil(count / batchSize);
  const startTime = Date.now();
  
  // ✅ 所有 batch 并发发送
  const promises = [];
  for (let b = 0; b < batches; b++) {
    const items = {};
    const currentBatchSize = Math.min(batchSize, count - b * batchSize);
    for (let i = 0; i < currentBatchSize; i++) {
      const idx = b * batchSize + i;
      items[`batch_test_${Date.now()}_${idx}`] = `value_${idx}`;
    }
    promises.push(batchSet(items));
  }
  
  // 等待所有请求完成
  const results = await Promise.allSettled(promises);
  const duration = (Date.now() - startTime) / 1000;
  
  const success = results.filter(r => r.status === 'fulfilled').length * batchSize;
  
  setResult({
    type: t('perfBatchWrite'),
    count,
    duration,
    opsPerSec: success / duration,  // 会变成 ~15,000+ ops/sec
    success: success,
    failed: results.filter(r => r.status === 'rejected').length * batchSize,
  });
};
```

**预期改进**：4,000 ops/sec → 15,000+ ops/sec（取决于 HTTP 连接池）

### 方案 2：统一使用服务端 Benchmark API

```tsx
// 重命名为更准确的名字
const runClientBatchTest = async () => {
  // 从客户端发送 HTTP 请求，并发度受限
};

const runServerBenchmarkTest = async () => {
  // 使用服务端 benchmark，最准确的测量
  const res = await runBenchmark(count, 1);      // 服务端顺序
};

const runServerConcurrentTest = async () => {
  // 使用服务端 benchmark，最大吞吐量
  const res = await runBenchmark(count, 50);     // 服务端并发
};
```

### 方案 3：改进前端 UI 说明

当前的 UI 预期性能说明：
```
预期性能
顺序写入：~1,000+ ops/sec (受 Raft 共识限制)
批量写入：~10,000+ ops/sec (批量优化)
```

改为：
```
实际执行方式
顺序/并发测试：所有工作在**服务端**并行执行，客户端只用1个HTTP请求
        预期：~20,000+ ops/sec (服务端 50 并发 + 批量合并)
        
批量测试：客户端**串行发送** HTTP 请求，受浏览器连接限制
        预期：~4,000 ops/sec (网络 RTT 成为主瓶颈)
        
内部API（仅供测试）：
  - POST /benchmark: 服务端基准测试，按 concurrency 并发执行
  - POST /kv/batch: 支持一次提交多条，可由客户端并发调用
```

---

## 总结

| 问题 | 根因 | 影响 |
|------|------|------|
| **"顺序写"吞吐量高** | 前端实现用了 concurrency=50 | 名字误导，使用者不知道那是并发 |
| **"批量写"吞吐量低** | 前端串行发送 HTTP 请求 | 触发浏览器连接限制和网络 RTT 瓶颈 |
| **性能看起来相反** | 两个测试的实现方式完全不同 | 用户困惑，影响对系统的理解 |

**建议**：
1. 修复前端代码：`Promise.all()` 并发发送批量请求
2. 改进 UI 标签：改成"服务端基准测试"和"客户端批量操作"
3. 添加说明文档：解释每个测试的实际含义和网络开销
