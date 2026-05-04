package com.example.taskflow.data.repository

import android.content.Context
import android.content.SharedPreferences
import com.example.taskflow.data.remote.ApiClient
import com.example.taskflow.data.remote.PreferenceBulkRequest
import com.example.taskflow.data.remote.PreferenceDto
import com.example.taskflow.data.remote.PreferenceItem
import com.example.taskflow.data.remote.PreferencePutRequest
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * 跨端用户偏好仓库(Android 端,规格 §17 阶段 13)。
 *
 * 设计:
 *  - source-of-truth 是服务端 user_preferences。本仓库读 scope='android' 下的全量偏好,
 *    并把布尔 / 整数等业务语义封装在 [AndroidPrefs] 数据类里。
 *  - 写入是乐观更新:立刻把新值发布到 [state],同时把对应 key 异步推到服务端;
 *    推失败也不回滚,等下次 refresh() 时由服务端校正。
 *  - 同时把最近一次成功的快照写到本地 SharedPreferences("taskflow_prefs_cache"),
 *    在断网启动 / 还没拉到服务端数据时,UI 也能立刻渲染上次保存的状态。
 *  - 这里的"键空间"必须和服务端 handler 校验一致([a-z0-9._-],<=64)。
 *
 * 边界:
 *  - 所有"通知 / 提醒类"开关都 scope='android';它们与 web/windows 各自独立。
 *  - 主题 / 折叠已完成 等跨端共享的偏好统一放在 scope='common' —— 但本阶段先不实现共享,
 *    保留扩展位即可。
 */
