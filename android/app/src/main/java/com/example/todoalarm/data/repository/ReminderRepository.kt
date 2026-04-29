package com.example.todoalarm.data.repository

import com.example.todoalarm.alarm.AlarmScheduler
import com.example.todoalarm.data.auth.TokenManager
import com.example.todoalarm.data.local.AppDatabase
import com.example.todoalarm.data.local.LocalAlarmLogEntity
import com.example.todoalarm.data.local.ReminderCacheEntity
import com.example.todoalarm.data.remote.ApiClient
import com.example.todoalarm.data.remote.ReminderDto
import com.example.todoalarm.data.remote.ReminderInput
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOf
import java.time.Instant

class ReminderRepository(
    private val client: ApiClient,
    private val db: AppDatabase,
    private val tokenManager: TokenManager,
    private val scheduler: AlarmScheduler,
) {
    fun observeAll(): Flow<List<ReminderCacheEntity>> {
        val uid = tokenManager.current().userId ?: return flowOf(emptyList())
        return db.reminderDao().activeForUser(uid)
    }

    suspend fun refreshAll(): Result<List<ReminderDto>> {
        val r = safeCall(client.moshi) { client.api.remindersList() }
        if (r is Result.Success) {
            val items = r.data.items.orEmpty()
            cacheUpsert(items)
            // 重新排所有规则。在 boot 后 / 登录后 / refresh 时会被调用
            tokenManager.current().userId?.let { uid -> scheduler.rescheduleAll(uid) }
        }
        return r.map { it.items.orEmpty() }
    }

    suspend fun create(input: ReminderInput): Result<ReminderDto> {
        val r = safeCall(client.moshi) { client.api.reminderCreate(input) }
        if (r is Result.Success) {
            cacheUpsert(listOf(r.data))
            scheduler.schedule(toEntity(r.data))
        }
        return r
    }

    suspend fun update(id: Long, input: ReminderInput): Result<ReminderDto> {
        val r = safeCall(client.moshi) { client.api.reminderUpdate(id, input) }
        if (r is Result.Success) {
            cacheUpsert(listOf(r.data))
            scheduler.schedule(toEntity(r.data))
        }
        return r
    }

    suspend fun delete(id: Long): Result<Unit> {
        val r = safeCall(client.moshi) { client.api.reminderDelete(id) }
        if (r is Result.Success) {
            db.reminderDao().deleteById(id)
            scheduler.cancel(id)
        }
        return r
    }

    suspend fun setEnabled(id: Long, enabled: Boolean): Result<ReminderDto> {
        val r = if (enabled) safeCall(client.moshi) { client.api.reminderEnable(id) }
        else safeCall(client.moshi) { client.api.reminderDisable(id) }
        if (r is Result.Success) {
            cacheUpsert(listOf(r.data))
            if (enabled) scheduler.schedule(toEntity(r.data)) else scheduler.cancel(id)
        }
        return r
    }

    /**
     * 由 AlarmActivity 调用,响铃时用户点了"完成任务"。
     * 离线 → 返回 false,Activity 那边只停响铃 + 提示用户联网后重试。
     * 在线 → 把绑定的 todo 标记完成,返回 true。
     */
    suspend fun tryCompleteFromAlarm(ruleId: Long): Boolean {
        val cached = db.reminderDao().byId(ruleId) ?: return false
        cached.next_fire_at?.let { iso ->
            try {
                db.alarmLogDao().ack(ruleId, iso, Instant.now().toString())
            } catch (_: Exception) { }
        }
        val todoId = cached.todo_id ?: return true   // 没绑定 todo,本地确认即可
        val r = safeCall(client.moshi) { client.api.todoComplete(todoId) }
        return r.isSuccess
    }

    /** 把本次响铃记入 local_alarm_log 用于幂等 */
    suspend fun logFire(ruleId: Long, fireAtIso: String) {
        try {
            db.alarmLogDao().logFire(LocalAlarmLogEntity(
                rule_id = ruleId, fire_at = fireAtIso,
                fired_at = Instant.now().toString(), acked_at = null,
            ))
        } catch (_: Exception) { }
    }

    private suspend fun cacheUpsert(items: List<ReminderDto>) {
        if (items.isEmpty()) return
        db.reminderDao().upsert(items.map { toEntity(it) })
    }

    private fun toEntity(r: ReminderDto) = ReminderCacheEntity(
        id = r.id, user_id = r.user_id, todo_id = r.todo_id, title = r.title,
        trigger_at = r.trigger_at, rrule = r.rrule, dtstart = r.dtstart, timezone = r.timezone,
        channel_local = r.channel_local, channel_telegram = r.channel_telegram,
        is_enabled = r.is_enabled, next_fire_at = r.next_fire_at, last_fired_at = r.last_fired_at,
        ringtone = r.ringtone, vibrate = r.vibrate, fullscreen = r.fullscreen,
        scheduled_for = null,    // schedule() 会写入
        created_at = r.created_at, updated_at = r.updated_at,
    )
}
