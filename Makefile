.DEFAULT_GOAL := help

.PHONY: help install build test lint coverage clean

# ------------------------------------------------------------------------------
# Help
# ------------------------------------------------------------------------------
help: ## ヘルプの表示
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } /^## / { printf "%s\n", substr($$0, 4) } ' $(MAKEFILE_LIST)

##@ Dependencies
install: ## 依存関係の整理
	go mod tidy

##@ Development
lint: ## コードの静的解析
	go vet ./...

test: ## 単体テストの実行
	go test -v ./...

coverage: ## カバレッジ測定の実行
	go test -coverprofile=coverage.out ./...

##@ Build
build: ## バイナリのビルド
	go build -o bin/gh-secret-sync main.go

##@ Cleanup
clean: ## ビルド成果物および一時ファイルの削除
	rm -rf bin coverage.out

