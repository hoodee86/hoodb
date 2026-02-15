#!/bin/bash

# 启动整个集群的脚本

echo "Building HooDB..."
go build -o ./bin/hoodb ./cmd/hoodb

if [ $? -ne 0 ]; then
    echo "Build failed!"
    exit 1
fi

echo "Starting HooDB cluster (3 nodes)..."

# 清理旧的数据目录（可选）
# rm -rf ./data

# 启动节点1（后台运行）
echo "Starting node1..."
./bin/hoodb -config ./configs/node1.json > ./logs/node1.log 2>&1 &
NODE1_PID=$!
echo "Node1 started with PID: $NODE1_PID"

# 等待一下让第一个节点启动
sleep 3

# 启动节点2
echo "Starting node2..."
./bin/hoodb -config ./configs/node2.json > ./logs/node2.log 2>&1 &
NODE2_PID=$!
echo "Node2 started with PID: $NODE2_PID"

# 启动节点3
echo "Starting node3..."
./bin/hoodb -config ./configs/node3.json > ./logs/node3.log 2>&1 &
NODE3_PID=$!
echo "Node3 started with PID: $NODE3_PID"

echo ""
echo "============================================"
echo "HooDB cluster started successfully!"
echo "============================================"
echo "Node1: http://localhost:8001 (PID: $NODE1_PID)"
echo "Node2: http://localhost:8002 (PID: $NODE2_PID)"
echo "Node3: http://localhost:8003 (PID: $NODE3_PID)"
echo ""
echo "Logs:"
echo "  Node1: ./logs/node1.log"
echo "  Node2: ./logs/node2.log"
echo "  Node3: ./logs/node3.log"
echo ""
echo "To stop the cluster, run: ./stop_cluster.sh"
echo ""
echo "PIDs saved to pids.txt"
echo "$NODE1_PID" > pids.txt
echo "$NODE2_PID" >> pids.txt
echo "$NODE3_PID" >> pids.txt
