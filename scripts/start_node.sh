#!/bin/bash

# 启动单个节点的脚本
# 用法: ./start_node.sh <node_number>

if [ -z "$1" ]; then
    echo "Usage: $0 <node_number>"
    echo "Example: $0 1"
    exit 1
fi

NODE_NUM=$1
CONFIG_FILE="./configs/node${NODE_NUM}.json"

if [ ! -f "$CONFIG_FILE" ]; then
    echo "Error: Config file $CONFIG_FILE not found"
    exit 1
fi

echo "Starting HooDB node ${NODE_NUM}..."
echo "Config: $CONFIG_FILE"

# 构建程序（如果需要）
if [ ! -f "./bin/hoodb" ]; then
    echo "Building hoodb..."
    go build -o ./bin/hoodb ./cmd/hoodb
fi

# 启动节点
./bin/hoodb -config "$CONFIG_FILE"
