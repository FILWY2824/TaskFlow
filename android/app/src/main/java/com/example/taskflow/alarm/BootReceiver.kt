package com.example.taskflow.alarm

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log
import com.example.taskflow.TaskFlowApp
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

/**
 * 设备重启 / 应用升级后,所有 AlarmManager 注册都会丢失。
 * 在以下事件后重新拉一遍本地缓存,重新注册所有 active 提醒:
 *   - BOOT_COMPLETED
 *   - LOCKED_BOOT_COMPLETED (设备 still encrypted, 在用户解锁前;但本应用未启用 Direct Boot Aware,
 *     这条到达时实际上还没解锁;系统会再发一次普通 BOOT_COMPLETED,我们以后者为准)
 *   - MY_PACKAGE_REPLACED   (升级)
 */
class BootReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        val action = intent.action ?: return
        Log.i(TAG, "received $action")

        when (action) {
            Intent.ACTION_BOOT_COMPLETED,
            Intent.ACTION_MY_PACKAGE_REPLACED,
            Intent.ACTION_PACKAGE_REPLACED,
            "android.intent.action.LOCKED_BOOT_COMPLETED",
                -> rescheduleAll(context)

            else -> {} // ignore
        }
    }

    private fun rescheduleAll(context: Context) {
        val pendingResult = goAsync()
        val app = context.applicationContext as TaskFlowApp
        val container = app.container
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        scope.launch {
            try {
                val session = container.tokenManager.current()
                val uid = session.userId
                if (uid != null) {
                    container.alarmScheduler.rescheduleAll(uid)
                    Log.i(TAG, "rescheduleAll done for user $uid")
                } else {
                    Log.i(TAG, "no logged-in user; skipping")
                }
            } catch (e: Exception) {
                Log.w(TAG, "rescheduleAll failed: ${e.message}", e)
            } finally {
                pendingResult.finish()
            }
        }
    }

    companion object {
        private const val TAG = "BootReceiver"
    }
}
