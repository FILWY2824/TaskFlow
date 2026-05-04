# 顶层 Makefile —— 管理 server (Go) + web (Vue) + windows (Tauri) + android (Gradle)
#
# 子项目入口:
#   server/Makefile        —— Go: build / test / run / 交叉编译
#   web/package.json       —— Vue 3 + Vite
#   windows/package.json   —— Tauri v2(复用 ../web 源码)
#   android/build.gradle   —— Kotlin / Compose
#   deploy/                —— systemd / nginx / 备份脚本

VERSION ?= 1.3.0

.PHONY: all build dev server-build web-build server-run web-dev server-test web-typecheck \
        windows-build windows-dev android-build android-debug android-clean publish-release \
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
# dev: 在两个新的 Windows 终端里分别启动后端和前端
dev:
	cmd /c start "TaskFlow Server" cmd /k "cd /d server && go run ./cmd/server -config config.toml -env ../.env"
	cmd /c start "TaskFlow Web" cmd /k "cd /d web && npm run dev"

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
	cd windows && npm run build
	cd windows && npx tauri build
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/publish-release.ps1 -Platform windows -Version $(VERSION)

# Android(需要 Android Studio / SDK / Gradle wrapper)
android-debug:
	cd android && ./gradlew :app:assembleDebug
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/publish-release.ps1 -Platform android-debug -Version $(VERSION)

android-build:
	cd android && ./gradlew :app:assembleRelease
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/publish-release.ps1 -Platform android-release -Version $(VERSION)

android-clean:
	cd android && ./gradlew clean

publish-release:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/publish-release.ps1 -Platform all -Version $(VERSION)

# 源码 tarball(分发用,客户拿到后自行编译)
dist-src:
	rm -rf dist
	mkdir -p dist/taskflow-$(VERSION)
	cp -r server web windows android deploy dist/taskflow-$(VERSION)/
	cp README.md CHANGELOG.md Makefile dist/taskflow-$(VERSION)/ 2>/dev/null || true
	find dist/taskflow-$(VERSION) -type d \( -name node_modules -o -name target -o -name .gradle -o -name build -o -name dist \) -prune -exec rm -rf {} +
	cd dist && tar czf taskflow-$(VERSION)-src.tar.gz taskflow-$(VERSION)
	@echo "==> dist/taskflow-$(VERSION)-src.tar.gz"

# 构建 server + web 后打包成可部署 tarball(给 deploy/install.sh 吃)
dist: build
	rm -rf dist
	mkdir -p dist/taskflow-$(VERSION)
	cp server/taskflow-server dist/taskflow-$(VERSION)/ 2>/dev/null || true
	cp -r web/dist dist/taskflow-$(VERSION)/web
	cp -r deploy dist/taskflow-$(VERSION)/
	cp server/config.example.toml dist/taskflow-$(VERSION)/
	cp README.md CHANGELOG.md dist/taskflow-$(VERSION)/ 2>/dev/null || true
	cd dist && tar czf taskflow-$(VERSION).tar.gz taskflow-$(VERSION)
	@echo "==> dist/taskflow-$(VERSION).tar.gz"

clean:
	rm -rf dist
	$(MAKE) -C server clean
	rm -rf web/dist web/node_modules/.vite
	rm -rf windows/dist windows/src-tauri/target
	rm -rf android/app/build android/build android/.gradle
