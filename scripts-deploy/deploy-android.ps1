# deploy-android.ps1 — Android 客户端 (Kotlin / Compose) 编译/打包
#
# 干什么:
#   1) 检查 JDK 17 / ANDROID_HOME / SDK 平台
#   2) 引导 gradle-wrapper.jar (项目里没带, 必须先 bootstrap)
#   3) 写 local.properties (sdk.dir + 默认服务器地址)
#   4) ./gradlew :app:assembleRelease 或 :app:assembleDebug
#
# 用法:
#   # Debug APK (无需签名, 装机即用, 适合测试)
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-android.ps1
#
#   # Release APK (需签名 keystore, 见下面参数)
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-android.ps1 -Release
#
#   # 指定后端服务器地址 (烧进 BuildConfig.DEFAULT_SERVER_URL, 用户首次启动默认连这个)
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-android.ps1 `
#       -ServerUrl 'https://taskflow.example.com'
#
#   # Release + 签名
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-android.ps1 -Release `
#       -KeystorePath 'C:\keys\taskflow.jks' `
#       -KeystorePassword 'xxx' -KeyAlias 'taskflow' -KeyPassword 'xxx'
#
#   # 清理后重新构建 (有时候 Gradle 缓存会卡)
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-android.ps1 -Clean
#
# 产物:
#   android\app\build\outputs\apk\debug\TaskFlow-debug.apk
#   android\app\build\outputs\apk\release\TaskFlow-release.apk
#
# 报错排查 (按出现频率排序):
#   - "Could not find or load main class org.gradle.wrapper.GradleWrapperMain"
#       → gradle-wrapper.jar 缺失, 本脚本会自动下载
#   - "SDK location not found"
#       → 没装 Android SDK 或 ANDROID_HOME 没设, 脚本会引导
#   - "Failed to apply plugin 'com.android.application'"
#       → AGP 版本要 JDK 17 (不是 11 也不是 21)
#   - "compileSdk = 35 ... not installed"
#       → 用 sdkmanager 装: sdkmanager "platforms;android-35" "build-tools;35.0.0"
#   - 国内下载 Gradle 慢/卡
#       → 脚本支持 -GradleMirror 切清华镜像

[CmdletBinding()]
param(
    [string]$ServerUrl,
    [switch]$Release,
    [switch]$Clean,
    [switch]$SkipChecks,
    [switch]$GradleMirror,           # 用清华镜像下载 Gradle 发行版
    [string]$KeystorePath,           # Release 签名 (4 个一起给, 否则 Release 出 unsigned APK)
    [string]$KeystorePassword,
    [string]$KeyAlias,
    [string]$KeyPassword
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '_common.ps1')

Show-Diagnostic

$androidDir = Join-Path $script:RepoRoot 'android'

if (-not (Test-Path $androidDir)) {
    Write-Fail "找不到 android 目录: $androidDir" '请确认你在 TaskFlow 仓库根目录'
}

