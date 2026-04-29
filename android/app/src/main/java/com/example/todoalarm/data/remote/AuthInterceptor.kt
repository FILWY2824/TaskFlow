package com.example.todoalarm.data.remote

import com.example.todoalarm.data.auth.TokenManager
import com.squareup.moshi.Moshi
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.IOException
import java.util.concurrent.locks.ReentrantLock

/**
 * AuthInterceptor:
 *   - 把 Bearer access token 注入到所有非"无认证"请求里。
 *   - 收到 401 时,以单飞模式尝试刷新一次 access。刷新成功后用新 token 重试一次原请求。
 *     刷新失败 → 清空本地 token,放行 401 让上层 ViewModel 跳登录。
 *
 * 标记不需要认证的请求方式:`Request.Builder().tag(NoAuth, NoAuth)`,见 ApiAuthTags 帮手。
 */
class AuthInterceptor(
    private val tokenManager: TokenManager,
    private val moshi: Moshi,
    private val refreshUrlProvider: () -> String,
) : Interceptor {

    private val refreshLock = ReentrantLock()
    private val refreshAdapter = moshi.adapter(RefreshRequest::class.java)
    private val authResponseAdapter = moshi.adapter(AuthResponse::class.java)

    @Throws(IOException::class)
    override fun intercept(chain: Interceptor.Chain): Response {
        val original = chain.request()
        val noAuth = original.tag(NoAuth::class.java) != null

        val firstReq = if (noAuth) original else withAuth(original)
        val firstRes = chain.proceed(firstReq)

        if (noAuth || firstRes.code != 401) return firstRes

        // === 401 处理 ===
        firstRes.close()
        val refreshed = tryRefresh(chain) ?: run {
            // 刷新不到,清空 session,让上层路由跳回登录
            tokenManager.clear()
            // 重新构造一个新的 401 响应给调用方
            return chain.proceed(original)
        }

        // 用新 access token 重试一次原请求
        val retryReq = original.newBuilder()
            .header("Authorization", "Bearer $refreshed")
            .build()
        return chain.proceed(retryReq)
    }

    private fun withAuth(req: Request): Request {
        val token = tokenManager.current().accessToken ?: return req
        return req.newBuilder().header("Authorization", "Bearer $token").build()
    }

    /** 单飞:并发 401 都等同一个 refresh promise 完成。返回新 access token,失败返回 null。 */
    private fun tryRefresh(chain: Interceptor.Chain): String? {
        val current = tokenManager.current()
        val refreshToken = current.refreshToken ?: return null

        // 持锁前再读一遍 — 也许另一个请求刚刷新过
        refreshLock.lock()
        try {
            val now = tokenManager.current()
            if (now.accessToken != null && now.accessToken != current.accessToken) {
                return now.accessToken
            }

            val body = refreshAdapter.toJson(RefreshRequest(refreshToken))
                .toRequestBody("application/json".toMediaType())
            val refreshReq = Request.Builder()
                .url(refreshUrlProvider())
                .post(body)
                .tag(NoAuth::class.java, NoAuth)
                .build()

            val res = chain.proceed(refreshReq)
            if (!res.isSuccessful) {
                res.close()
                return null
            }
            val payload = res.body?.string() ?: run { res.close(); return null }
            res.close()

            val parsed = authResponseAdapter.fromJson(payload) ?: return null
            tokenManager.updateTokens(parsed.access_token, parsed.refresh_token)
            return parsed.access_token
        } finally {
            refreshLock.unlock()
        }
    }
}

/** Tag class. Use as `Request.Builder().tag(NoAuth::class.java, NoAuth)` to skip auth header. */
object NoAuth
