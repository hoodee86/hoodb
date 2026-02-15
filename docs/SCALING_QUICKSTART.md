# 集群扩容快速指南

## 问题：能否将 3 节点平滑扩容至 5 节点？

**答案：✅ 可以！** HooDB 支持零停机的平滑扩容。

## 快速开始

### 自动化扩容（推荐）

```bash
# 一键扩容至 5 节点
./scripts/scale_to_5nodes.sh
```

该脚本会自动完成：
1. 检查现有集群状态
2. 启动 node4 和 node5
3. 将新节点添加到 Raft 集群
4. 等待数据同步
5. 验证功能和性能

### 手动扩容

#### 步骤 1：启动新节点

```bash
# 启动 node4
./hoodb -config configs/node4.json > /tmp/node4.log 2>&1 &

# 启动 node5
./hoodb -config configs/node5.json > /tmp/node5.log 2>&1 &
```

#### 步骤 2：添加到集群

```bash
# 添加 node4
curl -X POST http://localhost:8001/cluster/add \
  -H "Content-Type: application/json" \
  -d '{"node_id": "node4", "address": "127.0.0.1:9004"}'

# 等待同步（30-60秒）
sleep 60

# 添加 node5
curl -X POST http://localhost:8001/cluster/add \
  -H "Content-Type: application/json" \
  -d '{"node_id": "node5", "address": "127.0.0.1:9005"}'
```

#### 步骤 3：验证

```bash
# 查看集群配置
curl http://localhost:8001/cluster/config | python3 -m json.tool

# 测试读写
curl -X PUT http://localhost:8001/kv/test -d '{"value":"works"}'
curl http://localhost:8004/kv/test
curl http://localhost:8005/kv/test
```

## 新增 API

### 查看集群配置

```bash
GET /cluster/config
```

返回：
```json
{
  "servers": [
    {"id": "node1", "address": "127.0.0.1:9001", "suffrage": "Voter"},
    {"id": "node2", "address": "127.0.0.1:9002", "suffrage": "Voter"},
    {"id": "node3", "address": "127.0.0.1:9003", "suffrage": "Voter"},
    {"id": "node4", "address": "127.0.0.1:9004", "suffrage": "Voter"},
    {"id": "node5", "address": "127.0.0.1:9005", "suffrage": "Voter"}
  ],
  "leader": "127.0.0.1:9001"
}
```

### 添加节点

```bash
POST /cluster/add
Content-Type: application/json

{
  "node_id": "node4",
  "address": "127.0.0.1:9004"
}
```

### 移除节点

```bash
POST /cluster/remove
Content-Type: application/json

{
  "node_id": "node5"
}
```

## 性能影响

| 指标 | 3 节点 | 5 节点 | 变化 |
|------|--------|--------|------|
| **写入吞吐量** | 30,000 ops/sec | ~27,000 ops/sec | -10% |
| **读取吞吐量** | 3x 单机 | 5x 单机 | +67% ✅ |
| **故障容忍** | 1 个节点 | 2 个节点 | +100% ✅ |
| **Quorum** | 2/3 | 3/5 | 更严格 |

## 注意事项

⚠️ **一次只添加一个节点**（Raft 限制）

⚠️ **等待数据同步**（30-60秒，取决于数据量）

⚠️ **只能向 Leader 发送添加/移除请求**

⚠️ **新节点 peers 配置为空数组**（不自行 Bootstrap）

## 详细文档

完整操作指南请参考：[docs/CLUSTER_SCALING.md](../docs/CLUSTER_SCALING.md)

包含：
- 技术原理详解
- 故障排查指南
- 性能优化建议
- 最佳实践

## 缩容

从 5 节点缩回 3 节点：

```bash
# 移除 node5
curl -X POST http://localhost:8001/cluster/remove \
  -d '{"node_id": "node5"}'
pkill -f 'hoodb.*node5'

# 移除 node4
curl -X POST http://localhost:8001/cluster/remove \
  -d '{"node_id": "node4"}'
pkill -f 'hoodb.*node4'
```

## 配置文件

新节点配置已准备好：
- `configs/node4.json`
- `configs/node5.json`

关键配置：`"peers": []` — 新节点不自行 Bootstrap

## 总结

✅ **技术上完全可行**：底层 Raft 支持动态成员变更

✅ **零停机扩容**：业务无感知，数据零丢失

✅ **操作简单**：一行 API 调用即可

✅ **自动化支持**：提供一键扩容脚本

**推荐配置**：
- 开发/测试：3 节点
- 生产环境：5 节点（最佳平衡点）
- 关键业务：7 节点（最高可用性）
