package com.example.todoalarm.alarm

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import android.util.Log

/**
 * 系统精确闹钟到点时,这里被回调。我们尽量轻量地把工作转给:
 *   1. AlarmForegroundService —— 启动响铃 / 振动 / 持锁屏。
 *   2. AlarmActivity —— 弹出全屏强提醒(锁屏可见)。
 *
 * 注意:onReceive 必须在 ~10 秒内返回。我们 startForegroundService + startActivity 后
 * 立刻 return,实际响铃 / UI 都在 service / activity 里跑。
 */
class AlarmReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != AlarmScheduler.ACTION_FIRE) return
        val ruleId = intent.getLongExtra(AlarmScheduler.EXTRA_RULE_ID, -1L)
        val title = intent.getStringExtra(AlarmScheduler.EXTRA_TITLE) ?: "提醒"
        val fireAt = intent.getStringExtra(AlarmScheduler.EXTRA_FIRE_AT) ?: ""
        val fullscreen = intent.getBooleanExtra(AlarmScheduler.EXTRA_FULLSCREEN, true)
        val vibrate = intent.getBooleanExtra(AlarmScheduler.EXTRA_VIBRATE, true)
        if (ruleId <= 0) {
            Log.w(TAG, "fired without rule_id, skipping")
            return
        }
        Log.i(TAG, "FIRE rule=$ruleId title=$title fire_at=$fireAt")

        // 1) Foreground service 持锁屏 + 响铃 / 振动
        val svc = Intent(context, AlarmForegroundService::class.java).apply {
            putExtra(AlarmScheduler.EXTRA_RULE_ID, ruleId)
            putExtra(AlarmScheduler.EXTRA_TITLE, title)
            putExtra(AlarmScheduler.EXTRA_FIRE_AT, fireAt)
            putExtra(AlarmScheduler.EXTRA_VIBRATE, vibrate)
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            context.startForegroundService(svc)
        } else {
            context.startService(svc)
        }

        // 2) 全屏 Activity(锁屏可见)
        if (fullscreen) {
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
