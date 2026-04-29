#!/usr/bin/env bash
# bootstrap.sh — 一次性下载 gradle-wrapper.jar,之后就可以用 ./gradlew 了。
#
# 仅需在第一次解压源码后运行。运行前提:已经在系统装过任一版本的 gradle(用于
# 触发 `gradle wrapper` 任务来生成本仓库需要的 wrapper jar)。
#
# 用法:
#   cd android
#   ./bootstrap.sh
#
# 替代方案:用 Android Studio 打开本目录,IDE 自带的 wrapper 检查会自动下载。

set -euo pipefail

cd "$(dirname "$0")"

if [[ -f gradle/wrapper/gradle-wrapper.jar ]]; then
    echo "==> gradle-wrapper.jar 已存在,无需重复 bootstrap"
    exit 0
fi

if ! command -v gradle >/dev/null 2>&1; then
    cat <<'EOF' >&2
ERROR: 找不到 gradle 命令。

bootstrap 需要先有任一版本的 Gradle(可以是 SDKMAN! 装的、apt 装的、Android Studio 自带的)。
两种解决:
  1. 装 SDKMAN! 后:  sdk install gradle
  2. 用 Android Studio:打开 android/ 目录,IDE 会自动 sync 并生成 wrapper。
EOF
    exit 1
fi

echo "==> 用本地 gradle 生成 wrapper jar"
gradle wrapper --gradle-version 8.10.2 --distribution-type bin

if [[ -f gradle/wrapper/gradle-wrapper.jar ]]; then
    echo "==> wrapper 就绪,接下来:"
    echo "    ./gradlew :app:assembleDebug"
fi
