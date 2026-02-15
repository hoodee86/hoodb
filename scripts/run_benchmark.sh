#!/bin/bash

# 运行性能对比测试

set -e

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}==================================${NC}"
echo -e "${BLUE}   存储引擎性能对比测试${NC}"
echo -e "${BLUE}==================================${NC}"
echo ""

cd benchmark

echo "安装依赖..."
go mod tidy

echo ""
echo "编译测试程序..."
go build -o storage_bench storage_bench.go

echo ""
echo -e "${YELLOW}运行性能测试...${NC}"
echo ""

./storage_bench

echo ""
echo -e "${GREEN}测试完成！${NC}"
cd ..

echo ""
echo -e "${GREEN}性能对比测试完成！${NC}"

cd ..
