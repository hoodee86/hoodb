# HooDB 测试指南

本文档介绍如何运行 HooDB 的各项测试。

## 测试环境准备

确保已安装 Go 1.23.0+ 和必要的依赖：

```bash
go mod tidy
```

## 1. 单元测试

### 存储层测试

测试 Pebble 存储引擎的基本功能：

```bash
go test ./internal/storage -v
```

测试内容：
- 基础 Put/Get/Delete 操作
- 并发写入 (273 ops/sec)
- 并发读取 (2.6M ops/sec)
- 批量操作 (202K ops/sec)
- 迭代器功能

### 基准测试

运行性能基准测试：

```bash
go test ./internal/storage -bench=. -benchmem
```

## 2. 存储引擎性能对比

对比 Pebble 和 LevelDB 的性能：

```bash
# 运行性能对比测试
./run_benchmark.sh

# 或者手动运行
cd benchmark
go build -o storage_bench storage_bench.go
./storage_bench
```

测试项目：
- 顺序写入 (10,000 次)
- 随机读取 (10,000 次)
- 批量写入 (10,000 次)

## 3. 分布式集群测试

### 3.1 启动集群

启动 3 节点集群：

```bash
./start_cluster.sh
```

集群节点：
- Node1: http://localhost:8001 (通常是 Leader)
- Node2: http://localhost:8002
- Node3: http://localhost:8003

查看集群状态：

```bash
curl http://localhost:8001/status
```

### 3.2 压力测试

测试集群的性能和稳定性：

```bash
./stress_test.sh
```

测试包括：
1. **顺序写入**: 1000 条记录
2. **并发写入**: 10 个并发 × 100 条
3. **并发读取**: 20 个并发 × 100 次
4. **混合读写**: 70% 读 + 30% 写
5. **大值写入**: 100 个 10KB 值

### 3.3 高可用性测试

测试集群的容错能力：

```bash
./ha_test.sh
```

测试场景：
1. **基础 HA 测试**: 验证数据一致性
2. **Follower 故障**: 停止并重启 Follower 节点
3. **Leader 故障**: 停止 Leader，验证新选举
4. **多节点故障**: 停止 2/3 节点，测试多数派要求
5. **负载下故障**: 持续写入时停止 Leader

**⚠️ 注意**: 
- HA 测试会多次启停节点，请确保集群正常运行
- 测试过程中可能会看到错误日志，这是正常的
- 测试完成后集群可能处于不完整状态，需要重启

### 3.4 停止集群

测试完成后停止集群：

```bash
./stop_cluster.sh
```

## 4. 手动测试

### 基本 API 测试

```bash
# 确保集群运行
./start_cluster.sh

# 写入数据
curl -X PUT http://localhost:8001/kv/test1 \
  -H "Content-Type: application/json" \
  -d '{"value":"hello"}'

# 读取数据
curl http://localhost:8001/kv/test1

# 从其他节点读取 (验证同步)
curl http://localhost:8002/kv/test1
curl http://localhost:8003/kv/test1

# 删除数据
curl -X DELETE http://localhost:8001/kv/test1

# 查看集群状态
curl http://localhost:8001/status
curl http://localhost:8002/status
curl http://localhost:8003/status
```

### Leader 选举测试

```bash
# 1. 查看当前 Leader
curl http://localhost:8001/status | jq .isLeader
curl http://localhost:8002/status | jq .isLeader
curl http://localhost:8003/status | jq .isLeader

# 2. 找到 Leader 的 PID
cat pids.txt

# 3. 停止 Leader 进程
kill <LEADER_PID>

# 4. 等待 5-10 秒

# 5. 查看新 Leader
curl http://localhost:8002/status | jq .isLeader
curl http://localhost:8003/status | jq .isLeader

# 6. 在新 Leader 上写入数据
curl -X PUT http://localhost:8002/kv/test2 \
  -H "Content-Type: application/json" \
  -d '{"value":"after election"}'
```

## 5. 测试脚本说明

### stress_test.sh

压力测试脚本，包含5个测试场景。会输出：
- 每个测试的耗时
- 吞吐量 (ops/sec)
- 最终集群状态

### ha_test.sh

高可用性测试脚本，会自动：
- 启动和停止节点
- 等待 Leader 选举
- 验证数据一致性
- 输出测试结果

### run_benchmark.sh

存储引擎性能对比脚本，会：
- 编译测试程序
- 运行 Pebble 和 LevelDB 测试
- 输出性能对比结果

## 6. 日志查看

查看节点日志：

```bash
# 实时查看
tail -f logs/node1.log
tail -f logs/node2.log
tail -f logs/node3.log

# 查看最近的错误
grep ERROR logs/*.log
grep error logs/*.log
```

## 7. 清理测试数据

```bash
# 停止集群
./stop_cluster.sh

# 清理数据目录
rm -rf data/node*

# 清理日志
rm -rf logs/*.log

# 清理 PID 文件
rm -f pids.txt

# 清理测试生成的数据库
rm -rf benchmark/bench_*
```

## 8. 故障排查

### 集群启动失败

```bash
# 检查端口占用
lsof -i :8001
lsof -i :8002
lsof -i :8003
lsof -i :9001
lsof -i :9002
lsof -i :9003

# 清理旧进程
./stop_cluster.sh
pkill -f hoodb

# 清理数据重新启动
rm -rf data/node* logs/*.log
./start_cluster.sh
```

### 测试脚本失败

```bash
# 确保脚本有执行权限
chmod +x *.sh

# 检查集群状态
curl http://localhost:8001/health
curl http://localhost:8001/status
```

### 节点无法同步

```bash
# 查看节点日志
tail -100 logs/node1.log
tail -100 logs/node2.log
tail -100 logs/node3.log

# 检查 Raft 状态
curl http://localhost:8001/status | jq .raftStats
```

## 9. 性能调优建议

### 提高写入性能

1. **批量写入**: 使用批量 API 而不是单条写入
2. **并发度**: 增加客户端并发数
3. **Raft 配置**: 调整 HeartbeatTimeout 和 ElectionTimeout

### 提高读取性能

1. **读负载均衡**: 从 Follower 节点读取 (如果可以接受最终一致性)
2. **缓存**: 在客户端添加缓存层
3. **批量读取**: 一次读取多个 key

### 提高可用性

1. **增加节点数**: 5 节点集群可以容忍 2 节点故障
2. **跨机架部署**: 避免单点故障
3. **监控**: 添加健康检查和告警

## 10. 测试结果参考

详细的测试结果和分析请参考 [TEST_REPORT.md](TEST_REPORT.md)。

主要性能指标：
- 顺序写入: ~28 ops/sec
- 并发写入: ~51 ops/sec (10 并发)
- 并发读取: ~635 ops/sec (20 并发)
- Leader 故障恢复: < 5 秒
- 数据一致性: 100% (正常运行)
