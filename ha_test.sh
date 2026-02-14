#!/bin/bash

# HooDB 高可用性测试脚本
# 测试包括：节点故障恢复、Leader选举、数据一致性

set -e

BLUE='\033[0;34m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}==================================${NC}"
echo -e "${BLUE}   HooDB 高可用性测试${NC}"
echo -e "${BLUE}==================================${NC}"
echo ""

# 查找Leader节点
find_leader() {
    for port in 8001 8002 8003; do
        if curl -s http://localhost:$port/health > /dev/null 2>&1; then
            status=$(curl -s http://localhost:$port/status 2>/dev/null)
            is_leader=$(echo $status | grep -o '"isLeader":[^,}]*' | cut -d':' -f2)
            if [ "$is_leader" = "true" ]; then
                echo $port
                return
            fi
        fi
    done
    echo ""
}

# 查找节点的PID
find_pid_by_port() {
    local port=$1
    lsof -ti:$port 2>/dev/null || echo ""
}

# 停止指定节点
stop_node() {
    local port=$1
    local pid=$(find_pid_by_port $port)
    
    if [ -n "$pid" ]; then
        echo -e "${YELLOW}停止节点 $port (PID: $pid)${NC}"
        kill $pid
        sleep 2
        return 0
    else
        echo -e "${RED}未找到节点 $port${NC}"
        return 1
    fi
}

# 启动指定节点
start_node() {
    local node_num=$1
    echo -e "${GREEN}启动节点 $node_num${NC}"
    ./bin/hoodb -config ./configs/node${node_num}.json > ./logs/node${node_num}.log 2>&1 &
    sleep 3
}

# 写入测试数据 (带重试机制)
write_data() {
    local leader_port=$1
    local prefix=$2
    local count=$3
    
    for i in $(seq 1 $count); do
        local retry=0
        local max_retry=3
        while [ $retry -lt $max_retry ]; do
            # 检查当前节点是否仍为Leader
            local current_leader=$(find_leader)
            if [ -z "$current_leader" ]; then
                sleep 1
                retry=$((retry + 1))
                continue
            fi
            result=$(curl -s -w "\n%{http_code}" -X PUT http://localhost:$current_leader/kv/${prefix}_key_$i \
                -H "Content-Type: application/json" \
                -d "{\"value\":\"${prefix}_value_$i\"}" 2>/dev/null)
            http_code=$(echo "$result" | tail -1)
            if [ "$http_code" = "200" ]; then
                break
            fi
            sleep 0.5
            retry=$((retry + 1))
        done
    done
}

# 验证数据一致性
verify_data() {
    local port=$1
    local prefix=$2
    local count=$3
    local failed=0
    
    for i in $(seq 1 $count); do
        result=$(curl -s http://localhost:$port/kv/${prefix}_key_$i 2>/dev/null)
        expected="\"value\":\"${prefix}_value_$i\""
        if ! echo "$result" | grep -q "$expected"; then
            failed=$((failed + 1))
        fi
    done
    
    echo $failed
}

# 测试1: 基础高可用测试
test_basic_ha() {
    echo -e "\n${YELLOW}[测试 1] 基础高可用测试${NC}"
    echo "写入初始数据..."
    
    local leader_port=$(find_leader)
    if [ -z "$leader_port" ]; then
        echo -e "${RED}错误: 无法找到Leader节点${NC}"
        return 1
    fi
    
    write_data $leader_port "ha_test" 50
    echo -e "${GREEN}✓ 写入 50 条记录到 Leader (端口 $leader_port)${NC}"
    
    # 从所有节点读取验证
    echo "验证数据一致性..."
    for port in 8001 8002 8003; do
        if curl -s http://localhost:$port/health > /dev/null 2>&1; then
            failed=$(verify_data $port "ha_test" 50)
            if [ $failed -eq 0 ]; then
                echo -e "${GREEN}✓ 节点 $port 数据一致${NC}"
            else
                echo -e "${RED}✗ 节点 $port 有 $failed 条数据不一致${NC}"
            fi
        fi
    done
}

# 测试2: Follower节点故障测试
test_follower_failure() {
    echo -e "\n${YELLOW}[测试 2] Follower节点故障测试${NC}"
    
    local leader_port=$(find_leader)
    echo "当前 Leader: $leader_port"
    
    # 找到一个Follower节点
    local follower_port=""
    for port in 8001 8002 8003; do
        if [ "$port" != "$leader_port" ]; then
            follower_port=$port
            break
        fi
    done
    
    # 停止Follower节点
    stop_node $follower_port
    echo -e "${YELLOW}已停止 Follower 节点 $follower_port${NC}"
    sleep 2
    
    # 继续写入数据
    echo "在 Follower 故障期间写入数据..."
    write_data $leader_port "follower_down" 30
    echo -e "${GREEN}✓ 写入 30 条记录 (Follower $follower_port 已停止)${NC}"
    
    # 重启Follower节点
    local node_num=$((follower_port - 8000))
    start_node $node_num
    echo "等待节点同步..."
    sleep 10
    
    # 验证数据同步
    echo "验证重启后的数据一致性..."
    failed=$(verify_data $follower_port "follower_down" 30)
    if [ $failed -eq 0 ]; then
        echo -e "${GREEN}✓ 节点 $follower_port 重启后数据同步成功${NC}"
    else
        echo -e "${RED}✗ 节点 $follower_port 有 $failed 条数据未同步${NC}"
    fi
}

# 测试3: Leader节点故障测试
test_leader_failure() {
    echo -e "\n${YELLOW}[测试 3] Leader节点故障测试 (最关键)${NC}"
    
    local old_leader=$(find_leader)
    echo "当前 Leader: $old_leader"
    
    # 写入一些数据
    write_data $old_leader "before_leader_down" 20
    echo -e "${GREEN}✓ 写入 20 条记录到当前 Leader${NC}"
    
    # 停止Leader节点
    stop_node $old_leader
    echo -e "${RED}✗ 已停止 Leader 节点 $old_leader${NC}"
    
    # 等待新Leader选举
    echo "等待新 Leader 选举..."
    local new_leader=""
    for attempt in $(seq 1 20); do
        sleep 1
        new_leader=$(find_leader)
        if [ -n "$new_leader" ] && [ "$new_leader" != "$old_leader" ]; then
            break
        fi
        echo -ne "\r  等待中... ${attempt}s"
    done
    echo ""
    
    # 查找新Leader
    local new_leader=$(find_leader)
    if [ -z "$new_leader" ]; then
        echo -e "${RED}错误: 未能选举出新 Leader${NC}"
        # 重启旧Leader以恢复集群
        local node_num=$((old_leader - 8000))
        start_node $node_num
        return 1
    fi
    
    echo -e "${GREEN}✓ 新 Leader 已选举: $new_leader${NC}"
    
    # 在新Leader上写入数据
    echo "在新 Leader 上写入数据..."
    write_data $new_leader "after_leader_down" 20
    echo -e "${GREEN}✓ 写入 20 条记录到新 Leader${NC}"
    
    # 重启旧Leader
    local node_num=$((old_leader - 8000))
    start_node $node_num
    echo "等待旧 Leader 节点重新加入集群..."
    sleep 5
    
    # 验证旧Leader上的数据
    echo "验证旧 Leader 重启后的数据一致性..."
    failed1=$(verify_data $old_leader "before_leader_down" 20)
    failed2=$(verify_data $old_leader "after_leader_down" 20)
    
    if [ $failed1 -eq 0 ] && [ $failed2 -eq 0 ]; then
        echo -e "${GREEN}✓ 旧 Leader 节点 $old_leader 重启后数据完全同步${NC}"
    else
        echo -e "${RED}✗ 旧 Leader 节点数据同步失败 (before: $failed1, after: $failed2)${NC}"
    fi
}

# 测试4: 多节点同时故障测试
test_multi_node_failure() {
    echo -e "\n${YELLOW}[测试 4] 多节点故障测试 (2/3节点故障)${NC}"
    
    local leader_port=$(find_leader)
    echo "当前 Leader: $leader_port"
    
    # 找到两个节点停止
    local nodes_to_stop=()
    local stop_count=0
    for port in 8001 8002 8003; do
        if [ $stop_count -lt 2 ]; then
            nodes_to_stop+=($port)
            stop_count=$((stop_count + 1))
        fi
    done
    
    # 停止两个节点
    for port in "${nodes_to_stop[@]}"; do
        stop_node $port
    done
    
    echo -e "${RED}已停止 2/3 节点${NC}"
    sleep 3
    
    # 尝试写入数据（应该失败或超时）
    echo "尝试在只有 1/3 节点的情况下写入..."
    local remaining_port=""
    for port in 8001 8002 8003; do
        if curl -s http://localhost:$port/health > /dev/null 2>&1; then
            remaining_port=$port
            break
        fi
    done
    
    if [ -n "$remaining_port" ]; then
        result=$(curl -s -X PUT http://localhost:$remaining_port/kv/multi_down_test \
            -H "Content-Type: application/json" \
            -d '{"value":"test"}' 2>/dev/null || echo "failed")
        
        if echo "$result" | grep -q "not leader"; then
            echo -e "${GREEN}✓ 正确行为: 集群无法选举 Leader (需要多数节点)${NC}"
        else
            echo -e "${YELLOW}⚠ 意外行为: $result${NC}"
        fi
    fi
    
    # 重启节点
    echo "重启已停止的节点..."
    for port in "${nodes_to_stop[@]}"; do
        local node_num=$((port - 8000))
        start_node $node_num
    done
    
    sleep 8
    echo -e "${GREEN}✓ 节点已重启，集群恢复${NC}"
    
    # 验证集群恢复
    local new_leader=$(find_leader)
    if [ -n "$new_leader" ]; then
        echo -e "${GREEN}✓ 集群已恢复，新 Leader: $new_leader${NC}"
    else
        echo -e "${RED}✗ 集群未能恢复${NC}"
    fi
}

# 测试5: 持续负载下的故障测试
test_failure_under_load() {
    echo -e "\n${YELLOW}[测试 5] 持续负载下的故障测试${NC}"
    
    local leader_port=$(find_leader)
    echo "当前 Leader: $leader_port"
    
    # 启动后台写入任务 (带自动重试和Leader跟随)
    echo "启动后台写入负载..."
    (
        for i in $(seq 1 100); do
            local retry=0
            while [ $retry -lt 5 ]; do
                local current_leader=$(find_leader)
                if [ -n "$current_leader" ]; then
                    result=$(curl -s -w "\n%{http_code}" --max-time 2 -X PUT http://localhost:$current_leader/kv/load_key_$i \
                        -H "Content-Type: application/json" \
                        -d "{\"value\":\"load_value_$i\"}" 2>/dev/null)
                    http_code=$(echo "$result" | tail -1)
                    if [ "$http_code" = "200" ]; then
                        break
                    fi
                fi
                sleep 0.3
                retry=$((retry + 1))
            done
            sleep 0.05
        done
    ) &
    local bg_pid=$!
    
    sleep 2
    
    # 在负载期间停止Leader
    local target_leader=$(find_leader)
    echo "在负载期间停止 Leader $target_leader..."
    stop_node $target_leader
    
    # 等待后台任务完成
    wait $bg_pid
    
    echo "后台写入任务完成"
    sleep 3
    
    # 检查有多少数据成功写入
    local new_leader=$(find_leader)
    if [ -n "$new_leader" ]; then
        echo "新 Leader: $new_leader"
        local success_count=0
        for i in $(seq 1 100); do
            if curl -s http://localhost:$new_leader/kv/load_key_$i 2>/dev/null | grep -q "load_value_$i"; then
                success_count=$((success_count + 1))
            fi
        done
        
        echo -e "${GREEN}✓ 负载测试: $success_count/100 条记录成功写入${NC}"
        
        if [ $success_count -ge 50 ]; then
            echo -e "${GREEN}✓ 系统在故障期间保持了良好的可用性${NC}"
        else
            echo -e "${YELLOW}⚠ 故障期间可用性较低${NC}"
        fi
    else
        echo -e "${RED}✗ 集群未能恢复${NC}"
    fi
    
    # 重启停止的节点
    local node_num=$((target_leader - 8000))
    start_node $node_num
    sleep 5
}

# 主函数
main() {
    echo "开始高可用性测试..."
    echo "注意: 此测试将多次启停节点，请确保集群正常运行"
    echo ""
    
    # 检查集群是否运行
    local running_nodes=0
    for port in 8001 8002 8003; do
        if curl -s http://localhost:$port/health > /dev/null 2>&1; then
            running_nodes=$((running_nodes + 1))
        fi
    done
    
    if [ $running_nodes -lt 3 ]; then
        echo -e "${RED}错误: 需要3个节点都在运行${NC}"
        echo "请先启动集群: ./start_cluster.sh"
        exit 1
    fi
    
    test_basic_ha
    test_follower_failure
    test_leader_failure
    test_multi_node_failure
    test_failure_under_load
    
    echo ""
    echo -e "${GREEN}==================================${NC}"
    echo -e "${GREEN}   高可用性测试完成！${NC}"
    echo -e "${GREEN}==================================${NC}"
    
    # 显示最终集群状态
    echo ""
    echo "最终集群状态:"
    for port in 8001 8002 8003; do
        if curl -s http://localhost:$port/health > /dev/null 2>&1; then
            echo -e "\n节点 $port:"
            curl -s http://localhost:$port/status | python3 -m json.tool | grep -E '"isLeader"|"state"|"lastIndex"'
        else
            echo -e "\n${RED}节点 $port: 未运行${NC}"
        fi
    done
}

main