class PreferenceRepository(
    context: Context,
    private val client: ApiClient,
) {
    private val cache: SharedPreferences =
        context.getSharedPreferences("taskflow_prefs_cache", Context.MODE_PRIVATE)

    private val _state = MutableStateFlow(loadFromCache())
    val state: StateFlow<AndroidPrefs> = _state.asStateFlow()

    /** 当前快照(同步读)。 */
    fun current(): AndroidPrefs = _state.value

    /**
     * 拉服务端的 scope='android' 偏好,与 [DEFAULTS] 合并后发布到 [state] 并落本地缓存。
     * 失败时静默(保留上次缓存)。
     */
    suspend fun refresh(): Result<Unit> {
        val r = safeCall(client.moshi) { client.api.preferencesList(scope = SCOPE) }
        return when (r) {
            is Result.Success -> {
                val byKey = r.data.items.associateBy { it.key }
                val next = DEFAULTS.copy(
                    fullScreenAlarm  = readBool(byKey, K_FULL_SCREEN, DEFAULTS.fullScreenAlarm),
                    vibrate          = readBool(byKey, K_VIBRATE, DEFAULTS.vibrate),
                    inAppToast       = readBool(byKey, K_IN_APP_TOAST, DEFAULTS.inAppToast),
                    todoDueLocalAlarm= readBool(byKey, K_TODO_DUE, DEFAULTS.todoDueLocalAlarm),
                    pomodoroSound    = readBool(byKey, K_POMO_SOUND, DEFAULTS.pomodoroSound),
                    pomodoroAutoComplete = readBool(byKey, K_POMO_AUTO, DEFAULTS.pomodoroAutoComplete),
                    useSystemAlarmClock = readBool(byKey, K_SYS_CLOCK, DEFAULTS.useSystemAlarmClock),
                    autoSnoozeMinutes   = readInt(byKey, K_AUTO_SNOOZE, DEFAULTS.autoSnoozeMinutes),
                )
                _state.value = next
                saveToCache(next)
                Result.Success(Unit)
            }
            is Result.Error -> r
        }
    }

    /** 单条更新(乐观)。返回 Success(Unit) 表示本地已应用,无论服务端推送是否成功。 */
    suspend fun set(transform: (AndroidPrefs) -> AndroidPrefs): Result<Unit> {
        val next = transform(_state.value)
        if (next == _state.value) return Result.Success(Unit)
        _state.value = next
        saveToCache(next)
        // 把所有改动一次性推上去(简单可靠,反正 payload 很小)
        val items = listOf(
            PreferenceItem(SCOPE, K_FULL_SCREEN,  bool(next.fullScreenAlarm)),
            PreferenceItem(SCOPE, K_VIBRATE,      bool(next.vibrate)),
            PreferenceItem(SCOPE, K_IN_APP_TOAST, bool(next.inAppToast)),
            PreferenceItem(SCOPE, K_TODO_DUE,     bool(next.todoDueLocalAlarm)),
            PreferenceItem(SCOPE, K_POMO_SOUND,   bool(next.pomodoroSound)),
            PreferenceItem(SCOPE, K_POMO_AUTO,    bool(next.pomodoroAutoComplete)),
            PreferenceItem(SCOPE, K_SYS_CLOCK,    bool(next.useSystemAlarmClock)),
            PreferenceItem(SCOPE, K_AUTO_SNOOZE,  next.autoSnoozeMinutes.toString()),
        )
        return safeCall(client.moshi) { client.api.preferencesBulk(PreferenceBulkRequest(items)) }
            .map { Unit }
    }

    /** 把单一键直接写到服务端(用于不希望批量重发的场景)。 */
    suspend fun putOne(key: String, value: String): Result<PreferenceDto> =
        safeCall(client.moshi) { client.api.preferencePut(SCOPE, key, PreferencePutRequest(value)) }

    // ---------- 内部 ----------

    private fun readBool(map: Map<String, PreferenceDto>, key: String, default: Boolean): Boolean {
        val v = map[key]?.value ?: return default
        return v == "1" || v == "true"
    }
    private fun readInt(map: Map<String, PreferenceDto>, key: String, default: Int): Int {
        val v = map[key]?.value ?: return default
        return v.toIntOrNull() ?: default
    }
    private fun bool(b: Boolean): String = if (b) "1" else "0"

    private fun loadFromCache(): AndroidPrefs = AndroidPrefs(
        fullScreenAlarm     = cache.getBoolean(K_FULL_SCREEN, DEFAULTS.fullScreenAlarm),
        vibrate             = cache.getBoolean(K_VIBRATE, DEFAULTS.vibrate),
        inAppToast          = cache.getBoolean(K_IN_APP_TOAST, DEFAULTS.inAppToast),
        todoDueLocalAlarm   = cache.getBoolean(K_TODO_DUE, DEFAULTS.todoDueLocalAlarm),
        pomodoroSound       = cache.getBoolean(K_POMO_SOUND, DEFAULTS.pomodoroSound),
        pomodoroAutoComplete= cache.getBoolean(K_POMO_AUTO, DEFAULTS.pomodoroAutoComplete),
        useSystemAlarmClock = cache.getBoolean(K_SYS_CLOCK, DEFAULTS.useSystemAlarmClock),
        autoSnoozeMinutes   = cache.getInt(K_AUTO_SNOOZE, DEFAULTS.autoSnoozeMinutes),
    )

    private fun saveToCache(p: AndroidPrefs) {
        cache.edit().apply {
            putBoolean(K_FULL_SCREEN, p.fullScreenAlarm)
            putBoolean(K_VIBRATE, p.vibrate)
            putBoolean(K_IN_APP_TOAST, p.inAppToast)
            putBoolean(K_TODO_DUE, p.todoDueLocalAlarm)
            putBoolean(K_POMO_SOUND, p.pomodoroSound)
            putBoolean(K_POMO_AUTO, p.pomodoroAutoComplete)
            putBoolean(K_SYS_CLOCK, p.useSystemAlarmClock)
            putInt(K_AUTO_SNOOZE, p.autoSnoozeMinutes)
            apply()
        }
    }

    companion object {
        const val SCOPE = "android"

        const val K_FULL_SCREEN  = "notification.full_screen_alarm"
        const val K_VIBRATE      = "notification.vibrate"
        const val K_IN_APP_TOAST = "notification.in_app_toast"
        const val K_TODO_DUE     = "notification.todo_due_local_alarm"
        const val K_POMO_SOUND   = "pomodoro.sound"
        const val K_POMO_AUTO    = "pomodoro.auto_complete"
        const val K_SYS_CLOCK    = "notification.use_system_alarm_clock"
        const val K_AUTO_SNOOZE  = "notification.auto_snooze_minutes"

        val DEFAULTS = AndroidPrefs(
            fullScreenAlarm = true,
            vibrate = true,
            inAppToast = true,
            todoDueLocalAlarm = true,
            pomodoroSound = true,
            pomodoroAutoComplete = true,
            useSystemAlarmClock = false,
            autoSnoozeMinutes = 10,
        )
    }
}

/**
 * Android 端独有的偏好集合(scope='android' 投影)。
 *
 * 字段说明:
 *  - fullScreenAlarm      到点时是否拉起 AlarmActivity 锁屏全屏(系统级 USE_FULL_SCREEN_INTENT)
 *  - vibrate              强提醒时是否震动
 *  - inAppToast           应用前台时是否额外弹出应用内 toast
 *  - todoDueLocalAlarm    任务开始时间到点是否本地弹窗(独立于服务端 reminder)
 *  - pomodoroSound        番茄到点是否响铃
 *  - pomodoroAutoComplete 番茄到点是否自动结束并入库
 *  - useSystemAlarmClock  在创建 reminder 时,是否同时通过 AlarmClock.ACTION_SET_ALARM
 *                         往系统时钟应用里写一条;最稳的"一定不漏"兜底,代价是要离开 App 一次。
 *  - autoSnoozeMinutes    强提醒响铃多少分钟后没人响应自动停止(避免占用扬声器无限循环)
 */
data class AndroidPrefs(
    val fullScreenAlarm: Boolean,
    val vibrate: Boolean,
    val inAppToast: Boolean,
    val todoDueLocalAlarm: Boolean,
    val pomodoroSound: Boolean,
    val pomodoroAutoComplete: Boolean,
    val useSystemAlarmClock: Boolean,
    val autoSnoozeMinutes: Int,
)
