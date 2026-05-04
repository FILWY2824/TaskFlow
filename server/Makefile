BINARY      := taskflow-server
PKG         := ./...
GOFLAGS     := -trimpath
LDFLAGS     := -s -w
VERSION     ?= 1.3.0
ENV_FILE    ?= ../.env

.PHONY: all tidy build run test fmt vet clean lint

all: build

tidy:
	go mod tidy

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS) -X main.version=$(VERSION)" -o $(BINARY) ./cmd/server

# 交叉编译 Linux amd64,可直接拷到 VPS 运行
build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags="$(LDFLAGS) -X main.version=$(VERSION)" -o $(BINARY)-linux-amd64 ./cmd/server

# 交叉编译 Linux arm64(树莓派 / ARM 服务器)
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -ldflags="$(LDFLAGS) -X main.version=$(VERSION)" -o $(BINARY)-linux-arm64 ./cmd/server

run:
	go run ./cmd/server -config config.toml -env $(ENV_FILE)

test:
	go test $(PKG) -count=1

fmt:
	gofmt -w -s .

vet:
	go vet $(PKG)

clean:
	rm -f $(BINARY) $(BINARY)-linux-amd64 $(BINARY)-linux-arm64
