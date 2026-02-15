# HooDB - 分布式 Key-Value 数据库

基于 **Pebble** (LSM 存储引擎) 和 **Raft** 共识算法实现的分布式 Key-Value 数据库系统。

## 特性

- 使用 Pebble 作为底层存储引擎（纯 Go 实现，兼容 RocksDB API）
- 基于 HashiCorp Raft 实现分布式一致性
- 支持多节点集群部署与动态扩缩容
- 自动快照、数据同步与故障恢复
- 写入批处理器（流水线架构）实现高吞吐
- RESTful HTTP API（带 `/api/v1` 版本化路由）
- React + Material UI 管理前端（中英双语）

## 架构

```
┌─────────────────────────────────────┐
│       React Frontend (MUI)          │
│   KV 操作 / 集群监控 / 性能测试       │
└──────────────┬──────────────────────┘
               │ HTTP
┌──────────────▼──────────────────────┐
│      HTTP API Layer (Gin)           │
│   /api/v1/kv  /cluster  /benchmark  │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│    KV Service + Write Batcher       │
│  批量合并 → 单次 Raft Apply           │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│       Raft Consensus Layer          │
│  Leader 选举 / 日志复制 / 快照        │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│     Pebble Storage Engine           │
│   LSM-Tree / Bloom Filter / Snappy  │
└─────────────────────────────────────┘
```

## 项目结构

```
hoodb/
├── cmd/hoodb/main.go           # 程序入口
├── api/http/handler.go         # HTTP API 路由与处理器
├── internal/
│   ├── config/config.go        # 配置加载与校验
│   ├── raft/raft.go            # Raft 共识层封装
│   ├── service/
│   │   ├── kvservice.go        # KV 服务核心 + FSM 实现
│   │   └── batcher.go          # 写入批处理器（流水线架构）
│   └── storage/pebble.go       # Pebble 存储引擎封装
├── client/                     # React 前端
│   ├── src/components/         # UI 组件
│   ├── src/services/api.ts     # API 客户端
│   └── src/i18n/               # 国际化 (EN/ZH)
├── configs/                    # 节点配置
│   ├── node1.json ... node5.json
├── scripts/                    # 运维脚本
│   ├── start_cluster.sh        # 启动集群
│   ├── stop_cluster.sh         # 停止集群
│   ├── scale_to_5nodes.sh      # 扩容至 5 节点
│   └── ...                     # 测试/基准脚本
├── benchmark/                  # 存储引擎性能对比
├── docs/                       # 项目文档
├── Makefile                    # 构建/测试/运行
└── go.mod
```

## 快速开始

### 前置要求

- Go 1.21+
- Node.js 18+ (前端可选)

### 编译

```bash
make build        # 编译后端
make build-web    # 编译前端 (可选)
```

### 启动集群

```bash
make run          # 编译 + 启动 3 节点集群
make stop         # 停止集群
```

或手动启动单个节点：

```bash
./bin/hoodb -config configs/node1.json
```

### 运行测试

```bash
make test         # 所有单元测试
make bench        # 性能基准测试
```

## API

所有业务接口支持 `/api/v1` 前缀（推荐）和无前缀的兼容路由。

### KV 操作

```bash
# 写入
curl -X PUT http://localhost:8001/api/v1/kv/mykey \
  -H "Content-Type: application/json" \
  -d '{"value":"myvalue"}'

# 读取
curl http://localhost:8001/api/v1/kv/mykey

# 删除
curl -X DELETE http://localhost:8001/api/v1/kv/mykey

# 批量写入
curl -X POST http://localhost:8001/api/v1/kv/batch \
  -H "Content-Type: application/json" \
  -d '{"items":{"k1":"v1","k2":"v2"}}'
```

### 集群管理

```bash
# 集群状态
curl http://localhost:8001/api/v1/cluster/stats

# 集群配置
curl http://localhost:8001/api/v1/cluster/config

# 动态添加节点
curl -X POST http://localhost:8001/api/v1/cluster/add \
  -d '{"node_id":"node4","address":"127.0.0.1:9004"}'

# 移除节点
curl -X POST http://localhost:8001/api/v1/cluster/remove \
  -d '{"node_id":"node4"}'
```

### 状态 & 健康

```bash
curl http://localhost:8001/health    # 健康检查（含集群状态）
curl http://localhost:8001/status    # 详细 Raft 指标
```

### 服务端基准测试

```bash
curl -X POST http://localhost:8001/api/v1/benchmark \
  -d '{"count":10000,"concurrency":50}'
```

## 配置文件

每个节点的配置在 `configs/node{n}.json`：

```json
{
  "nodeId": "node1",
  "httpAddr": ":8001",
  "raftAddr": "127.0.0.1:9001",
  "dataDir": "./data/node1",
  "peers": [
    "127.0.0.1:9001",
    "127.0.0.1:9002",
    "127.0.0.1:9003"
  ]
}
```

> `peers` 为空时，节点将作为新成员动态加入已有集群。

## 技术细节

### Raft 共识

- BatchApplyCh + BoltDB NoSync 消除 fsync 瓶颈
- 快照阈值 256 条日志，间隔 30 秒
- 写入批处理器：4 并行执行器，最大 200 条/批

### 存储引擎

- 512MB 块缓存 + 128MB MemTable
- Bloom Filter (10 bits) 加速读取
- 7 层 LSM，Snappy 压缩
- 4 并发 Compaction 线程

## 文档

- [集群扩容指南](docs/SCALING_QUICKSTART.md)
- [扩容实现详解](docs/SCALING_IMPLEMENTATION.md)
- [性能分析：顺序写 vs 批量写](docs/PERFORMANCE_ANALYSIS_SEQUENTIAL_VS_BATCH.md)
- [前端性能异常分析](docs/FRONTEND_PERFORMANCE_ANOMALY_ANALYSIS.md)

## 许可证

MIT License
