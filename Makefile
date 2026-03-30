BINARY_NAME=cli-proxy-api
MODULE_NAME=github.com/router-for-me/CLIProxyAPI/v6
MAIN_PATH=./cmd/server

# 获取版本信息
VERSION=$(shell git describe --tags --always || echo "dev")
GIT_COMMIT=$(shell git rev-parse --short HEAD || echo "none")
BUILD_TIME=$(shell date "+%Y-%m-%d %H:%M:%S")

# 构建注入参数
LDFLAGS=-ldflags "-X 'main.Version=${VERSION}' -X 'main.Commit=${GIT_COMMIT}' -X 'main.BuildDate=${BUILD_TIME}'"

.PHONY: build clean help

## build: 编译二进制文件并注入版本信息
build:
	@echo "正在编译 $(BINARY_NAME)..."
	@echo "Version: $(VERSION)"
	@echo "Commit: $(GIT_COMMIT)"
	@echo "BuildTime: $(BUILD_TIME)"
	go build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "编译完成！可执行文件: ./$(BINARY_NAME)"
	@ls -lh $(BINARY_NAME)

## clean: 清理编译生成的二进制文件
clean:
	rm -f $(BINARY_NAME)

## help: 显示帮助信息
help:
	@echo "可用命令:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' |  sed -e 's/^/ /'
