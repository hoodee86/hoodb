# HooDB 集群扩容指南

## 概述

本文档说明如何将 HooDB 集群从 3 节点平滑扩容至 5 节点，整个过程**无需停机**，对业务无影响。

## 前置条件

- ✅ 现有 3 节点集群运行正常
- ✅ Leader 节点可访问
- ✅ 新节点的配置文件已准备
- ✅ 网络连通性已确认

## 技术原理

### Raft 动态成员变更

HooDB 采用 Raft 共识算法的 **单步成员变更** (Single-Server Configuration Change) 机制：

```
1. Leader 收到 AddVoter 请求
2. Leader 将新配置作为特殊日志条目提交
3. 新配置复制到多数节点后生效
4. 新节点自动从 Leader 同步数据
```

**安全性保证**：
- 任何时刻最多一个 Leader
- 新旧配置交替期间仍满足多数派
- 配置变更失败会自动回滚

### 容错能力变化

| 集群规模 | 法定人数 (Quorum) | 允许故障数 | 可用性 |
|---------|-----------------|----------|--------|
| **3 节点** | 2 | 1 | 99.9% |
| **5 节点** | 3 | 2 | 99.99% |
| **7 节点** | 4 | 3 | 99.999% |

**推荐配置**：
- 生产环境：5 节点（平衡性能和可靠性）
- 关键业务：7 节点（最高可用性）
- 开发/测试：3 节点（节省资源）

## 扩容步骤

### 第 1 步：准备新节点配置

已为 node4 和 node5 创建配置文件：

```bash
# configs/node4.json
{
  "nodeId": "node4",
  "httpAddr": ":8004",
  "raftAddr": "127.0.0.1:9004",
  "dataDir": "./data/node4",
  "peers": []  # 新节点 peers 为空
}

# configs/node5.json
{
  "nodeId": "node5",
  "httpAddr": ":8005",
  "raftAddr": "127.0.0.1:9005",
  "dataDir": "./data/node5",
  "peers": []
}
```

**关键点**：
- ✅ `nodeId` 必须唯一
- ✅ `httpAddr` 和 `raftAddr` 端口不冲突
- ✅ `peers` 设为空数组（新节点不自行 Bootstrap）

### 第 2 步：查看当前集群状态

```bash
# 查询 Leader 节点
curl http://localhost:8001/status | jq '.isLeader'

# 查看集群配置
curl http://localhost:8001/cluster/config | jq
```

**预期输出**：
```json
{
  "servers": [
    {
      "id": "node1",
      "address": "127.0.0.1:9001",
      "suffrage": "Voter"
    },
    {
      "id": "node2",
      "address": "127.0.0.1:9002",
      "suffrage": "Voter"
    },
    {
      "id": "node3",
      "address": "127.0.0.1:9003",
      "suffrage": "Voter"
    }
  ],
  "leader": "127.0.0.1:9001"
}
```

### 第 3 步：启动新节点（但不加入集群）

**重要**：修改启动逻辑，新节点不执行 Bootstrap

```bash
# 启动 node4
./hoodb -config configs/node4.json > /tmp/node4.log 2>&1 &

# 启动 node5
./hoodb -config configs/node5.json > /tmp/node5.log 2>&1 &

# 检查进程
ps aux | grep hoodb
```

**此时状态**：
- node4 和 node5 已启动，但处于 **Follower** 状态
- 尚未加入 Raft 集群，无法参与投票
- 等待 Leader 将其添加到配置中

### 第 4 步：添加 node4 到集群

```bash
# 向 Leader 发送添加节点请求
curl -X POST http://localhost:8001/cluster/add \
  -H "Content-Type: application/json" \
  -d '{
    "node_id": "node4",
    "address": "127.0.0.1:9004"
  }'
```

**预期响应**：
```json
{
  "message": "node added successfully",
  "node_id": "node4",
  "address": "127.0.0.1:9004"
}
```

**幕后发生的事情**：
1. Leader 向 Raft 提交 `AddVoter` 配置变更
2. 配置变更复制到多数节点（node1, node2, node3 中的 2 个）
3. 新配置生效，node4 成为 **Voter**
4. Leader 自动向 node4 复制所有历史日志
5. node4 完成同步后可正常参与投票

