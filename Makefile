.PHONY: build test bench run clean fmt lint help

# 项目变量
APP_NAME   := hoodb
BUILD_DIR  := bin
CMD_DIR    := cmd/hoodb
WEB_DIR    := hoodb-web

# Go 编译标志
LDFLAGS    := -s -w
GOFLAGS    := -trimpath

## help: 显示帮助信息
help:
	@echo "HooDB - 分布式 Key-Value 数据库"
	@echo ""
	@echo "使用方法:"
	@echo "  make build         编译后端"
	@echo "  make build-web     编译前端"
	@echo "  make test          运行所有测试"
	@echo "  make bench         运行性能基准测试"
	@echo "  make run           编译并启动 3 节点集群"
	@echo "  make stop          停止集群"
	@echo "  make clean         清理编译产物和数据"
	@echo "  make fmt           格式化代码"
	@echo "  make lint          静态检查"
	@echo ""

## build: 编译后端二进制
build:
	@echo "==> Building $(APP_NAME)..."
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./$(CMD_DIR)
	@echo "==> Done: $(BUILD_DIR)/$(APP_NAME)"

## build-web: 编译前端
build-web:
	@echo "==> Building frontend..."
	cd $(WEB_DIR) && npm install && npm run build
	@echo "==> Done: $(WEB_DIR)/build/"

## test: 运行所有 Go 测试
test:
	go test -race -count=1 ./...

## bench: 运行性能基准测试
bench:
	go test -bench=. -benchmem ./internal/storage/ ./benchmark/

## run: 编译并启动 3 节点集群
run: build
	@bash scripts/start_cluster.sh

## stop: 停止集群
stop:
	@bash scripts/stop_cluster.sh

## clean: 清理编译产物和数据目录
clean:
	rm -rf $(BUILD_DIR)/$(APP_NAME)
	rm -rf data/
	rm -rf logs/*.log
	rm -f pids.txt

## fmt: 格式化 Go 代码
fmt:
	gofmt -s -w .

## lint: 静态检查 (需要安装 golangci-lint)
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "请安装 golangci-lint: https://golangci-lint.run/usage/install/"; exit 1; }
	golangci-lint run ./...
