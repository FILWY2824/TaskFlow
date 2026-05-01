# deploy-web.ps1 — Web 前端 + Go 后端 一键编译/启动
#
# 干什么:
#   1) 检查 Node.js / Go / npm 是否齐全
#   2) cd web && npm install && npm run build  → 产出 web/dist
#   3) cd server && go build                   → 产出 server/taskflow-server.exe
#   4) 可选:启动后端 + 前端 dev server (开发模式)
#
# 用法:
#   # 仅生产构建 (产出可部署的 web/dist + server 二进制)
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-web.ps1
#
#   # 跳过类型检查 (vue-tsc 慢且偶发误报)
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-web.ps1 -SkipTypeCheck
#
#   # 开发模式: 启动后端 8080 + 前端 dev 3003 (Ctrl+C 同时停)
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-web.ps1 -Dev
#
#   # 只构建 server 不构建 web
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-web.ps1 -ServerOnly
#
#   # 只构建 web 不构建 server
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-web.ps1 -WebOnly
#
# 产物:
#   web/dist/                      ← 上传到 nginx 静态目录
#   server/taskflow-server.exe     ← 本机运行,或交叉编译见 -BuildLinux
#
# 报错排查:
#   - 报 "缺少 xxx" → 按提示装 Node 20+ / Go 1.22+
#   - npm install 卡住 → 设国内镜像: npm config set registry https://registry.npmmirror.com
#   - go build 报 modernc.org/sqlite → 这是纯 Go 实现的 sqlite, 不需要 CGO, 直接 go mod tidy 重试
#   - vue-tsc 报类型错误 → 加 -SkipTypeCheck 先跳过, 后续在编辑器里慢慢修

[CmdletBinding()]
param(
    [switch]$Dev,
    [switch]$SkipTypeCheck,
    [switch]$ServerOnly,
    [switch]$WebOnly,
    [switch]$BuildLinux,        # 交叉编译 Linux amd64 (用于上传到 VPS)
    [int]$ServerPort = 8080,
    [int]$WebPort = 3003
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '_common.ps1')

Show-Diagnostic

# ====== 1. 环境检查 ======
Write-Step "检查环境依赖"

if (-not $ServerOnly) {
    Test-Command -Name 'node' -VersionArg '--version' `
        -InstallHint '请安装 Node.js 20+ : https://nodejs.org/zh-cn/download/'
    $nodeVer = (& node --version) -replace '^v',''
    $major = [int]($nodeVer.Split('.')[0])
    if ($major -lt 18) {
        Write-Fail "Node.js 版本太低 ($nodeVer), 需要 18+ (推荐 20+)" `
            '到 https://nodejs.org 下载 LTS 版本'
    }
    Test-Command -Name 'npm' -VersionArg '--version'
}

