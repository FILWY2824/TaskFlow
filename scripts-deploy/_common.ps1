# _common.ps1 — 公共函数与环境检查
# 被 deploy-web.ps1 / deploy-windows.ps1 / deploy-android.ps1 dot-source。
# 不要单独执行。

$script:RepoRoot = Split-Path -Parent $PSScriptRoot

function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Write-Ok {
    param([string]$Message)
    Write-Host "    [OK] $Message" -ForegroundColor Green
}

function Write-Warn2 {
    param([string]$Message)
    Write-Host "    [WARN] $Message" -ForegroundColor Yellow
}

function Write-Fail {
    param([string]$Message, [string]$Hint = '')
    Write-Host ""
    Write-Host "[X] $Message" -ForegroundColor Red
    if ($Hint) {
        Write-Host "    >> $Hint" -ForegroundColor Yellow
    }
    Write-Host ""
    exit 1
}

# 检查命令是否存在,顺便打印版本
function Test-Command {
    param(
        [Parameter(Mandatory=$true)][string]$Name,
        [string]$VersionArg = '--version',
        [string]$InstallHint
    )
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $cmd) {
        Write-Fail "缺少 $Name" $InstallHint
    }
    try {
        # 拆参数交给 splatting,即便只有一个参数也安全
        $argList = @($VersionArg.Split(' ', [System.StringSplitOptions]::RemoveEmptyEntries))
        $verOutput = & $Name @argList 2>&1 | Select-Object -First 1
        Write-Ok "$Name 已安装: $verOutput"
    } catch {
        Write-Ok "$Name 已安装: $($cmd.Source)"
    }
}

# 安全执行外部命令,失败时打印明确错误
function Invoke-Native {
    param(
        [Parameter(Mandatory=$true)][string]$Description,
        [Parameter(Mandatory=$true)][scriptblock]$Action,
        [string]$Hint
    )
    Write-Host "    > $Description" -ForegroundColor DarkGray
    & $Action
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "$Description 失败 (exit code: $LASTEXITCODE)" $Hint
    }
}

# 加载 .env 文件到当前进程环境(不持久化)
function Import-DotEnv {
    param([Parameter(Mandatory=$true)][string]$Path)
    if (-not (Test-Path $Path)) {
        Write-Warn2 "$Path 不存在,跳过 env 加载"
        return @{}
    }
    Write-Host "    > 加载 $Path"
    $loaded = @{}
    foreach ($line in Get-Content $Path) {
        $trim = $line.Trim()
        if ($trim -eq '' -or $trim.StartsWith('#')) { continue }
        $idx = $trim.IndexOf('=')
        if ($idx -lt 1) { continue }
        $key = $trim.Substring(0, $idx).Trim()
        $val = $trim.Substring($idx + 1).Trim().Trim('"').Trim("'")
        Set-Item -Path "Env:$key" -Value $val
        $loaded[$key] = $val
        Write-Host "      $key=$val" -ForegroundColor DarkGray
    }
    return $loaded
}

# 打印一行版本信息(用于诊断)
function Show-Diagnostic {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor DarkGray
    Write-Host " 环境诊断信息" -ForegroundColor DarkGray
    Write-Host "========================================" -ForegroundColor DarkGray
    Write-Host " PowerShell : $($PSVersionTable.PSVersion)"
    Write-Host " OS         : $([System.Environment]::OSVersion.VersionString)"
    Write-Host " Repo Root  : $script:RepoRoot"
    Write-Host " Working Dir: $(Get-Location)"
    Write-Host "========================================" -ForegroundColor DarkGray
}
