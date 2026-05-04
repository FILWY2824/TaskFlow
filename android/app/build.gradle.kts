// app/build.gradle.kts — 主模块
//
// 关键决定:
//   - 自管 DI(单例 AppContainer),不用 Hilt,减少注解处理开销与 incremental 复杂度。
//   - Room 用 KSP(KAPT 已不推荐)。
//   - Moshi codegen + Kotlin reflection 0 用,所有 DTO 都标注 @JsonClass(generateAdapter = true)。
//   - Compose 插件由 Kotlin 2.0 直接提供,不再依赖独立 compiler 版本对齐。
//   - DEFAULT_SERVER_URL 由仓库根目录 .env 的 PUBLIC_BASE_URL 烧入,与 server / web / windows 共用一份配置。

import java.io.File
import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.ksp)
}

/**
 * 解析仓库根目录 ../.env(android/ 的上一级)。
 *
 * 文件不存在或字段不存在都不算错 —— 返回 null,由调用方 fallback。
 * 不是完整的 dotenv 解析器:足够覆盖 KEY=VALUE / KEY="VALUE" / 注释行 / 空行。
 */
fun parseRootDotEnv(key: String): String? {
    val repoRoot = rootProject.projectDir.parentFile ?: rootProject.projectDir
    val envFiles = listOf(
        File(repoRoot, ".env.local"),
        File(repoRoot, ".env"),
        File(repoRoot, ".env.example"),
        rootProject.file(".env.local"),
        rootProject.file(".env"),
        rootProject.file("../.env.local"),
        rootProject.file("../.env"),
        rootProject.file("../.env.example"),
    ).distinctBy { it.absoluteFile.toPath().normalize().toString() }

    envFiles.forEach fileLoop@ { envFile ->
        if (!envFile.exists()) return@fileLoop
        envFile.readLines().forEach lineLoop@ { rawLine ->
            val line = rawLine.trim()
            if (line.isEmpty() || line.startsWith("#")) return@lineLoop
            val idx = line.indexOf('=')
            if (idx <= 0) return@lineLoop
            val k = line.substring(0, idx).trim()
            if (k != key) return@lineLoop
            var v = line.substring(idx + 1).trim()
        // 去掉首尾配对的引号
            if ((v.startsWith("\"") && v.endsWith("\"")) || (v.startsWith("'") && v.endsWith("'"))) {
                v = v.substring(1, v.length - 1)
            }
            return v
        }
    }
    return null
}

