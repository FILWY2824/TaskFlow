package com.example.taskflow.util

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.provider.AlarmClock
import android.util.Log
import java.time.Instant
import java.time.ZoneId
import java.time.ZonedDateTime

/**
 * 把一个 reminder 同步成系统"时钟"应用里的一条闹钟。
 *
 * 这是 Android 上最稳的"绝不漏响"兜底机制 —— 系统时钟由厂商深度白名单,
 * 不会受 Doze、自启动管控、电池优化的影响。代价是:
 *   1. 每条 reminder 都会让用户看到一次"打开时钟" 的系统跳转(IME 上,我们用 ACTION_SET_ALARM
 *      可以设置 EXTRA_SKIP_UI 跳过 UI,但是有些 ROM 仍然会显示一次提示);
 *   2. 用户能在时钟应用里手动改/删,客户端无法感知;
 *   3. AlarmClock API 没有"按 ID 删除"的能力,所以重新创建提醒时只是新增一条,
 *      旧的需要用户去时钟应用清理 —— 这个限制在 README 里要说明。
 *
 * 因此默认关闭(AndroidPrefs.useSystemAlarmClock 默认 false)。
 * 用户在 Settings 里勾上之后,reminder 创建/更新时才会同步系统时钟。
 *
 * 所有调用都被 try-catch 包裹:某些 ROM 没装时钟应用 / 拒绝 ACTION_SET_ALARM 时
 * 不能让上层流程崩溃 —— 主路径(AlarmManager + Foreground Service)依然会触发。
 */
object SystemAlarmClock {

    private const val TAG = "SystemAlarmClock"

    /**
     * 在系统时钟里添加一条单次闹钟。triggerEpochMs 是绝对时间(UTC ms)。
     * 注意:AlarmClock.ACTION_SET_ALARM 只能设"今天/明天的某个 HH:mm",
     * 不能直接设具体的远期日期 —— 我们把时间转换成 (HOUR, MINUTES) 后下发,
     * 系统时钟会按"下一个匹配点"叫醒用户。
     *
     * 对于较远的提醒(超过 24h),建议同时保留服务端 + AlarmManager 双保险,
     * 系统时钟只承担"近期"那部分。
     */
    fun setAlarm(ctx: Context, triggerEpochMs: Long, message: String, zoneId: ZoneId): Boolean {
        val zdt: ZonedDateTime = try {
            Instant.ofEpochMilli(triggerEpochMs).atZone(zoneId)
        } catch (e: Exception) {
            Log.w(TAG, "bad triggerEpochMs=$triggerEpochMs: $e")
            return false
        }
        val intent = Intent(AlarmClock.ACTION_SET_ALARM).apply {
            putExtra(AlarmClock.EXTRA_HOUR, zdt.hour)
            putExtra(AlarmClock.EXTRA_MINUTES, zdt.minute)
            putExtra(AlarmClock.EXTRA_MESSAGE, message.take(80))
            putExtra(AlarmClock.EXTRA_SKIP_UI, true)   // 大多数厂商 ROM 会尊重,跳过弹窗
            putExtra(AlarmClock.EXTRA_VIBRATE, true)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        return try {
            ctx.startActivity(intent)
            true
        } catch (e: ActivityNotFoundException) {
            Log.w(TAG, "no clock app to handle ACTION_SET_ALARM")
            false
        } catch (e: Exception) {
            Log.w(TAG, "setAlarm failed: $e")
            false
        }
    }

    /**
     * 打开系统时钟应用展示已有闹钟列表,方便用户手动清理过期条目。
     * 用户想"清空 TaskFlow 加进去的"系统闹钟时,在 Settings 里点这个按钮跳过去。
     */
    fun openClockApp(ctx: Context): Boolean {
        val intent = Intent(AlarmClock.ACTION_SHOW_ALARMS)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        return try {
            ctx.startActivity(intent)
            true
        } catch (e: ActivityNotFoundException) {
            false
        } catch (e: Exception) {
            Log.w(TAG, "openClockApp failed: $e")
            false
        }
    }
}