# ====== 1. 环境检查 ======
if (-not $SkipChecks) {
    Write-Step "检查 JDK"

    $javaCmd = Get-Command java -ErrorAction SilentlyContinue
    if (-not $javaCmd) {
        Write-Fail "缺少 Java" @'
装 JDK 17 (推荐 Temurin):
  https://adoptium.net/zh-CN/temurin/releases/?version=17
装好后设 JAVA_HOME 指向 JDK 安装目录, 并把 %JAVA_HOME%\bin 加到 PATH.
'@
    }
    $javaVerOutput = & java -version 2>&1 | Out-String
    Write-Ok "java: $($javaVerOutput.Split([Environment]::NewLine)[0])"

    # 解析版本 (java 17 输出 "openjdk version \"17.0.x\"")
    if ($javaVerOutput -match '"(\d+)\.') {
        $javaMajor = [int]$Matches[1]
        if ($javaMajor -lt 17) {
            Write-Fail "JDK 版本过低 (需要 17, 当前 $javaMajor)" `
                'AGP 8.7.x 强制要求 JDK 17. 装 https://adoptium.net/zh-CN/temurin/releases/?version=17'
        }
        if ($javaMajor -gt 21) {
            Write-Warn2 "JDK $javaMajor 可能太新, AGP 8.7 官方支持到 21. 出问题就降到 17."
        }
    }

    Write-Step "检查 Android SDK"
    $sdkRoot = $env:ANDROID_HOME
    if (-not $sdkRoot) { $sdkRoot = $env:ANDROID_SDK_ROOT }
    if (-not $sdkRoot) {
        # 尝试常见位置
        $candidates = @(
            "$env:LOCALAPPDATA\Android\Sdk",
            "$env:USERPROFILE\AppData\Local\Android\Sdk",
            "C:\Android\Sdk"
        )
        foreach ($c in $candidates) {
            if (Test-Path $c) { $sdkRoot = $c; break }
        }
    }
    if (-not $sdkRoot -or -not (Test-Path $sdkRoot)) {
        Write-Fail "找不到 Android SDK" @'
装 Android Studio (含 SDK):
  https://developer.android.com/studio
装好后 SDK 默认在 %LOCALAPPDATA%\Android\Sdk
然后设环境变量: setx ANDROID_HOME "%LOCALAPPDATA%\Android\Sdk"
重开 PowerShell 后再跑此脚本.
'@
    }
    Write-Ok "Android SDK: $sdkRoot"

    # 检查 platform 35 是否装了
    $platform35 = Join-Path $sdkRoot 'platforms\android-35'
    if (-not (Test-Path $platform35)) {
        Write-Warn2 "未装 platforms\android-35 (compileSdk=35 必须). Gradle 会尝试自动装,"
        Write-Warn2 "失败时手动装: sdkmanager `"platforms;android-35`" `"build-tools;35.0.0`""
    } else {
        Write-Ok "platforms\android-35 已装"
    }

    # 写 local.properties (sdk.dir)
    $localPropsPath = Join-Path $androidDir 'local.properties'
    $sdkPathEscaped = $sdkRoot -replace '\\','\\' -replace ':','\:'
    $localPropsLines = @()
    if (Test-Path $localPropsPath) {
        $localPropsLines = Get-Content $localPropsPath
    }
    $hasSdkDir = $localPropsLines | Where-Object { $_ -match '^\s*sdk\.dir\s*=' }
    if (-not $hasSdkDir) {
        Add-Content -Path $localPropsPath -Value "sdk.dir=$sdkPathEscaped"
        Write-Ok "已写 sdk.dir 到 local.properties"
    }
}

# ====== 2. 写 local.properties 的 server url ======
if ($ServerUrl) {
    Write-Step "写默认服务器地址 (烧进 BuildConfig.DEFAULT_SERVER_URL)"
    $localPropsPath = Join-Path $androidDir 'local.properties'
    $existing = if (Test-Path $localPropsPath) { Get-Content $localPropsPath } else { @() }
    $filtered = $existing | Where-Object { $_ -notmatch '^\s*taskflow\.default\.server\.url\s*=' }
    $filtered = @($filtered) + "taskflow.default.server.url=$ServerUrl"
    Set-Content -Path $localPropsPath -Value $filtered -Encoding UTF8
    Write-Ok "默认服务器地址: $ServerUrl"
}

# ====== 3. Bootstrap gradle-wrapper.jar ======
Write-Step "检查 gradle-wrapper.jar"
$wrapperJar = Join-Path $androidDir 'gradle\wrapper\gradle-wrapper.jar'
if (-not (Test-Path $wrapperJar)) {
    Write-Warn2 "gradle-wrapper.jar 缺失, 自动引导 (~145MB 一次性)"

    Push-Location $androidDir
    try {
        # 优先用本机 gradle (如果有)
        $gradleCmd = Get-Command gradle -ErrorAction SilentlyContinue
        if ($gradleCmd) {
            Write-Host "    > 检测到本机 gradle: $($gradleCmd.Source)"
            & gradle wrapper --gradle-version 8.10.2 --distribution-type bin
            if ($LASTEXITCODE -ne 0 -or -not (Test-Path $wrapperJar)) {
                Write-Warn2 "gradle wrapper 命令没成功, 走下载方案"
            }
        }

        # 没成功就下载
        if (-not (Test-Path $wrapperJar)) {
            $gradleVersion = '8.10.2'
            $tmpDir = Join-Path $env:TEMP "taskflow-gradle-bootstrap-$gradleVersion"
            $zipFile = Join-Path $tmpDir "gradle-$gradleVersion-bin.zip"

            New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null

            if (-not (Test-Path $zipFile)) {
                $zipUrl = if ($GradleMirror) {
                    "https://mirrors.tuna.tsinghua.edu.cn/gradle/distributions/v$gradleVersion/gradle-$gradleVersion-bin.zip"
                } else {
                    "https://services.gradle.org/distributions/gradle-$gradleVersion-bin.zip"
                }
                Write-Host "    > 下载 $zipUrl"
                $oldProgress = $ProgressPreference
                $ProgressPreference = 'SilentlyContinue'
                try {
                    Invoke-WebRequest -Uri $zipUrl -OutFile $zipFile -UseBasicParsing
                } catch {
                    $hint = if ($GradleMirror) {
                        '清华镜像也下载失败. 检查网络或手动下载放到: ' + $zipFile
                    } else {
                        '下载失败. 加 -GradleMirror 走清华源, 或手动下载放到: ' + $zipFile
                    }
                    Write-Fail "下载 Gradle 失败: $_" $hint
                } finally {
                    $ProgressPreference = $oldProgress
                }
            }

            Write-Host "    > 解压 gradle-wrapper.jar"
            Add-Type -AssemblyName System.IO.Compression.FileSystem
            $zip = [System.IO.Compression.ZipFile]::OpenRead($zipFile)
            try {
                $entry = $zip.Entries | Where-Object {
                    $_.Name -like 'gradle-wrapper-*.jar'
                } | Select-Object -First 1
                if (-not $entry) {
                    Write-Fail "在 zip 中找不到 gradle-wrapper jar" '建议改用 Android Studio 打开 android 目录'
                }
                New-Item -ItemType Directory -Force -Path (Split-Path $wrapperJar) | Out-Null
                [System.IO.Compression.ZipFileExtensions]::ExtractToFile($entry, $wrapperJar, $true)
            } finally {
                $zip.Dispose()
            }
        }
    } finally {
        Pop-Location
    }

    if (Test-Path $wrapperJar) {
        Write-Ok "gradle-wrapper.jar 就绪"
    } else {
        Write-Fail 'gradle-wrapper.jar 引导失败' '建议用 Android Studio 打开 android\ 目录, IDE 会自动 sync'
    }
} else {
    Write-Ok "gradle-wrapper.jar 已存在"
}

# ====== 4. 构建 ======
Push-Location $androidDir
try {
    $gradlew = Join-Path $androidDir 'gradlew.bat'
    if (-not (Test-Path $gradlew)) {
        Write-Fail "找不到 gradlew.bat" '项目结构可能损坏, 重新解压源码'
    }

    if ($Clean) {
        Write-Step "清理上次构建"
        Invoke-Native -Description 'gradlew clean' -Action {
            & $gradlew clean --no-daemon
        }
    }

    # Release 签名参数
    $gradleExtraArgs = @()
    if ($Release -and $KeystorePath) {
        if (-not (Test-Path $KeystorePath)) {
            Write-Fail "Keystore 文件不存在: $KeystorePath"
        }
        if (-not $KeystorePassword -or -not $KeyAlias -or -not $KeyPassword) {
            Write-Fail "Release 签名需要同时提供 -KeystorePath -KeystorePassword -KeyAlias -KeyPassword"
        }
        # 通过 -P 传, 让 Gradle 在 build.gradle 里读 (项目可能没接, 仅作环境注入)
        $env:TASKFLOW_KEYSTORE_PATH = (Resolve-Path $KeystorePath).Path
        $env:TASKFLOW_KEYSTORE_PASSWORD = $KeystorePassword
        $env:TASKFLOW_KEY_ALIAS = $KeyAlias
        $env:TASKFLOW_KEY_PASSWORD = $KeyPassword
        Write-Ok "签名参数已注入到环境变量 (TASKFLOW_KEYSTORE_*)"
    }

    if ($Release) {
        Write-Step "构建 Release APK"
        Invoke-Native -Description 'gradlew :app:assembleRelease' -Action {
            & $gradlew ':app:assembleRelease' --no-daemon @gradleExtraArgs
        } -Hint @'
常见问题:
  * "compileSdk = 35 not installed" → sdkmanager "platforms;android-35"
  * "Could not resolve" → 网络问题, 设代理或 Gradle 镜像 (gradle-wrapper.properties)
  * 内存不足 → 改 android/gradle.properties: org.gradle.jvmargs=-Xmx4096m
'@
    } else {
        Write-Step "构建 Debug APK"
        Invoke-Native -Description 'gradlew :app:assembleDebug' -Action {
            & $gradlew ':app:assembleDebug' --no-daemon
        } -Hint @'
常见问题:
  * "Could not find or load main class GradleWrapperMain" → wrapper jar 损坏, 删 gradle/wrapper/gradle-wrapper.jar 后重跑
  * KSP 编译错 → 删 .gradle 缓存重来: gradlew --stop; Remove-Item .gradle -Recurse -Force
'@
    }

    # 列出产物
    $apkRoot = Join-Path $androidDir 'app\build\outputs\apk'
    if (Test-Path $apkRoot) {
        Write-Host ""
        Write-Host "==> 构建完成, 产物:" -ForegroundColor Green
        Get-ChildItem -Path $apkRoot -Recurse -Filter '*.apk' | ForEach-Object {
            $sizeMB = [math]::Round($_.Length / 1MB, 1)
            Write-Host "    [$sizeMB MB] $($_.FullName)" -ForegroundColor Cyan
        }
    } else {
        Write-Fail "找不到 APK 输出目录: $apkRoot" '构建可能失败, 翻上方日志'
    }
} finally {
    Pop-Location
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host " Android 客户端就绪" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host " 安装到设备: adb install -r 上面的 .apk 路径"
Write-Host " 或上传到下载页让用户扫码下载"
Write-Host ""
