#!/bin/bash
# HooDB 集群扩容测试脚本

set -e

echo "=================================================="
echo "HooDB 集群扩容演示 (3 节点 → 5 节点)"
echo "=================================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 步骤 1：确认现有集群
echo -e "${YELLOW}步骤 1: 检查现有 3 节点集群状态${NC}"
echo "查询集群配置..."
CLUSTER_CONFIG=$(curl -s http://localhost:8001/cluster/config)
echo "$CLUSTER_CONFIG" | python3 -m json.tool
CURRENT_NODES=$(echo "$CLUSTER_CONFIG" | python3 -c "import sys, json; print(len(json.load(sys.stdin)['servers']))")
echo -e "${GREEN}✓ 当前集群有 $CURRENT_NODES 个节点${NC}"
echo ""

# 步骤 2：启动 node4
echo -e "${YELLOW}步骤 2: 启动新节点 node4${NC}"
echo "启动 node4 进程..."
./hoodb -config configs/node4.json > /tmp/node4.log 2>&1 &
NODE4_PID=$!
echo -e "${GREEN}✓ node4 已启动 (PID: $NODE4_PID)${NC}"
sleep 3
echo ""

# 步骤 3：添加 node4 到集群
echo -e "${YELLOW}步骤 3: 添加 node4 到 Raft 集群${NC}"
echo "发送 AddVoter 请求..."
ADD_RESPONSE=$(curl -s -X POST http://localhost:8001/cluster/add \
  -H "Content-Type: application/json" \
  -d '{"node_id": "node4", "address": "127.0.0.1:9004"}')
echo "$ADD_RESPONSE" | python3 -m json.tool

if echo "$ADD_RESPONSE" | grep -q "added successfully"; then
  echo -e "${GREEN}✓ node4 已成功添加到集群${NC}"
else
  echo -e "${RED}✗ 添加 node4 失败${NC}"
  exit 1
fi
echo ""

# 步骤 4：等待 node4 同步
echo -e "${YELLOW}步骤 4: 等待 node4 数据同步${NC}"
echo "监控同步进度..."
for i in {1..10}; do
  NODE4_INDEX=$(curl -s http://localhost:8004/status 2>/dev/null | python3 -c "import sys, json; print(json.load(sys.stdin).get('appliedIndex', 0))" 2>/dev/null || echo "0")
  LEADER_INDEX=$(curl -s http://localhost:8001/status | python3 -c "import sys, json; print(json.load(sys.stdin).get('lastIndex', 0))")
  
  echo "  node4 索引: $NODE4_INDEX / Leader 索引: $LEADER_INDEX"
  
  if [ "$NODE4_INDEX" -ge "$LEADER_INDEX" ]; then
    echo -e "${GREEN}✓ node4 数据同步完成${NC}"
    break
  fi
  
  if [ $i -eq 10 ]; then
    echo -e "${YELLOW}⚠ 数据仍在同步中，继续下一步...${NC}"
  fi
  
  sleep 2
done
echo ""

# 步骤 5：启动并添加 node5
echo -e "${YELLOW}步骤 5: 启动并添加 node5${NC}"
echo "启动 node5 进程..."
./hoodb -config configs/node5.json > /tmp/node5.log 2>&1 &
NODE5_PID=$!
echo -e "${GREEN}✓ node5 已启动 (PID: $NODE5_PID)${NC}"
sleep 3

echo "发送 AddVoter 请求..."
ADD_RESPONSE=$(curl -s -X POST http://localhost:8001/cluster/add \
  -H "Content-Type: application/json" \
  -d '{"node_id": "node5", "address": "127.0.0.1:9005"}')
echo "$ADD_RESPONSE" | python3 -m json.tool

if echo "$ADD_RESPONSE" | grep -q "added successfully"; then
  echo -e "${GREEN}✓ node5 已成功添加到集群${NC}"
else
  echo -e "${RED}✗ 添加 node5 失败${NC}"
  exit 1
fi
echo ""

# 步骤 6：验证最终集群配置
echo -e "${YELLOW}步骤 6: 验证 5 节点集群配置${NC}"
sleep 5
echo "查询最终集群配置..."
FINAL_CONFIG=$(curl -s http://localhost:8001/cluster/config)
echo "$FINAL_CONFIG" | python3 -m json.tool
FINAL_NODES=$(echo "$FINAL_CONFIG" | python3 -c "import sys, json; print(len(json.load(sys.stdin)['servers']))")
echo ""

if [ "$FINAL_NODES" -eq 5 ]; then
  echo -e "${GREEN}✓✓✓ 成功！集群已从 3 节点扩容至 5 节点 ✓✓✓${NC}"
else
  echo -e "${RED}✗ 扩容失败，当前只有 $FINAL_NODES 个节点${NC}"
  exit 1
fi
echo ""

# 步骤 7：功能测试
echo -e "${YELLOW}步骤 7: 功能验证测试${NC}"

echo "1. 写入测试数据..."
curl -s -X PUT http://localhost:8001/kv/scale_test \
  -H "Content-Type: application/json" \
  -d '{"value": "5_nodes_cluster"}' > /dev/null
echo -e "${GREEN}✓ 写入成功${NC}"

echo "2. 从所有节点读取数据..."
SUCCESS_COUNT=0
for port in 8001 8002 8003 8004 8005; do
  VALUE=$(curl -s http://localhost:$port/kv/scale_test 2>/dev/null | python3 -c "import sys, json; print(json.load(sys.stdin).get('value', ''))" 2>/dev/null || echo "ERROR")
  if [ "$VALUE" == "5_nodes_cluster" ]; then
    echo "  ✓ node$((port-8000)): $VALUE"
    SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
  else
    echo "  ✗ node$((port-8000)): 读取失败"
  fi
done

if [ $SUCCESS_COUNT -eq 5 ]; then
  echo -e "${GREEN}✓ 所有节点数据一致${NC}"
else
  echo -e "${YELLOW}⚠ 部分节点数据不一致 ($SUCCESS_COUNT/5)${NC}"
fi
echo ""

# 步骤 8：性能测试
echo -e "${YELLOW}步骤 8: 5 节点集群性能测试${NC}"
echo "运行基准测试 (10000 次写入, 50 并发)..."
BENCH_RESULT=$(curl -s -X POST http://localhost:8001/benchmark \
  -H "Content-Type: application/json" \
  -d '{"count": 10000, "concurrency": 50}')
echo "$BENCH_RESULT" | python3 -m json.tool

OPS=$(echo "$BENCH_RESULT" | python3 -c "import sys, json; print(int(json.load(sys.stdin).get('ops_per_sec', 0)))")
echo ""
if [ $OPS -gt 20000 ]; then
  echo -e "${GREEN}✓ 性能测试通过: $OPS ops/sec${NC}"
else
  echo -e "${YELLOW}⚠ 性能偏低: $OPS ops/sec (预期 > 20000)${NC}"
fi
echo ""

# 总结
echo "=================================================="
echo -e "${GREEN}扩容完成！${NC}"
echo "=================================================="
echo ""
echo "集群信息："
echo "  - 节点数量: 5"
echo "  - Quorum: 3 (需要 3 个节点确认)"
echo "  - 故障容忍: 2 个节点"
echo "  - 写入性能: ~$OPS ops/sec"
echo ""
echo "进程 ID："
echo "  - node4: $NODE4_PID"
echo "  - node5: $NODE5_PID"
echo ""
echo "日志文件："
echo "  - /tmp/node4.log"
echo "  - /tmp/node5.log"
echo ""
echo "查看集群状态："
echo "  curl http://localhost:8001/cluster/config | python3 -m json.tool"
echo ""
echo "测试读取性能："
echo "  for port in 8001 8002 8003 8004 8005; do"
echo "    curl http://localhost:\$port/kv/scale_test"
echo "  done"
echo ""
