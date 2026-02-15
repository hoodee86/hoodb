#!/bin/bash

# HooDB 压力测试脚本
# 测试包括：并发写入、并发读取、混合读写

set -e

BLUE='\033[0;34m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}==================================${NC}"
echo -e "${BLUE}   HooDB 压力测试${NC}"
echo -e "${BLUE}==================================${NC}"
echo ""

# 检查集群是否运行
check_cluster() {
    for port in 8001 8002 8003; do
        if ! curl -s http://localhost:$port/health > /dev/null 2>&1; then
            echo -e "${RED}错误: 节点 $port 未运行${NC}"
            echo "请先启动集群: ./start_cluster.sh"
            exit 1
        fi
    done
    echo -e "${GREEN}✓ 集群运行正常${NC}"
}

# 查找Leader节点
find_leader() {
    for port in 8001 8002 8003; do
        status=$(curl -s http://localhost:$port/status)
        is_leader=$(echo $status | grep -o '"isLeader":[^,}]*' | cut -d':' -f2)
        if [ "$is_leader" = "true" ]; then
            echo $port
            return
        fi
    done
    echo "8001"
}

# 测试1: 顺序写入测试
test_sequential_write() {
    echo -e "\n${YELLOW}[测试 1] 顺序写入测试${NC}"
    local leader_port=$(find_leader)
    local num_ops=1000
    local start_time=$(date +%s.%N)
    
    for i in $(seq 1 $num_ops); do
        curl -s -X PUT http://localhost:$leader_port/kv/seq_key_$i \
            -H "Content-Type: application/json" \
            -d "{\"value\":\"seq_value_$i\"}" > /dev/null
        
        if [ $((i % 100)) -eq 0 ]; then
            echo -ne "\r进度: $i/$num_ops"
        fi
    done
    
    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc)
    local ops_per_sec=$(echo "scale=2; $num_ops / $duration" | bc)
    
    echo ""
    echo -e "${GREEN}✓ 顺序写入 $num_ops 条记录${NC}"
    echo -e "  耗时: ${duration}s"
    echo -e "  吞吐量: ${ops_per_sec} ops/sec"
}

# 测试2: 并发写入测试
test_concurrent_write() {
    echo -e "\n${YELLOW}[测试 2] 并发写入测试${NC}"
    local leader_port=$(find_leader)
    local num_workers=10
    local ops_per_worker=100
    
    local start_time=$(date +%s.%N)
    
    for worker in $(seq 1 $num_workers); do
        (
            for i in $(seq 1 $ops_per_worker); do
                curl -s -X PUT http://localhost:$leader_port/kv/con_key_${worker}_${i} \
                    -H "Content-Type: application/json" \
                    -d "{\"value\":\"con_value_${worker}_${i}\"}" > /dev/null
            done
        ) &
    done
    
    wait
    
    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc)
    local total_ops=$((num_workers * ops_per_worker))
    local ops_per_sec=$(echo "scale=2; $total_ops / $duration" | bc)
    
    echo -e "${GREEN}✓ 并发写入 $total_ops 条记录 ($num_workers 个并发)${NC}"
    echo -e "  耗时: ${duration}s"
    echo -e "  吞吐量: ${ops_per_sec} ops/sec"
}

# 测试3: 并发读取测试
test_concurrent_read() {
    echo -e "\n${YELLOW}[测试 3] 并发读取测试${NC}"
    local num_workers=20
    local ops_per_worker=100
    
    local start_time=$(date +%s.%N)
    
    for worker in $(seq 1 $num_workers); do
        (
            for i in $(seq 1 $ops_per_worker); do
                # 随机选择节点进行读取
                local port=$((8001 + RANDOM % 3))
                local key_id=$((1 + RANDOM % 100))
                curl -s http://localhost:$port/kv/seq_key_$key_id > /dev/null
            done
        ) &
    done
    
    wait
    
    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc)
    local total_ops=$((num_workers * ops_per_worker))
    local ops_per_sec=$(echo "scale=2; $total_ops / $duration" | bc)
    
    echo -e "${GREEN}✓ 并发读取 $total_ops 次 ($num_workers 个并发)${NC}"
    echo -e "  耗时: ${duration}s"
    echo -e "  吞吐量: ${ops_per_sec} ops/sec"
}

