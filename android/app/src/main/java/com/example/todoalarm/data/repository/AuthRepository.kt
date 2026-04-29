package com.example.todoalarm.data.repository

import com.example.todoalarm.data.auth.TokenManager
import com.example.todoalarm.data.remote.ApiClient
import com.example.todoalarm.data.remote.LoginRequest
import com.example.todoalarm.data.remote.LogoutRequest
import com.example.todoalarm.data.remote.RegisterRequest
import com.example.todoalarm.data.remote.UserDto
import com.example.todoalarm.data.local.AppDatabase

class AuthRepository(
    private val client: ApiClient,
    private val tokenManager: TokenManager,
    private val db: AppDatabase,
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
