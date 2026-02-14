# HooDB - 分布式Key-Value数据库

基于 Pebble 和 Raft 算法实现的分布式Key-Value数据库系统。

## 特性

- ✅ 使用 Pebble 作为底层存储引擎（纯Go实现，兼容RocksDB）
- ✅ 基于 Raft 算法实现分布式一致性
- ✅ 支持多节点集群部署
- ✅ 自动数据同步
- ✅ HTTP API 接口
- ✅ 高可用性和容错能力

## 架构

```
┌─────────────────────────────────────┐
│         HTTP API Layer              │
│   (PUT/GET/DELETE Key-Value)        │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│      Distributed KV Service         │
│    (处理客户端请求，转发到Raft)       │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│         Raft Consensus              │
│  (Leader选举，日志复制，一致性)      │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│      Pebble Storage Engine          │
│      (持久化存储Key-Value)            │
└─────────────────────────────────────┘
```

## 快速开始

### 前置要求

1. Go 1.21 或更高版本

**注意：** HooDB 使用 Pebble 作为存储引擎，它是纯 Go 实现的，不需要安装额外的 C 库。

### 安装依赖

```bash
go mod download
```

### 启动集群

启动3个节点的集群：

```bash
# 启动节点1 (Leader候选)
./start_node.sh 1

# 启动节点2
./start_node.sh 2

# 启动节点3
./start_node.sh 3
```

或者使用提供的脚本一次性启动所有节点：

```bash
./start_cluster.sh
```

## API 使用

### 设置 Key-Value

```bash
curl -X PUT http://localhost:8001/kv/mykey \
  -H "Content-Type: application/json" \
  -d '{"value":"myvalue"}'
```

### 获取 Value

```bash
curl http://localhost:8001/kv/mykey
```

### 删除 Key

```bash
curl -X DELETE http://localhost:8001/kv/mykey
```

### 查看节点状态

```bash
curl http://localhost:8001/status
```

## 配置文件

每个节点的配置在 `configs/node{n}.json` 中：

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

## 项目结构

```
hoodb/
├── cmd/
│   └── hoodb/
│       └── main.go           # 主程序入口
├── internal/
│   ├── storage/
│   │   └── rocksdb.go        # RocksDB存储层
│   ├── raft/
│   │   └── raft.go           # Raft共识层
│   └── service/
│       └── kvservice.go      # KV服务核心
├── api/
│   └── http/
│       └── handler.go        # HTTP API处理
├── configs/                  # 配置文件目录
│   ├── nodepebble.go         # Pebble
│   ├── node2.json
│   └── node3.json
├── scripts/                  # 启动脚本
├── go.mod
└── README.md
```

## 开发说明

### Raft 共识机制

- 使用 HashiCorp Raft 库实现
- 自动进行 Leader 选举
- 日志复制确保数据一致性
- 支持动态成员变更

### 存储引擎

- Pebble 提供高性能持久化存储（纯Go实现）
- 支持快照和恢复
- LSM树结构优化写入性能
- 兼容RocksDB API

## 测试

```bash
go test ./...
```

## 许可证

MIT License