**监控同步进度**：
```bash
# 查看 node4 日志
tail -f /tmp/node4.log

# 检查 node4 的日志索引
curl http://localhost:8004/status | jq '.lastIndex, .appliedIndex'
```

**等待同步完成**（取决于数据量）：
- 小数据量（< 1GB）：通常 < 30 秒
- 中等数据量（1-10GB）：1-5 分钟
- 大量数据（> 10GB）：建议使用快照恢复

### 第 5 步：验证 node4 状态

```bash
# 检查集群配置
curl http://localhost:8001/cluster/config | jq

# 验证 node4 健康状态
curl http://localhost:8004/health

# 检查 node4 是否可读
curl http://localhost:8004/kv/test_key
```

**预期输出**：
```json
{
  "servers": [
    {"id": "node1", "address": "127.0.0.1:9001", "suffrage": "Voter"},
    {"id": "node2", "address": "127.0.0.1:9002", "suffrage": "Voter"},
    {"id": "node3", "address": "127.0.0.1:9003", "suffrage": "Voter"},
    {"id": "node4", "address": "127.0.0.1:9004", "suffrage": "Voter"}  ✅
  ],
  "leader": "127.0.0.1:9001"
}
```

### 第 6 步：添加 node5 到集群

重复第 4-5 步的操作：

```bash
# 添加 node5
curl -X POST http://localhost:8001/cluster/add \
  -H "Content-Type: application/json" \
  -d '{
    "node_id": "node5",
    "address": "127.0.0.1:9005"
  }'

# 等待同步完成
sleep 30

# 验证最终配置
curl http://localhost:8001/cluster/config | jq
```

**最终集群状态**：
```json
{
  "servers": [
    {"id": "node1", "address": "127.0.0.1:9001", "suffrage": "Voter"},
    {"id": "node2", "address": "127.0.0.1:9002", "suffrage": "Voter"},
    {"id": "node3", "address": "127.0.0.1:9003", "suffrage": "Voter"},
    {"id": "node4", "address": "127.0.0.1:9004", "suffrage": "Voter"},  ✅
    {"id": "node5", "address": "127.0.0.1:9005", "suffrage": "Voter"}   ✅
  ],
  "leader": "127.0.0.1:9001"
}
```

### 第 7 步：功能验证

#### 写入测试（Leader）
```bash
curl -X PUT http://localhost:8001/kv/scale_test_1 \
  -H "Content-Type: application/json" \
  -d '{"value": "5_nodes_cluster"}'
```

#### 读取测试（所有节点）
```bash
# 从各个节点读取，验证数据一致性
for port in 8001 8002 8003 8004 8005; do
  echo "Node $port:"
  curl -s http://localhost:$port/kv/scale_test_1 | jq
done
```

**预期结果**：所有节点返回相同的值

#### 性能测试
```bash
# 5 节点集群写入性能
curl -X POST http://localhost:8001/benchmark \
  -H "Content-Type: application/json" \
  -d '{"count": 10000, "concurrency": 50}'
```

**预期性能**：
- 写入吞吐量：~25,000-30,000 ops/sec（略低于 3 节点，因为需要更多确认）
- 读取吞吐量：~1.67x（5 节点 vs 3 节点）

#### 故障容错测试
```bash
# 停止 2 个节点，集群应仍可写入
pkill -f 'hoodb.*node4'
pkill -f 'hoodb.*node5'

# 验证集群仍可用（3 个节点满足 Quorum=3）
curl -X PUT http://localhost:8001/kv/fault_test \
  -d '{"value": "still_works"}'
```

## 注意事项

### ⚠️  一次只添加一个节点

**错误做法**：
```bash
# ❌ 不要同时添加多个节点
curl -X POST http://localhost:8001/cluster/add -d '{"node_id":"node4",...}'
curl -X POST http://localhost:8001/cluster/add -d '{"node_id":"node5",...}'  # 可能失败
```