if (-not $WebOnly) {
    Test-Command -Name 'go' -VersionArg 'version' `
        -InstallHint '请安装 Go 1.22+ : https://go.dev/dl/'
}

# ====== 2. 构建 Server ======
if (-not $WebOnly) {
    Write-Step "构建 Go 后端"
    Push-Location (Join-Path $script:RepoRoot 'server')
    try {
        # go mod tidy 一定要跑, 否则 go.sum 缺项目会报 missing go.sum entry
        Invoke-Native -Description 'go mod tidy' -Action {
            & go mod tidy
        } -Hint '检查网络; 国内可设 $env:GOPROXY = "https://goproxy.cn,direct"'

        if ($BuildLinux) {
            Write-Host "    > 交叉编译 Linux amd64 (用于上传到 VPS)"
            $env:CGO_ENABLED = '0'
            $env:GOOS = 'linux'
            $env:GOARCH = 'amd64'
            Invoke-Native -Description 'go build linux/amd64' -Action {
                & go build -trimpath -ldflags='-s -w -X main.version=0.4.0' `
                    -o taskflow-server-linux-amd64 ./cmd/server
            }
            Remove-Item Env:CGO_ENABLED, Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
            Write-Ok "产物: server\taskflow-server-linux-amd64 (拷到 VPS 用)"
        } else {
            $env:CGO_ENABLED = '0'
            Invoke-Native -Description 'go build (Windows)' -Action {
                & go build -trimpath -ldflags='-s -w -X main.version=0.4.0' `
                    -o taskflow-server.exe ./cmd/server
            }
            Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
            Write-Ok "产物: server\taskflow-server.exe"
        }
    } finally {
        Pop-Location
    }
}

# ====== 3. 构建 Web ======
if (-not $ServerOnly) {
    Write-Step "构建 Vue 3 前端"
    Push-Location (Join-Path $script:RepoRoot 'web')
    try {
        if (-not (Test-Path 'node_modules')) {
            Invoke-Native -Description 'npm install (首次安装, 慢)' -Action {
                & npm install
            } -Hint '失败时设国内镜像: npm config set registry https://registry.npmmirror.com'
        } else {
            Write-Ok 'node_modules 已存在, 跳过 install (要重装就先删 node_modules)'
        }

        if ($Dev) {
            # Dev 模式不构建,后面会启动 dev server
            Write-Ok 'Dev 模式跳过生产构建'
        } else {
            if ($SkipTypeCheck) {
                Invoke-Native -Description 'vite build (跳过类型检查)' -Action {
                    & npx vite build
                }
            } else {
                # web/package.json 里 build = "vue-tsc --noEmit && vite build"
                Invoke-Native -Description 'vue-tsc + vite build' -Action {
                    & npm run build
                } -Hint '类型检查报错可加 -SkipTypeCheck 先跳过'
            }
            Write-Ok '产物: web\dist\  (上传到 nginx 静态目录, 或被 docker-compose 打包)'
        }
    } finally {
        Pop-Location
    }
}

# ====== 4. Dev 模式: 同时起后端和前端 ======
if ($Dev) {
    Write-Step "Dev 模式: 同时启动后端 ($ServerPort) 和前端 ($WebPort)"

    $serverDir = Join-Path $script:RepoRoot 'server'
    $webDir    = Join-Path $script:RepoRoot 'web'

    # 检查 server config
    $cfgPath = Join-Path $serverDir 'config.toml'
    if (-not (Test-Path $cfgPath)) {
        $examplePath = Join-Path $serverDir 'config.example.toml'
        if (Test-Path $examplePath) {
            Copy-Item $examplePath $cfgPath
            Write-Warn2 "已从 config.example.toml 复制出 config.toml, 请按需修改 (尤其 jwt_secret)"
        }
    }

    # 后端用 Start-Process 后台跑
    Write-Host "    > 启动后端 (新窗口)"
    $serverProc = Start-Process -FilePath 'go' `
        -ArgumentList 'run','./cmd/server','-config','config.toml' `
        -WorkingDirectory $serverDir `
        -PassThru `
        -WindowStyle Normal

    Start-Sleep -Seconds 2

    # npm 在 Windows 上其实是 npm.cmd, 直接 Start-Process 'npm' 会找不到, 用 cmd /c 兜底
    Write-Host "    > 启动前端 (新窗口, http://127.0.0.1:$WebPort)"
    $npmCmd = (Get-Command npm).Source
    $webProc = Start-Process -FilePath 'cmd' `
        -ArgumentList '/c', "`"$npmCmd`"", 'run', 'dev' `
        -WorkingDirectory $webDir `
        -PassThru `
        -WindowStyle Normal

    Write-Host ""
    Write-Host "==> Dev 服务已启动:" -ForegroundColor Green
    Write-Host "    后端: http://127.0.0.1:$ServerPort  (PID $($serverProc.Id))"
    Write-Host "    前端: http://127.0.0.1:$WebPort     (PID $($webProc.Id))"
    Write-Host ""
    Write-Host "按 Ctrl+C 退出本脚本 (后台进程会一起结束)" -ForegroundColor Yellow

    try {
        # 阻塞等待用户中断
        while ($true) {
            Start-Sleep -Seconds 1
            if ($serverProc.HasExited) {
                Write-Warn2 "后端已退出 (exit code $($serverProc.ExitCode))"
                break
            }
            if ($webProc.HasExited) {
                Write-Warn2 "前端已退出 (exit code $($webProc.ExitCode))"
                break
            }
        }
    } finally {
        Write-Host ""
        Write-Step "停止后台进程"
        if (-not $serverProc.HasExited) { Stop-Process -Id $serverProc.Id -Force -ErrorAction SilentlyContinue }
        if (-not $webProc.HasExited)    { Stop-Process -Id $webProc.Id    -Force -ErrorAction SilentlyContinue }
    }
} else {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host " 构建完成" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    if (-not $WebOnly) {
        if ($BuildLinux) {
            Write-Host " Linux 后端: server\taskflow-server-linux-amd64"
        } else {
            Write-Host " Windows 后端: server\taskflow-server.exe"
            Write-Host "   运行: cd server; .\taskflow-server.exe -config config.toml"
        }
    }
    if (-not $ServerOnly) {
        Write-Host " Web 静态文件: web\dist\  (上传到 nginx 即可)"
    }
    Write-Host ""
    Write-Host " 下次想边改边看: -Dev 切到开发模式" -ForegroundColor DarkGray
    Write-Host ""
}
