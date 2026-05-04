package com.example.taskflow.ui.screens

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.example.taskflow.AppContainer
import com.example.taskflow.data.remote.NotificationDto
import com.example.taskflow.data.remote.PomodoroSessionDto
import com.example.taskflow.data.remote.StatsSummaryDto
import com.example.taskflow.data.remote.TelegramBindToken
import com.example.taskflow.data.remote.TelegramBinding
import com.example.taskflow.data.repository.AndroidPrefs
import com.example.taskflow.data.repository.PreferenceRepository
import com.example.taskflow.data.repository.Result
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import java.time.ZoneId

// ============== Notifications ==============

data class NotificationsUiState(
    val loading: Boolean = false,
    val items: List<NotificationDto> = emptyList(),
    val unreadCount: Int = 0,
    val error: String? = null,
)

class NotificationsViewModel(private val container: AppContainer) : ViewModel() {
    private val _state = MutableStateFlow(NotificationsUiState())
    val state: StateFlow<NotificationsUiState> = _state.asStateFlow()

    init { refresh() }

    fun refresh(onlyUnread: Boolean = false) {
        _state.value = _state.value.copy(loading = true, error = null)
        viewModelScope.launch {
            val r = container.notificationRepository.list(onlyUnread = onlyUnread, limit = 100)
            _state.value = when (r) {
                is Result.Success -> _state.value.copy(loading = false, items = r.data.first, unreadCount = r.data.second)
                is Result.Error -> _state.value.copy(loading = false, error = r.message)
            }
        }
    }

    fun markRead(id: Long) {
        viewModelScope.launch {
            container.notificationRepository.markRead(id)
            refresh()
        }
    }

    fun markAllRead() {
        viewModelScope.launch {
            container.notificationRepository.markAllRead()
            refresh()
        }
    }

    fun clearError() {
        _state.value = _state.value.copy(error = null)
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = NotificationsViewModel(container) as T
    }
}

// ============== Telegram ==============

data class TelegramUiState(
    val loading: Boolean = false,
    val bindings: List<TelegramBinding> = emptyList(),
    val activeBindToken: TelegramBindToken? = null,
    val error: String? = null,
    val info: String? = null,
)

class TelegramViewModel(private val container: AppContainer) : ViewModel() {
    private val _state = MutableStateFlow(TelegramUiState())
    val state: StateFlow<TelegramUiState> = _state.asStateFlow()

    init { refresh() }

    fun refresh() {
        _state.value = _state.value.copy(loading = true, error = null)
        viewModelScope.launch {
            val r = container.telegramRepository.bindings()
            _state.value = when (r) {
                is Result.Success -> _state.value.copy(loading = false, bindings = r.data)
                is Result.Error -> _state.value.copy(loading = false, error = r.message)
            }
        }
    }

    fun startBind() {
        _state.value = _state.value.copy(loading = true, error = null, info = null)
        viewModelScope.launch {
            val r = container.telegramRepository.createBindToken()
            _state.value = when (r) {
                is Result.Success -> _state.value.copy(loading = false, activeBindToken = r.data)
                is Result.Error -> _state.value.copy(loading = false, error = r.message)
            }
        }
    }

    fun checkBindStatus() {
        val token = _state.value.activeBindToken?.token ?: return
        viewModelScope.launch {
            val r = container.telegramRepository.bindStatus(token)
            if (r is Result.Success && r.data.status == "bound") {
                _state.value = _state.value.copy(activeBindToken = null, info = "绑定成功 ✓")
                refresh()
            }
        }
    }

    fun unbind(id: Long) {
        viewModelScope.launch {
            val r = container.telegramRepository.unbind(id)
            if (r is Result.Success) refresh()
            else if (r is Result.Error) _state.value = _state.value.copy(error = r.message)
        }
    }

    fun sendTest(id: Long) {
        viewModelScope.launch {
            val r = container.telegramRepository.sendTest(id)
            _state.value = when (r) {
                is Result.Success -> _state.value.copy(info = "测试消息已发送")
                is Result.Error -> _state.value.copy(error = r.message)
            }
        }
    }

    fun clearError() {
        _state.value = _state.value.copy(error = null)
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = TelegramViewModel(container) as T
    }
}

// ============== Stats ==============

data class StatsUiState(
    val loading: Boolean = false,
    val summary: StatsSummaryDto? = null,
    val error: String? = null,
)

class StatsViewModel(private val container: AppContainer) : ViewModel() {
    private val _state = MutableStateFlow(StatsUiState())
    val state: StateFlow<StatsUiState> = _state.asStateFlow()