**正确做法**：
```bash
# ✅ 等待 node4 完全同步后再添加 node5
curl -X POST http://localhost:8001/cluster/add -d '{"node_id":"node4",...}'
sleep 60  # 等待同步
curl -X POST http://localhost:8001/cluster/add -d '{"node_id":"node5",...}'
```

**原因**：Raft 单步成员变更限制，确保任何时刻只有一个配置变更在进行。

### ⚠️  验证数据完整性

```bash
# 添加节点前记录当前日志索引
BEFORE_INDEX=$(curl -s http://localhost:8001/status | jq '.lastIndex')

# 添加节点后，新节点应同步到相同索引
NODE4_INDEX=$(curl -s http://localhost:8004/status | jq '.appliedIndex')

# 验证
if [ "$NODE4_INDEX" -ge "$BEFORE_INDEX" ]; then
  echo "✅ 数据同步完成"
else
  echo "⚠️  数据仍在同步中，请等待"
fi
```

### ⚠️  Leader 变更

添加节点过程中，如果 Leader 宕机：

```bash
# 查询新 Leader
NEW_LEADER=$(curl -s http://localhost:8002/status | jq -r '.leader')

# 向新 Leader 发送请求
curl -X POST http://${NEW_LEADER}/cluster/add -d '...'
```

### ⚠️  Bootstrap 逻辑修改

**当前代码问题**：新节点如果 `nodeId` 是 "node1"，会自动执行 Bootstrap，导致创建新集群。

