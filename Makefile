# 顶层 Makefile —— 管理 server (Go) + web (Vue) + windows (Tauri) + android (Gradle)
#
# 子项目入口:
#   server/Makefile        —— Go: build / test / run / 交叉编译
#   web/package.json       —— Vue 3 + Vite
#   windows/package.json   —— Tauri v2(复用 ../web 源码)
#   android/build.gradle   —— Kotlin / Compose
#   deploy/                —— systemd / nginx / 备份脚本

VERSION ?= 0.4.0

.PHONY: all build server-build web-build server-run web-dev server-test web-typecheck \
        windows-build windows-dev android-build android-debug android-clean \
        dist dist-src clean

all: build

# 默认构建:后端 + 前端(生产部署最常用)
build: server-build web-build

server-build:
	$(MAKE) -C server build VERSION=$(VERSION)

web-build:
	cd web && npm run build

# 跨平台后端二进制(发到 VPS)
build-linux-amd64:
	$(MAKE) -C server build-linux-amd64 VERSION=$(VERSION)
	cd web && npm run build

build-linux-arm64:
	$(MAKE) -C server build-linux-arm64 VERSION=$(VERSION)
	cd web && npm run build

# 开发模式
server-run:
	$(MAKE) -C server run

web-dev:
	cd web && npm run dev

server-test:
	$(MAKE) -C server test

web-typecheck:
	cd web && npm run type-check

test: server-test web-typecheck

# Windows Tauri(需要本地 Rust toolchain + WebView2;bundle msi/nsis 需要 Windows 主机)
windows-dev:
	cd windows && npm run tauri:dev

windows-build:
	cd windows && npm run tauri:build

# Android(需要 Android Studio / SDK / Gradle wrapper)
android-debug:
	cd android && ./gradlew :app:assembleDebug

android-build:
	cd android && ./gradlew :app:assembleRelease

android-clean:
	cd android && ./gradlew clean

# 源码 tarball(分发用,客户拿到后自行编译)
dist-src:
	rm -rf dist
	mkdir -p dist/todoalarm-$(VERSION)
	cp -r server web windows android deploy dist/todoalarm-$(VERSION)/
	cp README.md CHANGELOG.md Makefile dist/todoalarm-$(VERSION)/ 2>/dev/null || true
	find dist/todoalarm-$(VERSION) -type d \( -name node_modules -o -name target -o -name .gradle -o -name build -o -name dist \) -prune -exec rm -rf {} +
	cd dist && tar czf todoalarm-$(VERSION)-src.tar.gz todoalarm-$(VERSION)
	@echo "==> dist/todoalarm-$(VERSION)-src.tar.gz"

# 构建 server + web 后打包成可部署 tarball(给 deploy/install.sh 吃)
dist: build
	rm -rf dist
	mkdir -p dist/todoalarm-$(VERSION)
	cp server/todoalarm-server dist/todoalarm-$(VERSION)/ 2>/dev/null || true
	cp -r web/dist dist/todoalarm-$(VERSION)/web
	cp -r deploy dist/todoalarm-$(VERSION)/
	cp server/config.example.toml dist/todoalarm-$(VERSION)/
	cp README.md CHANGELOG.md dist/todoalarm-$(VERSION)/ 2>/dev/null || true
	cd dist && tar czf todoalarm-$(VERSION).tar.gz todoalarm-$(VERSION)
	@echo "==> dist/todoalarm-$(VERSION).tar.gz"

clean:
	rm -rf dist
	$(MAKE) -C server clean
	rm -rf web/dist web/node_modules/.vite
	rm -rf windows/dist windows/src-tauri/target
	rm -rf android/app/build android/build android/.gradle