    init { refresh() }

    fun refresh() {
        _state.value = _state.value.copy(loading = true, error = null)
        viewModelScope.launch {
            val r = container.statsRepository.summary()
            _state.value = when (r) {
                is Result.Success -> _state.value.copy(loading = false, summary = r.data)
                is Result.Error -> _state.value.copy(loading = false, error = r.message)
            }
        }
    }

    fun clearError() {
        _state.value = _state.value.copy(error = null)
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = StatsViewModel(container) as T
    }
}

// ============== Pomodoro ==============

data class PomodoroUiState(
    val active: PomodoroSessionDto? = null,
    val recent: List<PomodoroSessionDto> = emptyList(),
    val error: String? = null,
    val plannedMinutes: Int = 25,
    val kind: String = "focus",
)

class PomodoroViewModel(private val container: AppContainer) : ViewModel() {
    private val _state = MutableStateFlow(PomodoroUiState())
    val state: StateFlow<PomodoroUiState> = _state.asStateFlow()

    init { refresh() }

    fun refresh() {
        viewModelScope.launch {
            val r = container.pomodoroRepository.list(20)
            if (r is Result.Success) {
                _state.value = _state.value.copy(
                    recent = r.data,
                    active = r.data.firstOrNull { it.status == "active" },
                )
            } else if (r is Result.Error) {
                _state.value = _state.value.copy(error = r.message)
            }
        }
    }

    fun setPlanned(min: Int) { _state.value = _state.value.copy(plannedMinutes = min) }
    fun setKind(k: String) { _state.value = _state.value.copy(kind = k) }

    fun start() {
        viewModelScope.launch {
            val r = container.pomodoroRepository.start(
                plannedSeconds = _state.value.plannedMinutes * 60,
                kind = _state.value.kind, todoId = null, note = "",
            )
            if (r is Result.Success) _state.value = _state.value.copy(active = r.data)
            else if (r is Result.Error) _state.value = _state.value.copy(error = r.message)
            refresh()
        }
    }

    fun complete() {
        val id = _state.value.active?.id ?: return
        viewModelScope.launch {
            container.pomodoroRepository.complete(id)
            _state.value = _state.value.copy(active = null)
            refresh()
        }
    }

    fun abandon() {
        val id = _state.value.active?.id ?: return
        viewModelScope.launch {
            container.pomodoroRepository.abandon(id)
            _state.value = _state.value.copy(active = null)
            refresh()
        }
    }

    fun clearError() {
        _state.value = _state.value.copy(error = null)
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = PomodoroViewModel(container) as T
    }
}

// ============== Settings ==============

data class SettingsUiState(
    val email: String = "",
    val timezone: String = "Asia/Shanghai",
    val systemTimezone: String = "Asia/Shanghai",
    val timezoneSaving: Boolean = false,
    val displayName: String = "",
    val prefs: AndroidPrefs = PreferenceRepository.DEFAULTS,
    val prefsLoading: Boolean = false,
    val error: String? = null,
    val updateChecking: Boolean = false,
    val updateHasNew: Boolean? = null,
    val updateVersion: String? = null,
    val updateUrl: String? = null,
    val updateNotes: String? = null,
    val updateError: String? = null,
    val updateDialog: UpdateDialogUiState? = null,
) {
    val shouldSuggestSystemTimezone: Boolean
        get() = systemTimezone.isNotBlank() && timezone != systemTimezone
}

data class UpdateDialogUiState(
    val hasNew: Boolean,
    val version: String?,
    val url: String?,
    val notes: String?,
    val error: String? = null,
)

fun SettingsUiState.withUpdateDialog(
    hasNew: Boolean,
    version: String?,
    url: String?,
    notes: String?,
    error: String? = null,
): SettingsUiState = copy(
    updateChecking = false,
    updateHasNew = null,
    updateVersion = null,
    updateUrl = null,
    updateNotes = null,
    updateError = null,
    updateDialog = UpdateDialogUiState(
        hasNew = hasNew,
        version = version,
        url = url,
        notes = notes,
        error = error,
    ),
)

fun SettingsUiState.dismissUpdateDialog(): SettingsUiState = copy(updateDialog = null)

class SettingsViewModel(private val container: AppContainer) : ViewModel() {
    private val _state = MutableStateFlow(loadInitial())
    val state: StateFlow<SettingsUiState> = _state.asStateFlow()

    init {
        // 启动时把本地缓存的 prefs 立刻铺上去,然后异步从服务端拉权威值
        _state.value = _state.value.copy(prefs = container.preferenceRepository.current())
        refreshPrefs()
    }

