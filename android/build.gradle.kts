// 顶层 Gradle 构建文件:仅声明插件,不在这里 apply。
//
// AGP 8.7 + Kotlin 2.0 + Compose plugin(2.0 起 Compose 编译器拆成独立插件)。
plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.ksp) apply false
}
