package com.example.taskflow.ui.screens

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.example.taskflow.AppContainer
import com.example.taskflow.data.repository.Result
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * Android 端登录 UI 状态(OAuth-only)。
 *
 * 三端均强制走 OAuth,Android 走 "Custom Tabs + 服务端 poll"。状态机:
 *
 *   IDLE -> LAUNCHING -> WAITING -> FINALIZING -> SUCCESS
 *                                ^             v
 *                                +---ERROR-----+
 *
 * 用户在登录页:
 *   - 点 "通过认证中心登录" 按钮
 *      * ViewModel 生成 device_id 并把 OAuth start URL 通过 [pendingOpenUrl] 暴露
 *      * Activity 监听 pendingOpenUrl,用 Custom Tabs / Intent 打开
 *      * ViewModel 在后台 poll 服务端,拿到 handoff -> 调 finalize -> 保存 token -> success
 */
data class OAuthLoginState(
    val serverUrl: String = "",
    val phase: Phase = Phase.IDLE,
    val error: String? = null,
    /** 一次性"请打开这个 URL"信号,Activity 拿到后立刻 setNull,避免重复打开 */
    val pendingOpenUrl: String? = null,
    val success: Boolean = false,
) {
    enum class Phase { IDLE, LAUNCHING, WAITING, FINALIZING }
}

class LoginViewModel(private val container: AppContainer) : ViewModel() {

    private val _state = MutableStateFlow(
        OAuthLoginState(
            serverUrl = container.tokenManager.current().serverUrl
                ?: container.apiClient.currentBase().trimEnd('/'),
        )
    )
    val state: StateFlow<OAuthLoginState> = _state.asStateFlow()

    private var pollJob: Job? = null
    private var deviceId: String? = null

    fun setServerUrl(v: String) {
        _state.value = _state.value.copy(serverUrl = v, error = null)
    }

    fun reportError(message: String) {
        _state.value = _state.value.copy(phase = OAuthLoginState.Phase.IDLE, error = message)
    }

    fun clearError() {
        _state.value = _state.value.copy(error = null)
    }

    /**
     * 用户点 "通过认证中心登录":
     *   1) 应用 server URL 到 ApiClient + TokenManager
     *   2) 生成 device_id,拼出 start URL
     *   3) 通过 pendingOpenUrl 通知 Activity 打开浏览器
     *   4) 后台 poll handoff,拿到后 finalize,成功则 success=true,UI 跳转
     */
    fun startOAuth() {
        applyServerUrl(_state.value.serverUrl)
        pollJob?.cancel()
        _state.value = _state.value.copy(
            phase = OAuthLoginState.Phase.LAUNCHING,
            error = null,
            pendingOpenUrl = null,
        )
        pollJob = viewModelScope.launch {
            when (val cfg = container.authRepository.authConfig()) {
                is Result.Success -> {
                    if (!cfg.data.oauth_enabled) {
                        _state.value = _state.value.copy(
                            phase = OAuthLoginState.Phase.IDLE,
                            error = "当前服务端未启用认证中心登录",
                        )
                        return@launch
                    }
                }
                is Result.Error -> {
                    _state.value = _state.value.copy(
                        phase = OAuthLoginState.Phase.IDLE,
                        error = "无法连接服务端: ${cfg.message}",
                    )
                    return@launch
                }
            }

            val id = container.authRepository.generateDeviceId()
            deviceId = id
            val url = container.authRepository.oauthStartUrl(id)
            _state.value = _state.value.copy(
                phase = OAuthLoginState.Phase.WAITING,
                pendingOpenUrl = url,
            )
            val pollRes = container.authRepository.pollForHandoff(id)
            when (pollRes) {
                is Result.Success -> {
                    _state.value = _state.value.copy(phase = OAuthLoginState.Phase.FINALIZING)
                    val finalizeRes = container.authRepository.finalize(pollRes.data)
                    when (finalizeRes) {
                        is Result.Success -> {
                            // 拉一遍提醒列表到本地缓存,保证离线可触发
                            container.reminderRepository.refreshAll()
                            _state.value = _state.value.copy(
                                phase = OAuthLoginState.Phase.IDLE,
                                success = true,
                            )
                        }
                        is Result.Error -> _state.value = _state.value.copy(
                            phase = OAuthLoginState.Phase.IDLE,
                            error = finalizeRes.message,
                        )
                    }
                }
                is Result.Error -> _state.value = _state.value.copy(
                    phase = OAuthLoginState.Phase.IDLE,
                    error = pollRes.message,
                )
            }
        }
    }

    /** 用户在系统浏览器里取消、或点 UI 上的"取消" -> 立刻停轮询、重置状态。 */
    fun cancelOAuth() {
        pollJob?.cancel()
        pollJob = null
        deviceId = null
        _state.value = _state.value.copy(
            phase = OAuthLoginState.Phase.IDLE,
            pendingOpenUrl = null,
            error = null,
        )
    }

    /** Activity 把 pendingOpenUrl 消费完,通知 VM 重置,避免重复打开 */
    fun consumePendingOpenUrl() {
        if (_state.value.pendingOpenUrl != null) {
            _state.value = _state.value.copy(pendingOpenUrl = null)
        }
    }

    private fun applyServerUrl(url: String) {
        val v = url.trim()
        if (v.isNotBlank()) {
            container.tokenManager.setServerUrl(v)
            container.apiClient.setBase(v)
        }
    }

    override fun onCleared() {
        pollJob?.cancel()
        super.onCleared()
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = LoginViewModel(container) as T
    }
}
