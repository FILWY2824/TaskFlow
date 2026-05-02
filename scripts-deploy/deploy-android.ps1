# deploy-android.ps1 — Android 客户端 (Kotlin / Compose) 编译/打包
#
# 干什么:
#   1) 检查 JDK 17 / ANDROID_HOME / SDK 平台
#   2) 校验仓库根 .env(三端共用 PUBLIC_BASE_URL,作为出厂默认服务端 URL)
#   3) ./gradlew :app:assembleRelease 或 :app:assembleDebug
#
# 用法:
#   # Debug APK (无需签名, 装机即用, 适合测试)
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-android.ps1
#
#   # Release APK (需签名 keystore, 见下面参数)
#   powershell -ExecutionPolicy Bypass -File scripts-deploy\deploy-android.ps1 -Release
#
#   # 命令行临时覆盖默认服务器地址(优先级高于 .env 的 PUBLIC_BASE_URL)
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
# 注意:
#   * gradle-wrapper.jar 已经在仓库里(android/gradle/wrapper/),不需要任何 bootstrap。
#     如果它不见了,说明 git checkout 不完整,本脚本会直接报错。
#   * 默认服务器地址来自仓库根 .env 的 PUBLIC_BASE_URL,与 server / web / windows 共用一份。
#     不再有 android\local.properties 里 taskflow.default.server.url 这种局部覆盖入口。
#
# 报错排查:
#   - "SDK location not found"       → 没装 Android SDK 或 ANDROID_HOME 没设
#   - "compileSdk = 35 ... not installed"
#       → sdkmanager "platforms;android-35" "build-tools;35.0.0"
#   - 国内 Gradle 下载慢 → -GradleMirror 切清华镜像

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

# ====== 2. 读根目录 .env 拿 PUBLIC_BASE_URL,作为客户端出厂默认服务端 URL ======
# 三端共用根 .env;android/app/build.gradle.kts 也会读 .env 里的 PUBLIC_BASE_URL,
# 这里再 export 一份到环境变量是为了让 -ServerUrl 命令行覆盖也能生效。
Write-Step "解析根目录 .env"

$rootEnvFile = Join-Path $script:RepoRoot '.env'
$rootEnvExample = Join-Path $script:RepoRoot '.env.example'

if (-not (Test-Path $rootEnvFile)) {
    if (Test-Path $rootEnvExample) {
        Copy-Item $rootEnvExample $rootEnvFile
        Write-Warn2 "从 .env.example 复制了一份 .env;请确认 PUBLIC_BASE_URL 已填好。"
    } else {
        Write-Fail "找不到根目录 .env" "请先 cp .env.example .env 并修改 PUBLIC_BASE_URL"
    }
}
Import-DotEnv $rootEnvFile | Out-Null

# 命令行 -ServerUrl 优先级最高(本次构建生效,不写 .env)
if ($ServerUrl) {
    Write-Step "命令行覆盖默认服务器地址 = $ServerUrl"
    $env:TASKFLOW_DEFAULT_SERVER_URL = $ServerUrl
} elseif ($env:PUBLIC_BASE_URL) {
    $env:TASKFLOW_DEFAULT_SERVER_URL = $env:PUBLIC_BASE_URL
    Write-Ok "TASKFLOW_DEFAULT_SERVER_URL = $($env:TASKFLOW_DEFAULT_SERVER_URL)(取自 .env 的 PUBLIC_BASE_URL)"
} else {
    Write-Warn2 "根目录 .env 没设 PUBLIC_BASE_URL;客户端会用 build.gradle.kts 的兜底值。"
}

# ====== 3. gradle-wrapper.jar 检查(项目里已带,正常情况下不会缺) ======
Write-Step "检查 gradle-wrapper.jar"
$wrapperJar = Join-Path $androidDir 'gradle\wrapper\gradle-wrapper.jar'
if (-not (Test-Path $wrapperJar)) {
    Write-Fail "缺少 $wrapperJar" @'
项目仓库自带 gradle-wrapper.jar(48 KB)。如果它不见了,通常意味着 git checkout
不完整,或者是被某些"清理空目录"脚本误删了。请重新 clone 或从最近一次 commit 恢复:
  git checkout HEAD -- android/gradle/wrapper/gradle-wrapper.jar
'@
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
