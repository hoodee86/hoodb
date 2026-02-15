#!/bin/bash

# HooDB API 测试脚本

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 找到Leader节点
find_leader() {
    for port in 8001 8002 8003; do
        status=$(curl -s http://localhost:$port/status)
        is_leader=$(echo $status | grep -o '"isLeader":[^,}]*' | cut -d':' -f2)
        if [ "$is_leader" = "true" ]; then
            echo $port
            return
        fi
    done
    echo "8001" # 默认返回8001
}

echo -e "${BLUE}==================================${NC}"
echo -e "${BLUE}   HooDB API 测试脚本${NC}"
echo -e "${BLUE}==================================${NC}"
echo ""

# 等待集群启动
echo -e "${BLUE}等待集群启动...${NC}"
sleep 2

# 查找Leader
LEADER_PORT=$(find_leader)
LEADER_URL="http://localhost:$LEADER_PORT"

echo -e "${GREEN}找到Leader节点: $LEADER_URL${NC}"
echo ""

# 测试1: 查看集群状态
echo -e "${BLUE}测试 1: 查看集群状态${NC}"
echo "GET $LEADER_URL/status"
curl -s $LEADER_URL/status | python3 -m json.tool
echo ""
echo ""

# 测试2: 设置key-value
echo -e "${BLUE}测试 2: 设置 key-value${NC}"
echo "PUT $LEADER_URL/kv/name"
curl -X PUT $LEADER_URL/kv/name \
  -H "Content-Type: application/json" \
  -d '{"value":"HooDB"}' | python3 -m json.tool
echo ""
echo ""

echo "PUT $LEADER_URL/kv/version"
curl -X PUT $LEADER_URL/kv/version \
  -H "Content-Type: application/json" \
  -d '{"value":"1.0.0"}' | python3 -m json.tool
echo ""
echo ""

echo "PUT $LEADER_URL/kv/description"
curl -X PUT $LEADER_URL/kv/description \
  -H "Content-Type: application/json" \
  -d '{"value":"Distributed KV database"}' | python3 -m json.tool
echo ""
echo ""

# 测试3: 获取key-value
echo -e "${BLUE}测试 3: 获取 key-value${NC}"
echo "GET $LEADER_URL/kv/name"
curl -s $LEADER_URL/kv/name | python3 -m json.tool
echo ""
echo ""

echo "GET $LEADER_URL/kv/version"
curl -s $LEADER_URL/kv/version | python3 -m json.tool
echo ""
echo ""

# 测试4: 从其他节点读取（测试数据同步）
echo -e "${BLUE}测试 4: 从不同节点读取数据（验证同步）${NC}"
for port in 8001 8002 8003; do
    echo "GET http://localhost:$port/kv/name"
    curl -s http://localhost:$port/kv/name | python3 -m json.tool
    echo ""
done
echo ""

# 测试5: 删除key
echo -e "${BLUE}测试 5: 删除 key${NC}"
echo "DELETE $LEADER_URL/kv/description"
curl -X DELETE $LEADER_URL/kv/description | python3 -m json.tool
echo ""
echo ""

# 测试6: 验证删除
echo -e "${BLUE}测试 6: 验证删除${NC}"
echo "GET $LEADER_URL/kv/description"
curl -s $LEADER_URL/kv/description | python3 -m json.tool
echo ""
echo ""

# 测试7: 健康检查
echo -e "${BLUE}测试 7: 健康检查${NC}"
echo "GET $LEADER_URL/health"
curl -s $LEADER_URL/health | python3 -m json.tool
echo ""
echo ""

echo -e "${GREEN}==================================${NC}"
echo -e "${GREEN}   测试完成！${NC}"
echo -e "${GREEN}==================================${NC}"
