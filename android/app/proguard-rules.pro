# Minimal proguard rules.
# AGP / R8 知道 Compose / Moshi / Retrofit 的大部分 keep,但 Moshi 反射生成的适配器需手动保护:

# Moshi - 生成的 *JsonAdapter 类要保留
-keep class * implements com.squareup.moshi.JsonAdapter
-keep,allowobfuscation,allowshrinking @com.squareup.moshi.JsonClass class *
-keep @com.squareup.moshi.JsonClass class * { <init>(...); *; }

# OkHttp / Retrofit
-dontwarn okhttp3.**
-dontwarn retrofit2.**
-keep class retrofit2.** { *; }

# Coroutines stack trace
-keepnames class kotlinx.coroutines.internal.MainDispatcherFactory {}
-keepnames class kotlinx.coroutines.CoroutineExceptionHandler {}

# Tink / Security-Crypto annotations (compile-time only, not needed at runtime)
-dontwarn com.google.errorprone.annotations.**

# Compose - usually handled by AGP / kotlin-compose plugin
