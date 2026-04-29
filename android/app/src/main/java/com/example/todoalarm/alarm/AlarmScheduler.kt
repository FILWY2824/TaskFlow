package com.example.todoalarm.alarm

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import android.util.Log
import com.example.todoalarm.data.local.ReminderCacheEntity
import com.example.todoalarm.data.local.ReminderDao
import java.time.Instant

/**
 * AlarmScheduler 是 Android 端本地强提醒的核心。
 *
 * 职责:
 *   1. 把同步到本地的 ReminderCacheEntity.next_fire_at 注册成精确闹钟。
 *   2. 重新同步时,对比 scheduled_for 与最新 next_fire_at,只在变了的时候改注册。
 *   3. 重启 / 重新登录后,从本地 Room 重新拉一遍 active 提醒并全量重排。
 *
 * 关键决策:
 *   - 用 setExactAndAllowWhileIdle(),Doze 模式下也会触发,但有最低 9 分钟节流;
 *     对于"半年体检"这类提醒可以接受。极敏感场景可以换 setAlarmClock(),代价是
 *     状态栏出现一个闹钟图标。
 *   - PendingIntent 的 requestCode 用 reminderId.toInt(),保证按规则去重;FLAG_UPDATE_CURRENT
 *     让重新注册自动覆盖旧的。
 *   - cancel(rule) 用同一组 requestCode + FLAG_NO_CREATE 拿现有 PI 来取消;Android 13+
 *     必须 PendingIntent.FLAG_IMMUTABLE。
 */
class AlarmScheduler(
    private val context: Context,
    private val reminderDao: ReminderDao,
) {
    private val alarmManager: AlarmManager =
        context.getSystemService(Context.ALARM_SERVICE) as AlarmManager

    /** Schedule (or reschedule) a single rule. Idempotent. */
    suspend fun schedule(rule: ReminderCacheEntity) {
        val nextIso = rule.next_fire_at
        if (!rule.is_enabled || !rule.channel_local || nextIso == null) {
            cancel(rule.id)
            reminderDao.setScheduledFor(rule.id, null)
            return
        }
        val triggerMillis = try {
            Instant.parse(nextIso).toEpochMilli()
        } catch (e: Exception) {
            Log.w(TAG, "rule ${rule.id} has bad next_fire_at: $nextIso", e)
            return
        }
        val now = System.currentTimeMillis()
        if (triggerMillis < now - 60_000L) {
            // 已经过期超过 1 分钟,跳过(避免一次性补很多积压闹钟)
            Log.d(TAG, "rule ${rule.id} next_fire_at in past by ${(now - triggerMillis)/1000}s, skipping")
            cancel(rule.id)
            reminderDao.setScheduledFor(rule.id, null)
            return
        }
        if (rule.scheduled_for == nextIso) {
            // 已经按相同时间注册过
            return
        }

        val pi = makePendingIntent(rule.id, rule.title, nextIso, fullscreen = rule.fullscreen, vibrate = rule.vibrate)

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            // Android 12+ 检查精确闹钟许可
            if (!alarmManager.canScheduleExactAlarms()) {
                Log.w(TAG, "no SCHEDULE_EXACT_ALARM permission; falling back to setAndAllowWhileIdle")
                alarmManager.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, triggerMillis, pi)
            } else {
                alarmManager.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, triggerMillis, pi)
            }
        } else {
            alarmManager.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, triggerMillis, pi)
        }
        reminderDao.setScheduledFor(rule.id, nextIso)
        Log.i(TAG, "scheduled rule ${rule.id} for $nextIso")
    }

    /** Cancel any registered alarm for this rule. */
    fun cancel(ruleId: Long) {
        val intent = Intent(context, AlarmReceiver::class.java)
        val pi = PendingIntent.getBroadcast(
            context,
            ruleId.toInt(),
            intent,
            PendingIntent.FLAG_NO_CREATE or PendingIntent.FLAG_IMMUTABLE,
        )
        if (pi != null) {
            alarmManager.cancel(pi)
            pi.cancel()
        }
    }

    /** Re-schedule every active rule. Used on boot, on login, after a full resync. */
    suspend fun rescheduleAll(userId: Long) {
        val rules = reminderDao.localScheduled(userId)
        Log.i(TAG, "rescheduleAll: ${rules.size} rule(s) for user $userId")
        for (rule in rules) {
            schedule(rule)
        }
    }

    private fun makePendingIntent(
        ruleId: Long,
        title: String,
        fireAtIso: String,
        fullscreen: Boolean,
        vibrate: Boolean,
    ): PendingIntent {
        val intent = Intent(context, AlarmReceiver::class.java).apply {
            action = ACTION_FIRE
            putExtra(EXTRA_RULE_ID, ruleId)
            putExtra(EXTRA_TITLE, title)
            putExtra(EXTRA_FIRE_AT, fireAtIso)
            putExtra(EXTRA_FULLSCREEN, fullscreen)
            putExtra(EXTRA_VIBRATE, vibrate)
        }
        return PendingIntent.getBroadcast(
            context,
            ruleId.toInt(),
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    companion object {
        private const val TAG = "AlarmScheduler"
        const val ACTION_FIRE = "com.example.todoalarm.ACTION_FIRE"
        const val EXTRA_RULE_ID = "rule_id"
        const val EXTRA_TITLE = "title"
        const val EXTRA_FIRE_AT = "fire_at"
        const val EXTRA_FULLSCREEN = "fullscreen"
        const val EXTRA_VIBRATE = "vibrate"
    }
}