**解决方案**：修改 [`internal/service/kvservice.go:58-62`](../internal/service/kvservice.go#L58-L62)：

```go
// 修改前（有问题）
if nodeID == "node1" {
    if err := raftNode.Bootstrap(peers); err != nil {
        kv.logger.Printf("Bootstrap warning: %v", err)
    }
}

// 修改后（推荐）
if nodeID == "node1" && len(peers) > 0 {
    // 只有在首次部署（peers 非空）时才 Bootstrap
    if err := raftNode.Bootstrap(peers); err != nil {
        kv.logger.Printf("Bootstrap warning: %v", err)
    }
}
```

## 性能影响分析

### 写入性能

| 集群规模 | 吞吐量 | 相对 3 节点 | 原因 |
|---------|--------|-----------|------|
| 3 节点 | 30,000 ops/sec | 100% | 基准 |
| 5 节点 | ~27,000 ops/sec | ~90% | 需要更多节点确认 (3/5 vs 2/3) |
| 7 节点 | ~24,000 ops/sec | ~80% | 需要 4 个节点确认 |

**降低原因**：
- 更多的网络往返 (RTT)
- 更多的复制开销
- 更长的 commit 等待时间

**缓解方法**：
- 高速网络（10GbE）
- 地理位置集中（降低延迟）
- 批量写入优化

### 读取性能

| 集群规模 | 理论吞吐量 | 相对 3 节点 |
|---------|-----------|-----------|
| 3 节点 | 3x 单机 | 100% |
| 5 节点 | 5x 单机 | **+67%** ✅ |
| 7 节点 | 7x 单机 | **+133%** ✅ |

**提升原因**：读取不经过 Raft，直接访问本地 PebbleDB，节点越多吞吐越高。

### 故障容错

| 集群规模 | 允许故障数 | 脑裂风险 |
|---------|----------|---------|
| 3 节点 | 1 | 网络分区可能导致不可用 |
| 5 节点 | 2 | **更安全** ✅ |
| 7 节点 | 3 | **最安全** ✅ |

### 存储成本

| 集群规模 | 存储倍数 | 适用场景 |
|---------|---------|---------|
| 3 节点 | 3x | 数据量 < 1TB |
| 5 节点 | 5x | 数据量 < 500GB |
| 7 节点 | 7x | 数据量 < 300GB |

**建议**：数据量大时，考虑分片 (Sharding) 而非增加副本数。

## 缩容操作

如果需要从 5 节点缩容回 3 节点：

```bash
# 移除 node5
curl -X POST http://localhost:8001/cluster/remove \
  -H "Content-Type: application/json" \
  -d '{"node_id": "node5"}'

# 等待配置变更生效
sleep 10

# 停止 node5 进程
pkill -f 'hoodb.*node5'

# 清理 node5 数据（可选）
rm -rf ./data/node5

# 重复操作移除 node4
curl -X POST http://localhost:8001/cluster/remove \
  -d '{"node_id": "node4"}'
pkill -f 'hoodb.*node4'
rm -rf ./data/node4
```

**注意**：
- ⚠️  一次只移除一个节点
- ⚠️  移除后仍需满足 Quorum（如 5 节点 → 4 节点 → 3 节点）
- ⚠️  不能移除 Leader，需先等待 Leader 转移

## 故障排查

### 问题 1：添加节点失败 "not leader"

**原因**：向 Follower 发送了请求

**解决**：
```bash
# 查询 Leader
LEADER=$(curl -s http://localhost:8001/status | jq -r '.leader')

# 向 Leader 发送请求
curl -X POST http://${LEADER}/cluster/add -d '...'
```

### 问题 2：新节点日志同步很慢

**原因**：历史数据量大，网络带宽不足

**解决**：
```bash
# 方案 1：使用快照恢复（推荐）
# 在 Leader 上创建快照
curl -X POST http://localhost:8001/cluster/snapshot

# 将快照文件拷贝到新节点
scp ./data/node1/raft/snapshots/* node4:/data/node4/raft/snapshots/

# 方案 2：调整 Raft 参数（raft.go）
config.SnapshotThreshold = 1024  # 降低快照阈值，加快同步
```

### 问题 3：集群分裂（Split Brain）

**症状**：两个节点都认为自己是 Leader

**原因**：网络分区 + 配置冲突

**解决**：
```bash
# 停止所有节点
pkill -f hoodb

# 清理冲突数据（仅 Follower）
rm -rf ./data/node4/raft/*
rm -rf ./data/node5/raft/*

# 保留 Leader 数据
# 保留 ./data/node1/raft/*

# 重新启动并添加节点
```

### 问题 4：添加节点后性能显著下降

**排查**：
```bash
# 检查网络延迟
ping -c 10 192.168.1.4

# 检查磁盘 I/O
iostat -x 1

# 查看 Raft 统计
curl http://localhost:8001/status | jq '.raftStats'
```

**优化**：
- 升级网络（1GbE → 10GbE）
- 使用 NVMe SSD
- 调整 Raft 超时参数

## 最佳实践

### ✅ 推荐做法

1. **逐步扩容**：3 → 4 → 5，每次等待同步完成
2. **监控指标**：实时监控 lastIndex、appliedIndex
3. **备份数据**：扩容前先创建快照
4. **预生产测试**：在测试环境先验证
5. **非高峰操作**：在业务低峰期扩容

### ❌ 避免的做法

1. ❌ 同时添加多个节点
2. ❌ 在 Leader 选举期间扩容
3. ❌ 在高负载时扩容
4. ❌ 跳过数据完整性验证
5. ❌ 忽略网络延迟检查

## 总结

HooDB 支持**零停机**的平滑扩缩容，关键步骤：

```
1. 准备配置文件（peers 为空）
2. 启动新节点（不 Bootstrap）
3. 通过 API 添加到集群（一次一个）
4. 等待数据同步完成
5. 验证集群状态和性能
```

**核心优势**：
- ✅ 业务无感知（不停机）
- ✅ 数据零丢失（Raft 保证）
- ✅ 自动故障恢复
- ✅ 操作简单（一行 curl 命令）

**性能影响**：
- 写入：略有下降（~10%）
- 读取：线性提升（+67%）
- 容错：显著增强（1 → 2 故障容忍）

**适用场景**：
- 🎯 读多写少业务（充分利用读扩展）
- 🎯 高可用要求（关键业务）
- 🎯 数据量适中（< 500GB）

---

**参考资料**：
- [Raft 论文 - 成员变更](https://raft.github.io/raft.pdf) 第 6 节
- [HooDB 架构文档](./PERFORMANCE_ANALYSIS.md)
- [HashiCorp Raft 文档](https://pkg.go.dev/github.com/hashicorp/raft)
