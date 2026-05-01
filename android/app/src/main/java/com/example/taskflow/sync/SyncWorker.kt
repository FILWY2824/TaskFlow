package com.example.taskflow.sync

import android.content.Context
import android.util.Log
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.example.taskflow.TaskFlowApp
import com.example.taskflow.alarm.AlarmScheduler
import com.example.taskflow.data.local.AppDatabase
import com.example.taskflow.data.local.SyncMetaEntity
import com.example.taskflow.data.remote.ApiClient
import com.example.taskflow.data.remote.ReminderDto
import com.example.taskflow.data.remote.SyncEvent
import com.example.taskflow.data.repository.ReminderRepository
import com.example.taskflow.data.repository.safeCall
import java.time.Instant
import java.util.concurrent.TimeUnit

/**
 * 增量同步:每 15 分钟一次(WorkManager 的最低周期),也支持手动 enqueue 一次性拉取。
 *
 * 每次:
 *   1. 拉 /api/auth/me。401 → 静默退出(token 过期,UI 会感知)。
 *   2. 从 sync_meta.cursor 开始 /api/sync/pull,直到 has_more = false。
 *   3. 处理事件:
 *        - reminder created/updated -> GET /api/reminders/{id} → upsert + AlarmScheduler.schedule()
 *        - reminder deleted          -> 本地缓存删除 + AlarmScheduler.cancel()
 *        - todo / list / subtask 等  -> 这版先忽略(UI 主动 refresh 拉新数据)
 *   4. 把最新 cursor 写回 sync_meta。
 */
class SyncWorker(appContext: Context, params: WorkerParameters)
    : CoroutineWorker(appContext, params) {

    override suspend fun doWork(): Result {
        val app = applicationContext as TaskFlowApp
        val container = app.container
        val client = container.apiClient
        val db = container.db
        val scheduler = container.alarmScheduler
        val reminderRepo = container.reminderRepository

        val session = container.tokenManager.current()
        val userId = session.userId ?: return Result.success()

        return try {
            // probe me() 看 token 还有效吗
            val me = safeCall(client.moshi) { client.api.me() }
            if (me is com.example.taskflow.data.repository.Result.Error) {
                Log.w(TAG, "me failed: ${me.message}")
                return Result.success()    // 不重试
            }

            var cursor = db.syncMetaDao().getCursor(userId) ?: 0L
            var iterations = 0
            while (true) {
                val r = safeCall(client.moshi) { client.api.syncPull(cursor, 200) }
                if (r is com.example.taskflow.data.repository.Result.Error) {
                    Log.w(TAG, "syncPull failed: ${r.message}")
                    return Result.retry()
                }
                val page = (r as com.example.taskflow.data.repository.Result.Success).data
                val events = page.events.orEmpty()
                for (ev in events) {
                    handleEvent(ev, client, scheduler, reminderRepo)
                }
                cursor = page.next_cursor
                if (!page.has_more) break
                iterations++
                if (iterations > 20) {
                    Log.w(TAG, "too many pages, deferring rest")
                    break
                }
            }
            db.syncMetaDao().upsert(SyncMetaEntity(
                user_id = userId, cursor = cursor, updated_at = Instant.now().toString()
            ))
            Result.success()
        } catch (e: Exception) {
            Log.w(TAG, "sync worker exception", e)
            Result.retry()
        }
    }

    private suspend fun handleEvent(
        ev: SyncEvent,
        client: ApiClient,
        scheduler: AlarmScheduler,
        reminderRepo: ReminderRepository,
    ) {
        when (ev.entity_type) {
            "reminder" -> when (ev.action) {
                "deleted" -> {
                    scheduler.cancel(ev.entity_id)
                    (applicationContext as TaskFlowApp).container.db.reminderDao().deleteById(ev.entity_id)
                }
                else -> {
                    val r = safeCall(client.moshi) { client.api.reminderGet(ev.entity_id) }
                    if (r is com.example.taskflow.data.repository.Result.Success) {
                        // 用 Repository 的私有 path:这里直接借用 createOrUpdate 的副作用麻烦,简单点
                        // — 直接调一个抽出的 helper
                        upsertAndReschedule(r.data, scheduler)
                    }
                }
            }
            // todo / list / subtask / pomodoro 的 push:UI 自己 refresh 即可
            else -> {}
        }
    }

    private suspend fun upsertAndReschedule(dto: ReminderDto, scheduler: AlarmScheduler) {
        val app = applicationContext as TaskFlowApp
        val ent = com.example.taskflow.data.local.ReminderCacheEntity(
            id = dto.id, user_id = dto.user_id, todo_id = dto.todo_id, title = dto.title,
            trigger_at = dto.trigger_at, rrule = dto.rrule, dtstart = dto.dtstart, timezone = dto.timezone,
            channel_local = dto.channel_local, channel_telegram = dto.channel_telegram,
            is_enabled = dto.is_enabled, next_fire_at = dto.next_fire_at,
            last_fired_at = dto.last_fired_at, ringtone = dto.ringtone,
            vibrate = dto.vibrate, fullscreen = dto.fullscreen,
            scheduled_for = null, created_at = dto.created_at, updated_at = dto.updated_at,
        )
        app.container.db.reminderDao().upsertOne(ent)
        scheduler.schedule(ent)
    }

    companion object {
        private const val TAG = "SyncWorker"
        private const val UNIQUE_NAME = "taskflow_sync"

        fun schedulePeriodic(ctx: Context) {
            val req = PeriodicWorkRequestBuilder<SyncWorker>(15, TimeUnit.MINUTES)
                .setConstraints(
                    Constraints.Builder()
                        .setRequiredNetworkType(NetworkType.CONNECTED)
                        .build()
                )
                .build()
            WorkManager.getInstance(ctx).enqueueUniquePeriodicWork(
                UNIQUE_NAME, ExistingPeriodicWorkPolicy.KEEP, req,
            )
        }

        fun cancelPeriodic(ctx: Context) {
            WorkManager.getInstance(ctx).cancelUniqueWork(UNIQUE_NAME)
        }
    }
}
