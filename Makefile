APP_NAME := msmgr
BUILD_DIR := bin
GO_CACHE := $(CURDIR)/.cache/go-build

.PHONY: build test fmt clean split-markdown

build:
	mkdir -p $(GO_CACHE) $(BUILD_DIR)
	GOCACHE=$(GO_CACHE) go build -buildvcs=false -o $(BUILD_DIR)/$(APP_NAME) ./cmd/msmgr

test:
	mkdir -p $(GO_CACHE)
	GOCACHE=$(GO_CACHE) go test ./...

fmt:
	gofmt -w ./cmd ./internal

clean:
	rm -rf $(BUILD_DIR)

split-markdown: build
	./bin/$(APP_NAME) split-markdown $(ARGS)
