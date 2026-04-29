package com.example.todoalarm.ui.screens

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.example.todoalarm.AppContainer
import com.example.todoalarm.data.repository.Result
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import java.time.ZoneId

data class AuthUiState(
    val email: String = "",
    val password: String = "",
    val displayName: String = "",
    val timezone: String = ZoneId.systemDefault().id,
    val serverUrl: String = "",
    val isLoading: Boolean = false,
    val error: String? = null,
    val success: Boolean = false,
)

class LoginViewModel(private val container: AppContainer) : ViewModel() {

    private val _state = MutableStateFlow(
        AuthUiState(serverUrl = container.tokenManager.current().serverUrl ?: container.apiClient.currentBase().trimEnd('/'))
    )
    val state: StateFlow<AuthUiState> = _state.asStateFlow()

    fun setEmail(v: String) { _state.value = _state.value.copy(email = v, error = null) }
    fun setPassword(v: String) { _state.value = _state.value.copy(password = v, error = null) }
    fun setServerUrl(v: String) { _state.value = _state.value.copy(serverUrl = v, error = null) }

    fun login() {
        val s = _state.value
        if (s.email.isBlank() || s.password.isBlank()) {
            _state.value = s.copy(error = "请输入邮箱和密码")
            return
        }
        // 应用服务端 URL
        applyServerUrl(s.serverUrl)
        _state.value = s.copy(isLoading = true, error = null)
        viewModelScope.launch {
            val r = container.authRepository.login(s.email, s.password)
            _state.value = when (r) {
                is Result.Success -> {
                    // 拉一遍提醒列表到本地缓存,保证离线可触发
                    container.reminderRepository.refreshAll()
                    _state.value.copy(isLoading = false, success = true, error = null)
                }
                is Result.Error -> _state.value.copy(isLoading = false, error = r.message)
            }
        }
    }

    private fun applyServerUrl(url: String) {
        val v = url.trim()
        if (v.isNotBlank()) {
            container.tokenManager.setServerUrl(v)
            container.apiClient.setBase(v)
        }
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = LoginViewModel(container) as T
    }
}

class RegisterViewModel(private val container: AppContainer) : ViewModel() {
    private val _state = MutableStateFlow(
        AuthUiState(serverUrl = container.tokenManager.current().serverUrl ?: container.apiClient.currentBase().trimEnd('/'))
    )
    val state: StateFlow<AuthUiState> = _state.asStateFlow()

    fun setEmail(v: String) { _state.value = _state.value.copy(email = v, error = null) }
    fun setPassword(v: String) { _state.value = _state.value.copy(password = v, error = null) }
    fun setDisplayName(v: String) { _state.value = _state.value.copy(displayName = v) }
    fun setTimezone(v: String) { _state.value = _state.value.copy(timezone = v) }
    fun setServerUrl(v: String) { _state.value = _state.value.copy(serverUrl = v, error = null) }

    fun register() {
        val s = _state.value
        if (s.email.isBlank() || s.password.length < 8) {
            _state.value = s.copy(error = "邮箱必填,密码至少 8 位")
            return
        }
        val v = s.serverUrl.trim()
        if (v.isNotBlank()) {
            container.tokenManager.setServerUrl(v)
            container.apiClient.setBase(v)
        }
        _state.value = s.copy(isLoading = true, error = null)
        viewModelScope.launch {
            val r = container.authRepository.register(s.email, s.password, s.displayName, s.timezone)
            _state.value = when (r) {
                is Result.Success -> {
                    container.reminderRepository.refreshAll()
                    _state.value.copy(isLoading = false, success = true)
                }
                is Result.Error -> _state.value.copy(isLoading = false, error = r.message)
            }
        }
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = RegisterViewModel(container) as T
    }
}