    private fun loadInitial(): SettingsUiState {
        val s = container.tokenManager.current()
        return SettingsUiState(
            email = s.userEmail ?: "",
            timezone = s.timezone,
            systemTimezone = ZoneId.systemDefault().id,
            displayName = "",
            prefs = container.preferenceRepository.current(),
        )
    }

    fun setDisplayName(v: String) { _state.value = _state.value.copy(displayName = v) }

    fun refreshPrefs() {
        _state.value = _state.value.copy(prefsLoading = true)
        viewModelScope.launch {
            container.preferenceRepository.refresh()
            _state.value = _state.value.copy(
                prefs = container.preferenceRepository.current(),
                prefsLoading = false,
            )
        }
    }

    /** 通用单字段切换。UI 传入"如何在 AndroidPrefs 上改"的函数。 */
    fun togglePref(transform: (AndroidPrefs) -> AndroidPrefs) {
        viewModelScope.launch {
            val r = container.preferenceRepository.set(transform)
            _state.value = _state.value.copy(prefs = container.preferenceRepository.current())
            if (r is Result.Error) {
                _state.value = _state.value.copy(error = "偏好同步失败: ${r.message}(已本地保存)")
            }
        }
    }

    fun setAutoSnooze(minutes: Int) {
        val clamped = minutes.coerceIn(1, 60)
        togglePref { it.copy(autoSnoozeMinutes = clamped) }
    }

    fun syncSystemTimezone() {
        val tz = _state.value.systemTimezone.ifBlank { "Asia/Shanghai" }
        _state.value = _state.value.copy(timezoneSaving = true, error = null)
        viewModelScope.launch {
            val r = container.authRepository.updateMe(timezone = tz)
            _state.value = when (r) {
                is Result.Success -> _state.value.copy(
                    email = r.data.email,
                    timezone = r.data.timezone,
                    timezoneSaving = false,
                )
                is Result.Error -> _state.value.copy(
                    timezoneSaving = false,
                    error = "时区同步失败: ${r.message}",
                )
            }
        }
    }

    fun checkUpdate() {
        _state.value = _state.value.copy(
            updateChecking = true,
            updateError = null,
            updateHasNew = null,
            updateDialog = null,
        )
        viewModelScope.launch {
            try {
                val baseUrl = container.apiClient.currentBase()
                val url = "${baseUrl.trimEnd('/')}/downloads/latest.json"
                val result = kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
                    val conn = java.net.URL(url).openConnection() as java.net.HttpURLConnection
                    conn.connectTimeout = 8000
                    conn.readTimeout = 8000
                    try {
                        val body = conn.inputStream.bufferedReader().readText()
                        val json = org.json.JSONObject(body)
                        val android = json.getJSONObject("android")
                        Triple(android.getString("version"), android.optString("filename", ""), android.optString("notes", ""))
                    } finally { conn.disconnect() }
                }
                val (remoteVersion, filename, notes) = result
                val hasNew = compareVersions(remoteVersion, "1.3.0") > 0
                _state.value = _state.value.withUpdateDialog(
                    hasNew = hasNew,
                    version = remoteVersion,
                    url = if (hasNew && filename.isNotBlank()) "${baseUrl.trimEnd('/')}/downloads/android/$filename" else null,
                    notes = notes.ifBlank { if (hasNew) "发现可安装的新版本。" else "当前已是最新版本。" },
                )
            } catch (e: Exception) {
                val msg = e.message ?: "检测失败"
                _state.value = _state.value.withUpdateDialog(
                    hasNew = false,
                    version = null,
                    url = null,
                    notes = msg,
                    error = msg,
                )
            }
        }
    }

    fun dismissUpdateDialog() {
        _state.value = _state.value.dismissUpdateDialog()
    }

    fun clearError() {
        _state.value = _state.value.copy(error = null)
    }

    private fun compareVersions(a: String, b: String): Int {
        val pa = a.split(".").map { it.toIntOrNull() ?: 0 }
        val pb = b.split(".").map { it.toIntOrNull() ?: 0 }
        for (i in 0 until maxOf(pa.size, pb.size)) {
            val va = pa.getOrElse(i) { 0 }; val vb = pb.getOrElse(i) { 0 }
            if (va != vb) return va - vb
        }
        return 0
    }

    fun logout(onDone: () -> Unit) {
        viewModelScope.launch {
            container.authRepository.logout()
            onDone()
        }
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = SettingsViewModel(container) as T
    }
}
