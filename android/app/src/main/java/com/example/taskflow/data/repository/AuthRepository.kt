package com.example.taskflow.data.repository

import com.example.taskflow.data.auth.TokenManager
import com.example.taskflow.data.remote.ApiClient
import com.example.taskflow.data.remote.LoginRequest
import com.example.taskflow.data.remote.LogoutRequest
import com.example.taskflow.data.remote.RegisterRequest
import com.example.taskflow.data.remote.UserDto
import com.example.taskflow.data.local.AppDatabase

class AuthRepository(
    private val client: ApiClient,
    private val tokenManager: TokenManager,
    private val db: AppDatabase,
    private val prefs: PreferenceRepository? = null,
) {
    suspend fun login(email: String, password: String): Result<UserDto> {
        val r = safeCall(client.moshi) {
            client.api.login(LoginRequest(email = email.trim(), password = password))
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
                // 登录成功后,立刻拉一次本端 scope='android' 的偏好。
                // 失败也不影响登录流程,UI 会用本地缓存先把开关铺起来,等下次 onResume 再拉。
                prefs?.refresh()
                Result.Success(resp.user)
            }
            is Result.Error -> r
        }
    }

    suspend fun register(email: String, password: String, displayName: String?, timezone: String?): Result<UserDto> {
        val r = safeCall(client.moshi) {
            client.api.register(RegisterRequest(
                email = email.trim(),
                password = password,
                display_name = displayName?.takeIf { it.isNotBlank() },
                timezone = timezone?.takeIf { it.isNotBlank() },
            ))
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
