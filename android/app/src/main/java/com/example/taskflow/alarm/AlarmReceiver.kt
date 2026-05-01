package com.example.taskflow.alarm

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import android.util.Log
import com.example.taskflow.TaskFlowApp

/**
 * 系统精确闹钟到点时,这里被回调。我们尽量轻量地把工作转给:
 *   1. AlarmForegroundService —— 启动响铃 / 振动 / 持锁屏。
 *   2. AlarmActivity —— 弹出全屏强提醒(锁屏可见)。
 *
 * 注意:onReceive 必须在 ~10 秒内返回。我们 startForegroundService + startActivity 后
 * 立刻 return,实际响铃 / UI 都在 service / activity 里跑。
 *
 * 规格 §17:在分发前读一次 AndroidPrefs(用户的全局开关),按以下优先级合成最终行为:
 *   per-rule override(reminder_rule.fullscreen / .vibrate)  ->  AndroidPrefs 全局开关  ->  默认开。
 *
 * 例如:用户在设置里关掉了"震动",即使某条 rule 自己带 vibrate=true 也不再震动 ——
 * 偏好的语义是"我说的算",per-rule 的 true 只是在 rule 的级别上没被屏蔽。
 */
class AlarmReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != AlarmScheduler.ACTION_FIRE) return
        val ruleId = intent.getLongExtra(AlarmScheduler.EXTRA_RULE_ID, -1L)
        val title = intent.getStringExtra(AlarmScheduler.EXTRA_TITLE) ?: "提醒"
        val fireAt = intent.getStringExtra(AlarmScheduler.EXTRA_FIRE_AT) ?: ""
        val ruleFullscreen = intent.getBooleanExtra(AlarmScheduler.EXTRA_FULLSCREEN, true)
        val ruleVibrate = intent.getBooleanExtra(AlarmScheduler.EXTRA_VIBRATE, true)
        if (ruleId <= 0) {
            Log.w(TAG, "fired without rule_id, skipping")
            return
        }

        // 读全局偏好(AndroidPrefs)。
        val prefs = (context.applicationContext as? TaskFlowApp)
            ?.container?.preferenceRepository?.current()
        val effectiveFullscreen = ruleFullscreen && (prefs?.fullScreenAlarm ?: true)
        val effectiveVibrate    = ruleVibrate && (prefs?.vibrate ?: true)
        val autoSnoozeMs        = ((prefs?.autoSnoozeMinutes ?: 10).coerceIn(1, 60)) * 60_000L

        Log.i(TAG, "FIRE rule=$ruleId title=$title fire_at=$fireAt fs=$effectiveFullscreen vib=$effectiveVibrate snooze_ms=$autoSnoozeMs")

        // 1) Foreground service 持锁屏 + 响铃 / 振动
        val svc = Intent(context, AlarmForegroundService::class.java).apply {
            putExtra(AlarmScheduler.EXTRA_RULE_ID, ruleId)
            putExtra(AlarmScheduler.EXTRA_TITLE, title)
            putExtra(AlarmScheduler.EXTRA_FIRE_AT, fireAt)
            putExtra(AlarmScheduler.EXTRA_VIBRATE, effectiveVibrate)
            putExtra(AlarmScheduler.EXTRA_AUTO_SNOOZE_MS, autoSnoozeMs)
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            context.startForegroundService(svc)
        } else {
            context.startService(svc)
        }

        // 2) 全屏 Activity(锁屏可见)
        if (effectiveFullscreen) {
            val ui = Intent(context, AlarmActivity::class.java).apply {
                putExtra(AlarmScheduler.EXTRA_RULE_ID, ruleId)
                putExtra(AlarmScheduler.EXTRA_TITLE, title)
                putExtra(AlarmScheduler.EXTRA_FIRE_AT, fireAt)
                addFlags(
                    Intent.FLAG_ACTIVITY_NEW_TASK or
                        Intent.FLAG_ACTIVITY_CLEAR_TOP or
                        Intent.FLAG_ACTIVITY_NO_USER_ACTION
                )
            }
            context.startActivity(ui)
        }
    }

    companion object {
        private const val TAG = "AlarmReceiver"
    }
}
