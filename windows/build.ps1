# build.ps1 — TaskFlow Windows 客户端打包脚本(在 Windows 上执行)
#
# 用法:
#   1) cd taskflow\windows
#   2) Copy-Item .env.example .env  (按需修改 VITE_TASKFLOW_DEFAULT_SERVER 为你的公网域名)
#   3) powershell -ExecutionPolicy Bypass -File build.ps1
#
# 产物:
#   src-tauri\target\release\bundle\msi\TaskFlow_0.4.0_x64_zh-CN.msi
#   src-tauri\target\release\bundle\nsis\TaskFlow_0.4.0_x64-setup.exe
#
# 把 .msi / .exe 上传到你的服务器(例如 https://taskflow.teamcy.eu.cc/dl/)
# 即可让用户下载安装。

$ErrorActionPreference = 'Stop'

Set-Location -Path $PSScriptRoot

# 1) 加载 .env 到当前进程环境(不持久化)
$envFile = Join-Path $PSScriptRoot '.env'
if (Test-Path $envFile) {
    Write-Host "==> 加载 .env 中的环境变量"
    foreach ($line in Get-Content $envFile) {
        $trim = $line.Trim()
        if ($trim -eq '' -or $trim.StartsWith('#')) { continue }
        $idx = $trim.IndexOf('=')
        if ($idx -lt 1) { continue }
        $key = $trim.Substring(0, $idx).Trim()
        $val = $trim.Substring($idx + 1).Trim().Trim('"').Trim("'")
        Set-Item -Path "Env:$key" -Value $val
        Write-Host "    $key=$val"
    }
} else {
    Write-Warning "未找到 .env,使用 .env.example 中的默认值。"
    Write-Warning "建议:Copy-Item .env.example .env 并修改 VITE_TASKFLOW_DEFAULT_SERVER 为你的公网域名。"
}

# 2) 检查 ../web 与 windows 各自 npm install
if (-not (Test-Path '..\web\node_modules')) {
    Write-Host "==> 安装 web 依赖"
    Push-Location '..\web'
    npm install
    Pop-Location
}
if (-not (Test-Path 'node_modules')) {
    Write-Host "==> 安装 windows 依赖"
    npm install
}

# 3) 检查 Rust toolchain
if (-not (Get-Command cargo -ErrorAction SilentlyContinue)) {
    Write-Error "未检测到 Rust。请先安装:https://rustup.rs/"
    exit 1
}

# 4) 跑 tauri build
Write-Host "==> 开始打包(这一步会比较慢,首次约 5-15 分钟)"
npm run tauri:build

# 5) 列出产物
$bundleDir = 'src-tauri\target\release\bundle'
if (Test-Path $bundleDir) {
    Write-Host ""
    Write-Host "==> 打包完成,产物在:" -ForegroundColor Green
    Get-ChildItem -Path $bundleDir -Recurse -Include *.msi,*.exe |
        ForEach-Object { Write-Host "    $($_.FullName)" -ForegroundColor Cyan }
} else {
    Write-Warning "未找到 bundle 目录,打包可能失败。请翻看上方日志。"
}
