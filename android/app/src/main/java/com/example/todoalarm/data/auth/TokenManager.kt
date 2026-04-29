package com.example.todoalarm.data.auth

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Persists access / refresh tokens + the user's home server URL inside
 * an EncryptedSharedPreferences (AES-256-GCM, master key in Keystore).
 *
 * Why encrypted: tokens grant access to the user's data, and Android's
 * regular SharedPreferences is world-readable on rooted / dev devices.
 *
 * Threading: methods are thread-safe (all backed by the underlying SharedPreferences).
 * The currentSession StateFlow is updated whenever tokens change; UI / background
 * tasks observe it to react to login / logout / 401 → cleared.
 */
class TokenManager(context: Context) {

    private val prefs: SharedPreferences = run {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        EncryptedSharedPreferences.create(
            context,
            "todoalarm_secure_prefs",
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    private val _session = MutableStateFlow(load())
    val session: StateFlow<Session> = _session.asStateFlow()

    fun current(): Session = _session.value

    @Synchronized
    fun save(
        accessToken: String,
        refreshToken: String,
        userId: Long,
        userEmail: String,
        timezone: String,
    ) {
        prefs.edit().apply {
            putString(KEY_ACCESS, accessToken)
            putString(KEY_REFRESH, refreshToken)
            putLong(KEY_USER_ID, userId)
            putString(KEY_USER_EMAIL, userEmail)
            putString(KEY_TZ, timezone)
            apply()
        }
        _session.value = load()
    }

    @Synchronized
    fun updateTokens(accessToken: String, refreshToken: String) {
        prefs.edit()
            .putString(KEY_ACCESS, accessToken)
            .putString(KEY_REFRESH, refreshToken)
            .apply()
        _session.value = load()
    }

    @Synchronized
    fun setServerUrl(url: String) {
        prefs.edit().putString(KEY_SERVER_URL, url).apply()
        _session.value = load()
    }

    @Synchronized
    fun clear() {
        prefs.edit()
            .remove(KEY_ACCESS)
            .remove(KEY_REFRESH)
            .remove(KEY_USER_ID)
            .remove(KEY_USER_EMAIL)
            .remove(KEY_TZ)
            .apply()
        // 注意:不清 server URL,登出后下次登录还在同一台后端
        _session.value = load()
    }

    private fun load(): Session = Session(
        accessToken = prefs.getString(KEY_ACCESS, null),
        refreshToken = prefs.getString(KEY_REFRESH, null),
        userId = prefs.getLong(KEY_USER_ID, 0L).takeIf { it > 0 },
        userEmail = prefs.getString(KEY_USER_EMAIL, null),
        timezone = prefs.getString(KEY_TZ, "UTC") ?: "UTC",
        serverUrl = prefs.getString(KEY_SERVER_URL, null),
    )

    companion object {
        private const val KEY_ACCESS = "access_token"
        private const val KEY_REFRESH = "refresh_token"
        private const val KEY_USER_ID = "user_id"
        private const val KEY_USER_EMAIL = "user_email"
        private const val KEY_TZ = "user_tz"
        private const val KEY_SERVER_URL = "server_url"
    }
}

data class Session(
    val accessToken: String?,
    val refreshToken: String?,
    val userId: Long?,
    val userEmail: String?,
    val timezone: String,
    /** null = use BuildConfig.DEFAULT_SERVER_URL */
    val serverUrl: String?,
) {
    val isLoggedIn: Boolean get() = accessToken != null && userId != null
}