# 测试4: 混合读写测试
test_mixed_operations() {
    echo -e "\n${YELLOW}[测试 4] 混合读写测试 (70% 读 + 30% 写)${NC}"
    local leader_port=$(find_leader)
    local num_workers=10
    local ops_per_worker=100
    
    local start_time=$(date +%s.%N)
    
    for worker in $(seq 1 $num_workers); do
        (
            for i in $(seq 1 $ops_per_worker); do
                local rand=$((RANDOM % 100))
                if [ $rand -lt 30 ]; then
                    # 写操作 (30%)
                    curl -s -X PUT http://localhost:$leader_port/kv/mix_key_${worker}_${i} \
                        -H "Content-Type: application/json" \
                        -d "{\"value\":\"mix_value_${worker}_${i}\"}" > /dev/null
                else
                    # 读操作 (70%)
                    local port=$((8001 + RANDOM % 3))
                    local key_id=$((1 + RANDOM % 100))
                    curl -s http://localhost:$port/kv/seq_key_$key_id > /dev/null
                fi
            done
        ) &
    done
    
    wait
    
    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc)
    local total_ops=$((num_workers * ops_per_worker))
    local ops_per_sec=$(echo "scale=2; $total_ops / $duration" | bc)
    
    echo -e "${GREEN}✓ 混合操作 $total_ops 次 ($num_workers 个并发)${NC}"
    echo -e "  耗时: ${duration}s"
    echo -e "  吞吐量: ${ops_per_sec} ops/sec"
}

# 测试5: 大值写入测试
test_large_value() {
    echo -e "\n${YELLOW}[测试 5] 大值写入测试 (每个值 10KB)${NC}"
    local leader_port=$(find_leader)
    local num_ops=100
    local large_value=$(openssl rand -base64 10240) # 10KB
    
    local start_time=$(date +%s.%N)
    
    for i in $(seq 1 $num_ops); do
        curl -s -X PUT http://localhost:$leader_port/kv/large_key_$i \
            -H "Content-Type: application/json" \
            -d "{\"value\":\"$large_value\"}" > /dev/null
        
        if [ $((i % 10)) -eq 0 ]; then
            echo -ne "\r进度: $i/$num_ops"
        fi
    done
    
    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc)
    local ops_per_sec=$(echo "scale=2; $num_ops / $duration" | bc)
    local throughput_mb=$(echo "scale=2; $num_ops * 10 / $duration / 1024" | bc)
    
    echo ""
    echo -e "${GREEN}✓ 写入 $num_ops 个大值 (每个10KB)${NC}"
    echo -e "  耗时: ${duration}s"
    echo -e "  吞吐量: ${ops_per_sec} ops/sec (${throughput_mb} MB/s)"
}

# 测试6: 批量写入API测试
test_batch_write() {
    echo -e "\n${YELLOW}[测试 6] 批量写入API测试${NC}"
    local leader_port=$(find_leader)
    local num_batches=20
    local batch_size=50
    local total_ops=$((num_batches * batch_size))
    
    local start_time=$(date +%s.%N)
    
    for b in $(seq 1 $num_batches); do
        # 构建批量JSON
        local items="{"
        local first=true
        for i in $(seq 1 $batch_size); do
            if [ "$first" = true ]; then
                first=false
            else
                items+=","
            fi
            items+="\"batch_k_${b}_${i}\":\"batch_v_${b}_${i}\""
        done
        items+="}"
        
        curl -s -X POST http://localhost:$leader_port/kv/batch \
            -H "Content-Type: application/json" \
            -d "{\"items\":$items}" > /dev/null
        
        if [ $((b % 5)) -eq 0 ]; then
            echo -ne "\r进度: $((b * batch_size))/$total_ops"
        fi
    done
    
    local end_time=$(date +%s.%N)
    local duration=$(echo "$end_time - $start_time" | bc)
    local ops_per_sec=$(echo "scale=2; $total_ops / $duration" | bc)
    
    echo ""
    echo -e "${GREEN}✓ 批量写入 $total_ops 条记录 ($num_batches 批次 x $batch_size 每批)${NC}"
    echo -e "  耗时: ${duration}s"
    echo -e "  吞吐量: ${ops_per_sec} ops/sec"
}

# 主函数
main() {
    echo "检查集群状态..."
    check_cluster
    
    echo ""
    echo "开始压力测试..."
    
    test_sequential_write
    test_concurrent_write
    test_concurrent_read
    test_mixed_operations
    test_large_value
    test_batch_write
    
    echo ""
    echo -e "${GREEN}==================================${NC}"
    echo -e "${GREEN}   压力测试完成！${NC}"
    echo -e "${GREEN}==================================${NC}"
    
    # 显示集群状态
    echo ""
    echo "集群状态:"
    for port in 8001 8002 8003; do
        echo -e "\n节点 $port:"
        curl -s http://localhost:$port/status | python3 -m json.tool | grep -E '"isLeader"|"state"|"lastIndex"'
    done
}

main
