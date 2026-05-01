package com.example.taskflow.data.remote

import com.example.taskflow.BuildConfig
import com.example.taskflow.data.auth.TokenManager
import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import retrofit2.converter.moshi.MoshiConverterFactory
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

/**
 * 让 Retrofit 的 baseUrl 在用户改了服务端 URL 后也能切换 — 我们包了一层
 * AtomicReference,每次请求都从里头读 base 重写 host。
 */
class ApiClient(private val tokenManager: TokenManager) {

    private val baseRef = AtomicReference(resolveBase(tokenManager.current().serverUrl))

    val moshi: Moshi = Moshi.Builder()
        .add(KotlinJsonAdapterFactory())
        .build()

    private val authInterceptor = AuthInterceptor(
        tokenManager = tokenManager,
        moshi = moshi,
        refreshUrlProvider = { baseRef.get() + "api/auth/refresh" },
    )

    /**
     * Host-rewriting interceptor —— Retrofit 的 baseUrl 是创建时固定的;
     * 我们这里把每个请求的 host 改成当前 base,就支持运行时切换服务端。
     */
    private val hostRewrite = okhttp3.Interceptor { chain ->
        val current = baseRef.get().toHttpUrl()
        val originalUrl = chain.request().url
        // 仅改 host / port / scheme,保留路径和 query
        val rewritten = originalUrl.newBuilder()
            .scheme(current.scheme)
            .host(current.host)
            .port(current.port)
            .build()
        chain.proceed(chain.request().newBuilder().url(rewritten).build())
    }

    private val logging = HttpLoggingInterceptor().apply {
        level = if (BuildConfig.DEBUG) HttpLoggingInterceptor.Level.BASIC
        else HttpLoggingInterceptor.Level.NONE
    }

    private val okhttp: OkHttpClient = OkHttpClient.Builder()
        .addInterceptor(hostRewrite)
        .addInterceptor(authInterceptor)
        .addInterceptor(logging)
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .build()

    private val retrofit: Retrofit = Retrofit.Builder()
        // baseUrl 必须给一个能被解析的占位,真正的 host 由 hostRewrite 改写
        .baseUrl("http://placeholder.invalid/")
        .client(okhttp)
        .addConverterFactory(MoshiConverterFactory.create(moshi))
        .build()

    val api: ApiService = retrofit.create(ApiService::class.java)

    fun setBase(url: String) {
        baseRef.set(resolveBase(url))
    }

    fun currentBase(): String = baseRef.get()

    private fun resolveBase(userUrl: String?): String {
        val raw = userUrl?.takeIf { it.isNotBlank() } ?: BuildConfig.DEFAULT_SERVER_URL
        return if (raw.endsWith("/")) raw else "$raw/"
    }

    private fun String.toHttpUrl(): okhttp3.HttpUrl {
        val v = if (this.endsWith("/")) this else "$this/"
        return okhttp3.HttpUrl.Builder()
            .scheme(if (v.startsWith("https://")) "https" else "http")
            .host(v.removePrefix("https://").removePrefix("http://").substringBefore("/").substringBefore(":"))
            .port(
                v.removePrefix("https://").removePrefix("http://")
                    .substringBefore("/")
                    .let {
                        val portStr = it.substringAfter(":", "")
                        portStr.toIntOrNull() ?: if (v.startsWith("https://")) 443 else 80
                    }
            )
            .build()
    }
}

/** Helper to make the "no-auth" Retrofit request via direct OkHttp call sites. */
fun Request.Builder.skipAuth(): Request.Builder = tag(NoAuth::class.java, NoAuth)
