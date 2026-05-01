// app/build.gradle.kts — 主模块
//
// 关键决定:
//   - 自管 DI(单例 AppContainer),不用 Hilt,减少注解处理开销与 incremental 复杂度。
//   - Room 用 KSP(KAPT 已不推荐)。
//   - Moshi codegen + Kotlin reflection 0 用,所有 DTO 都标注 @JsonClass(generateAdapter = true)。
//   - Compose 插件由 Kotlin 2.0 直接提供,不再依赖独立 compiler 版本对齐。

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.ksp)
}

android {
    namespace = "com.example.taskflow"
    compileSdk = libs.versions.compileSdk.get().toInt()

    defaultConfig {
        applicationId = "com.example.taskflow"
        minSdk = libs.versions.minSdk.get().toInt()
        targetSdk = libs.versions.targetSdk.get().toInt()
        versionCode = 1
        versionName = "0.4.0"

        // 默认服务端地址。优先级:
        //   1) 环境变量 TASKFLOW_DEFAULT_SERVER_URL(打包时传入,例如 GitHub Actions)
        //   2) local.properties 里的 taskflow.default.server.url(本地开发常用)
        //   3) 兜底 http://10.0.2.2:8080(Android 模拟器把它路由到宿主机 127.0.0.1)
        // 用户安装后仍可在 App 设置里改,改的值会持久化到 EncryptedSharedPreferences。
        val defaultServer: String = run {
            val fromEnv = System.getenv("TASKFLOW_DEFAULT_SERVER_URL")?.trim().orEmpty()
            if (fromEnv.isNotEmpty()) return@run fromEnv.trimEnd('/')
            val localProps = java.util.Properties().apply {
                val f = rootProject.file("local.properties")
                if (f.exists()) f.inputStream().use { load(it) }
            }
            val fromLocal = localProps.getProperty("taskflow.default.server.url")?.trim().orEmpty()
            if (fromLocal.isNotEmpty()) return@run fromLocal.trimEnd('/')
            "http://10.0.2.2:8080"
        }
        buildConfigField("String", "DEFAULT_SERVER_URL", "\"$defaultServer\"")

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
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
    }
}

// 让 release/debug 的 APK 输出名为 TaskFlow-release.apk / TaskFlow-debug.apk,
// 与 Settings 页"客户端下载"卡片里的 href 路径一致(/android/app/build/outputs/apk/release/TaskFlow-release.apk)。
base {
    archivesName.set("TaskFlow")
}

dependencies {
    // AndroidX core
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime)
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
}
