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

/**
 * Retrofit 客户端封装 —— 服务端地址固化到 BuildConfig.DEFAULT_SERVER_URL,运行时不可切换。
 *
 * 历史说明:之前允许用户在设置页 / 登录页自行输入服务端 URL,
 * 并通过 AtomicReference + hostRewrite interceptor 做运行时切换。
 * 现已移除该能力,API 请求始终指向构建时烧入的地址。
 */
class ApiClient(private val tokenManager: TokenManager) {

    /** 服务端 base URL,始终以 "/" 结尾 */
    private val base: String = BuildConfig.DEFAULT_SERVER_URL.let {
        if (it.endsWith("/")) it else "$it/"
    }

    val moshi: Moshi = Moshi.Builder()
        .add(KotlinJsonAdapterFactory())
        .build()

    private val authInterceptor = AuthInterceptor(
        tokenManager = tokenManager,
        moshi = moshi,
        refreshUrlProvider = { base + "api/auth/refresh" },
    )

    /**
     * Host-rewriting interceptor —— Retrofit 的 baseUrl 是构建时烧入的,
     * 每次请求将 host 重写为 BuildConfig.DEFAULT_SERVER_URL。
     */
    private val hostRewrite = okhttp3.Interceptor { chain ->
        val current = base.toHttpUrl()
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

    /** 服务端地址已固化;此方法保留仅为兼容旧代码(AuthViewModels 仍会调用),无实际效果 */
    fun setBase(url: String) {}

    /** 返回当前服务端 base URL(始终为 BuildConfig.DEFAULT_SERVER_URL) */
    fun currentBase(): String = base

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
