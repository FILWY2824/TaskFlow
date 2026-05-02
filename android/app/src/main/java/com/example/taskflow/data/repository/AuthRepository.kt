package com.example.taskflow.data.repository

import com.example.taskflow.data.auth.TokenManager
import com.example.taskflow.data.local.AppDatabase
import com.example.taskflow.data.remote.ApiClient
import com.example.taskflow.data.remote.AuthConfigDto
import com.example.taskflow.data.remote.LogoutRequest
import com.example.taskflow.data.remote.OAuthFinalizeRequest
import com.example.taskflow.data.remote.UserDto
import kotlinx.coroutines.delay
import java.security.SecureRandom

/**
 * Android 端 Auth Repository(OAuth-only)。
 *
 * 三端均强制 OAuth 登录,Android 走 "Custom Tabs + 服务端 poll" 流程:
 *
 *   1) [generateDeviceId]  生成不可猜的 device_id(32 字节随机)
 *   2) [oauthStartUrl]     拼出 ${base}/api/auth/oauth/start?client=android&device_id=<id>
 *                          交给 Activity 打开 Custom Tab
 *   3) [pollForHandoff]    用户在浏览器里登录后,服务端把 handoff 通过 device_id 索引;
 *                          这里以 1.5s 间隔轮询 /api/auth/oauth/poll,直到拿到 code 或超时
 *   4) [finalize]          用 handoff code 调 /api/auth/oauth/finalize 换 access/refresh token
 *
 * 详见 server/internal/handlers/oauth.go。
 */
class AuthRepository(
    private val client: ApiClient,
    private val tokenManager: TokenManager,
    private val db: AppDatabase,
    private val prefs: PreferenceRepository? = null,
) {

    /** 拉一次 /api/auth/config,判断后端是否启用 OAuth(本项目要求必须启用)。 */
    suspend fun authConfig(): Result<AuthConfigDto> = safeCall(client.moshi) { client.api.authConfig() }

    /**
     * 生成 32 字节随机 device_id(64 hex)。每次按"通过认证中心登录"按钮时调一次。
     */
    fun generateDeviceId(): String {
        val buf = ByteArray(32)
        SecureRandom().nextBytes(buf)
        return buf.joinToString("") { "%02x".format(it) }
    }

    /** 拼出系统浏览器要打开的 OAuth start URL。 */
    fun oauthStartUrl(deviceId: String): String {
        val base = client.currentBase().trimEnd('/')
        return "$base/api/auth/oauth/start?client=android&device_id=${deviceId}"
    }

    /**
     * 启动轮询协程。建议在 ViewModelScope 里以 launch 调用,
     * pollIntervalMs / timeoutMs 都有合理默认。
     *
     * 返回值:
     *   - Result.Success(handoffCode) 用户已在浏览器中完成登录
     *   - Result.Error                超时 / 网络错误 / 用户主动取消(在 ViewModel 那层 cancel job 即可)
     */
    suspend fun pollForHandoff(
        deviceId: String,
        pollIntervalMs: Long = 1500,
        timeoutMs: Long = 5 * 60 * 1000,
    ): Result<String> {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            // 单次 poll: 200 -> 已就绪;204 -> 还没好;其他 -> 错误
            val raw = try {
                client.api.oauthPoll(deviceId)
            } catch (e: Exception) {
                // 网络瞬断,等下一轮再试
                delay(pollIntervalMs)
                continue
            }
            when (raw.code()) {
                200 -> {
                    val body = raw.body()
                    if (body?.code != null && body.code.isNotEmpty()) {
                        return Result.Success(body.code)
                    }
                    delay(pollIntervalMs)
                }
                204 -> delay(pollIntervalMs)
                else -> {
                    val msg = raw.errorBody()?.string()?.take(200) ?: "poll http ${raw.code()}"
                    return Result.Error("poll_failed", msg, raw.code())
                }
            }
        }
        return Result.Error("timeout", "登录超时,请重试", -1)
    }

    /** 用 handoff code 换 access / refresh token,并保存登录态。 */
    suspend fun finalize(handoffCode: String): Result<UserDto> {
        val r = safeCall(client.moshi) {
            client.api.oauthFinalize(OAuthFinalizeRequest(code = handoffCode))
        }
        return when (r) {
            is Result.Success -> {
                val resp = r.data
                tokenManager.save(
                    accessToken = resp.access_token,
                    refreshToken = resp.refresh_token,
                    userId = resp.user.id,
                    userEmail = resp.user.email,
                    timezone = resp.user.timezone,
                )
                prefs?.refresh()
                Result.Success(resp.user)
            }
            is Result.Error -> r
        }
    }

    suspend fun logout(): Result<Unit> {
        val refresh = tokenManager.current().refreshToken
        val userId = tokenManager.current().userId
        val r = safeCall(client.moshi) {
            client.api.logout(LogoutRequest(refresh_token = refresh))
        }
        // 本地无论 server 反应如何都清掉
        tokenManager.clear()
        if (userId != null) {
            try {
                db.todoDao().clearForUser(userId)
                db.listDao().clearForUser(userId)
                db.subtaskDao().clearForUser(userId)
                db.reminderDao().clearForUser(userId)
            } catch (_: Exception) { }
        }
        return r
    }

    suspend fun me(): Result<UserDto> = safeCall(client.moshi) { client.api.me() }
}