android {
    namespace = "com.example.taskflow"
    compileSdk = libs.versions.compileSdk.get().toInt()

    defaultConfig {
        applicationId = "com.example.taskflow"
        minSdk = libs.versions.minSdk.get().toInt()
        targetSdk = libs.versions.targetSdk.get().toInt()
        versionCode = 140
        versionName = "1.4.0"

        // 默认服务端地址(出厂值)。优先级:
        //   1) 环境变量 TASKFLOW_DEFAULT_SERVER_URL —— 打包时显式传入
        //   2) 仓库根 .env 的 PUBLIC_API_URL  —— 后端 API 域名(前后端分离)
        //   3) 仓库根 .env 的 PUBLIC_BASE_URL  —— 前端域名(单域名部署兼容回退)
        //   4) local.properties 的 taskflow.default.server.url  —— 单机开发
        //   5) 兜底 https://backend.taskflow.teamcy.eu.cc
        // 服务端地址已固化,安装后不再允许用户在 App 内修改。
        val defaultServer: String = run {
            System.getenv("TASKFLOW_DEFAULT_SERVER_URL")?.trim()?.takeIf { it.isNotEmpty() }
                ?.let { return@run it.trimEnd('/') }
            parseRootDotEnv("PUBLIC_API_URL")?.takeIf { it.isNotEmpty() }
                ?.let { return@run it.trimEnd('/') }
            parseRootDotEnv("PUBLIC_BASE_URL")?.takeIf { it.isNotEmpty() }
                ?.let { return@run it.trimEnd('/') }
            val localProps = Properties().apply {
                val f = rootProject.file("local.properties")
                if (f.exists()) f.inputStream().use { load(it) }
            }
            val fromLocal = localProps.getProperty("taskflow.default.server.url")?.trim().orEmpty()
            if (fromLocal.isNotEmpty()) return@run fromLocal.trimEnd('/')
            "https://backend.taskflow.teamcy.eu.cc"
        }
        buildConfigField("String", "DEFAULT_SERVER_URL", "\"$defaultServer\"")

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    // APK 签名配置:环境变量优先(CI/CD),其次读根 .env
    val signKeystorePath: String? = System.getenv("ANDROID_KEYSTORE_PATH")
        ?: parseRootDotEnv("ANDROID_KEYSTORE_PATH")
    val signKeystorePass: String? = System.getenv("ANDROID_KEYSTORE_PASSWORD")
        ?: parseRootDotEnv("ANDROID_KEYSTORE_PASSWORD")
    val signAlias: String? = System.getenv("ANDROID_KEY_ALIAS")
        ?: parseRootDotEnv("ANDROID_KEY_ALIAS")
    val signKeyPass: String? = System.getenv("ANDROID_KEY_PASSWORD")
        ?: parseRootDotEnv("ANDROID_KEY_PASSWORD")

    val releaseSigningReady = signKeystorePath != null &&
        signKeystorePass != null && signAlias != null &&
        rootProject.file(signKeystorePath).exists()

    if (releaseSigningReady) {
        signingConfigs {
            create("release") {
                storeFile = rootProject.file(signKeystorePath!!)
                storePassword = signKeystorePass!!
                keyAlias = signAlias!!
                keyPassword = signKeyPass ?: signKeystorePass
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            if (releaseSigningReady) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
        debug {
            // 默认就是 debug,允许明文 HTTP(只在 debug 生效,见 res/xml/network_security_config.xml)
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    packaging {
        resources {
            excludes += setOf(
                "/META-INF/{AL2.0,LGPL2.1}",
                "/META-INF/INDEX.LIST",
                "/META-INF/DEPENDENCIES",
            )
        }
        jniLibs {
            keepDebugSymbols += setOf(
                "**/libandroidx.graphics.path.so",
                "**/libdatastore_shared_counter.so",
            )
        }
    }
}

// 让 release/debug 的 APK 输出名为 TaskFlow-release.apk / TaskFlow-debug.apk。
// 部署侧 (scripts-deploy/deploy-android.ps1) 会把 release apk 拷到 /var/www/taskflow/downloads/。
base {
    archivesName.set("TaskFlow")
}

dependencies {
    // AndroidX core
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.vm.ktx)
    implementation(libs.androidx.lifecycle.vm.comp)
    implementation(libs.androidx.activity.compose)

    // Compose
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.foundation)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.ext)
    implementation(libs.compose.runtime.livedata)
    debugImplementation(libs.compose.ui.tooling)

    // Navigation
    implementation(libs.navigation.compose)

    // Work / DataStore / Security
    implementation(libs.work.runtime.ktx)
    implementation(libs.datastore.preferences)
    implementation(libs.security.crypto)

    // Custom Tabs(OAuth 登录用,LoginScreen 调用)
    implementation("androidx.browser:browser:1.8.0")

    // Room
    implementation(libs.room.runtime)
    implementation(libs.room.ktx)
    ksp(libs.room.compiler)

    // Network
    implementation(libs.retrofit)
    implementation(libs.retrofit.converter.moshi)
    implementation(libs.okhttp)
    implementation(libs.okhttp.logging)

    // Moshi
    implementation(libs.moshi)
    implementation(libs.moshi.kotlin)
    ksp(libs.moshi.codegen)

    // Coroutines
    implementation(libs.coroutines.core)
    implementation(libs.coroutines.android)

    testImplementation(kotlin("test"))
}
