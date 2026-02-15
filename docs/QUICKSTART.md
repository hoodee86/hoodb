# HooDB 快速使用指南

## 编译项目

已编译完成！可执行文件位于 `./bin/hoodb`

如需重新编译：
```bash
go build -o ./bin/hoodb ./cmd/hoodb
```

## 启动集群

### 方式一：使用启动脚本（推荐）

```bash
# 启动完整的3节点集群
./start_cluster.sh

# 停止集群
./stop_cluster.sh
```

### 方式二：手动启动单个节点

```bash
# 启动节点1
./start_node.sh 1

# 在另一个终端启动节点2
./start_node.sh 2

# 在另一个终端启动节点3
./start_node.sh 3
```

### 方式三：直接使用二进制文件

```bash
# 启动节点1
./bin/hoodb -config ./configs/node1.json

# 启动节点2
./bin/hoodb -config ./configs/node2.json

# 启动节点3
./bin/hoodb -config ./configs/node3.json
```

## 测试API

启动集群后，运行测试脚本：

```bash
./test_api.sh
```

或手动测试：

```bash
# 查看集群状态
curl http://localhost:8001/status | python3 -m json.tool

# 设置键值对（需要在Leader节点操作）
curl -X PUT http://localhost:8001/kv/mykey \
  -H "Content-Type: application/json" \
  -d '{"value":"myvalue"}'

# 获取键值（可以在任何节点操作）
curl http://localhost:8001/kv/mykey

# 从其他节点读取（验证数据同步）
curl http://localhost:8002/kv/mykey
curl http://localhost:8003/kv/mykey

# 删除键值
curl -X DELETE http://localhost:8001/kv/mykey
```

## 节点端口配置

- Node1: HTTP 8001, Raft 9001
- Node2: HTTP 8002, Raft 9002  
- Node3: HTTP 8003, Raft 9003

## 数据目录

每个节点的数据存储在：
- `./data/node1/` - 节点1数据
- `./data/node2/` - 节点2数据
- `./data/node3/` - 节点3数据

## 日志文件

集群日志位于：
- `./logs/node1.log`
- `./logs/node2.log`
- `./logs/node3.log`

## 常见问题

### 端口被占用

如果提示端口被占用，可以：
1. 停止占用端口的进程
2. 或修改 `configs/nodeX.json` 中的端口配置

### Leader选举失败

- 确保至少有2个节点在运行（Raft需要多数节点同意）
- 检查日志文件查看详细错误信息
- 第一次启动时，节点1会自动成为Leader

### 清理数据

```bash
# 停止所有节点
./stop_cluster.sh

# 删除数据目录
rm -rf ./data

# 重新启动
./start_cluster.sh
```

## API响应示例

### 成功的SET操作
```json
{
  "message": "success",
  "key": "mykey",
  "value": "myvalue"
}
```

### 成功的GET操作
```json
{
  "key": "mykey",
  "value": "myvalue"
}
```

### 在非Leader节点执行写操作
```json
{
  "error": "not leader",
  "leader": "127.0.0.1:9001"
}
```

### 节点状态信息
```json
{
  "isLeader": true,
  "leader": "127.0.0.1:9001",
  "state": "Leader",
  "lastIndex": 10,
  "appliedIndex": 10,
  "raftStats": {
    "applied_index": "10",
    "commit_index": "10",
    "last_log_index": "10",
    "last_log_term": "2",
    "state": "Leader"
  }
}
```

## 性能调优

可以通过修改 `internal/storage/pebble.go` 中的配置来调整性能：
- `Cache`: 缓存大小
- `MemTableSize`: 写缓冲大小
- `MaxConcurrentCompactions`: 并发压缩数量

## 监控

通过状态API可以监控节点状态：
```bash
watch -n 1 'curl -s http://localhost:8001/status | python3 -m json.tool'
```
