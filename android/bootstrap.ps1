# bootstrap.ps1 — Windows 上一键引导 gradle-wrapper.jar
#
# 这个脚本解决"项目里没有 gradle-wrapper.jar 导致 ./gradlew 不能跑"的问题。
# 思路:从官方 Gradle 8.10.2 发行包里提取 gradle-wrapper.jar,放到正确位置。
#
# 用法:
#   cd taskflow\android
#   powershell -ExecutionPolicy Bypass -File bootstrap.ps1
#
# 完成后即可:
#   .\gradlew :app:assembleRelease
# 或者用 Android Studio 打开 android\ 目录(IDE 会自动 sync)。

$ErrorActionPreference = 'Stop'
Set-Location -Path $PSScriptRoot

$wrapperJar = 'gradle\wrapper\gradle-wrapper.jar'
if (Test-Path $wrapperJar) {
    Write-Host "==> $wrapperJar 已存在,无需重复 bootstrap" -ForegroundColor Green
    exit 0
}

# 推荐做法:有 Android Studio 的人直接用它打开,IDE 会自动生成 wrapper。
$gradleCmd = Get-Command gradle -ErrorAction SilentlyContinue
if ($gradleCmd) {
    Write-Host "==> 检测到本机 gradle: $($gradleCmd.Source)"
    Write-Host "==> 用 'gradle wrapper' 命令生成 wrapper jar"
    & $gradleCmd.Source wrapper --gradle-version 8.10.2 --distribution-type bin
    if (Test-Path $wrapperJar) {
        Write-Host "==> wrapper 就绪,接下来:" -ForegroundColor Green
        Write-Host "    .\gradlew :app:assembleRelease" -ForegroundColor Cyan
        exit 0
    } else {
        Write-Warning "gradle wrapper 跑完了但没生成 jar,继续走下载方案。"
    }
}

# 兜底方案:从 Gradle 官方 zip 解 wrapper jar 出来。
$gradleVersion = '8.10.2'
$tmpDir = Join-Path $env:TEMP "taskflow-gradle-bootstrap-$gradleVersion"
$zipFile = Join-Path $tmpDir "gradle-$gradleVersion-bin.zip"
$zipUrl = "https://services.gradle.org/distributions/gradle-$gradleVersion-bin.zip"

New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null

if (-not (Test-Path $zipFile)) {
    Write-Host "==> 下载 Gradle $gradleVersion 发行包(~145MB,只下一次)..."
    try {
        # 比 Invoke-WebRequest 快,且不卡进度条
        $oldProgressPreference = $ProgressPreference
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $zipUrl -OutFile $zipFile -UseBasicParsing
        $ProgressPreference = $oldProgressPreference
    } catch {
        Write-Error "下载失败:$_`n请检查网络,或手动下载:$zipUrl 然后放到 $zipFile 后重跑此脚本。"
        exit 1
    }
}

Write-Host "==> 从发行包中提取 gradle-wrapper.jar"
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [System.IO.Compression.ZipFile]::OpenRead($zipFile)
try {
    $entry = $zip.Entries | Where-Object {
        $_.FullName -like "*gradle-$gradleVersion/lib/plugins/gradle-wrapper-*.jar" -or
        $_.FullName -like "*gradle-$gradleVersion/lib/gradle-wrapper-*.jar"
    } | Select-Object -First 1
    if (-not $entry) {
        # 不同版本路径不同,fallback:任何 gradle-wrapper-*.jar
        $entry = $zip.Entries | Where-Object { $_.Name -like 'gradle-wrapper-*.jar' } | Select-Object -First 1
    }
    if (-not $entry) {
        throw "在 $zipFile 中找不到 gradle-wrapper jar。请尝试用 Android Studio 打开 android\ 目录让 IDE 自动 sync。"
    }
    New-Item -ItemType Directory -Force -Path 'gradle\wrapper' | Out-Null
    [System.IO.Compression.ZipFileExtensions]::ExtractToFile($entry, $wrapperJar, $true)
} finally {
    $zip.Dispose()
}

if (Test-Path $wrapperJar) {
    Write-Host "==> wrapper 就绪,接下来:" -ForegroundColor Green
    Write-Host "    .\gradlew :app:assembleRelease" -ForegroundColor Cyan
    Write-Host "或用 Android Studio 直接打开 android\ 目录。" -ForegroundColor Cyan
} else {
    Write-Error "wrapper jar 提取失败,请尝试用 Android Studio 打开 android\ 目录。"
    exit 1
}
