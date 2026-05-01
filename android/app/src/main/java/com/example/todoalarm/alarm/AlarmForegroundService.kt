package com.example.todoalarm.alarm

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.media.AudioAttributes
import android.media.MediaPlayer
import android.media.RingtoneManager
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.PowerManager
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import android.util.Log
import androidx.core.app.NotificationCompat

/**
 * 在响铃期间持有 Foreground Service,目的:
 *   1. 防止系统在 Doze 下杀掉响铃 / Activity。
 *   2. 持有 wake lock 让屏幕保持亮(全屏 Activity 自身也会 keepScreenOn)。
 *   3. 用 MediaPlayer 循环播放系统默认 alarm 铃声。
 *   4. 播放振动模式。
 *   5. 安全网:90 秒后自动停止,避免无人值守时无限响铃 / 耗电。
 *
 * Service 通过 ACTION_STOP 接收停止信号(从 AlarmActivity 的"停止"按钮或通知 action)。
 */
class AlarmForegroundService : Service() {

    private var mediaPlayer: MediaPlayer? = null
    private var wakeLock: PowerManager.WakeLock? = null
    private var safetyHandler: Handler? = null
    private var safetyRunnable: Runnable? = null

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        ensureChannel(this)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopAlarmAndService()
            return START_NOT_STICKY
        }

        val ruleId = intent?.getLongExtra(AlarmScheduler.EXTRA_RULE_ID, -1L) ?: -1L
        val title = intent?.getStringExtra(AlarmScheduler.EXTRA_TITLE) ?: "提醒"
        val vibrate = intent?.getBooleanExtra(AlarmScheduler.EXTRA_VIBRATE, true) ?: true

        val notif = buildPersistentNotification(this, ruleId, title)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(NOTIF_ID_ONGOING, notif, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIF_ID_ONGOING, notif)
        }

        acquireWakeLock()
        startRingtone()
        if (vibrate) startVibration()

        // 90s safety timeout
        safetyHandler = Handler(Looper.getMainLooper())
        safetyRunnable = Runnable {
            Log.i(TAG, "auto-stop after 90s")
            stopAlarmAndService()
        }
        safetyHandler?.postDelayed(safetyRunnable!!, 90_000L)

        return START_NOT_STICKY
    }

    override fun onDestroy() {
        stopRingtone()
        stopVibration()
        releaseWakeLock()
        safetyRunnable?.let { safetyHandler?.removeCallbacks(it) }
        super.onDestroy()
    }

    private fun stopAlarmAndService() {
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun acquireWakeLock() {
        val pm = getSystemService(Context.POWER_SERVICE) as PowerManager
        wakeLock = pm.newWakeLock(
            PowerManager.PARTIAL_WAKE_LOCK,
            "TaskFlow:AlarmService",
        ).apply {
            setReferenceCounted(false)
            acquire(120_000L)
        }
    }

    private fun releaseWakeLock() {
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
    }

    private fun startRingtone() {
        try {
            val uri = RingtoneManager.getActualDefaultRingtoneUri(this, RingtoneManager.TYPE_ALARM)
                ?: RingtoneManager.getDefaultUri(RingtoneManager.TYPE_NOTIFICATION)
            mediaPlayer = MediaPlayer().apply {
                setAudioAttributes(
                    AudioAttributes.Builder()
                        .setUsage(AudioAttributes.USAGE_ALARM)
                        .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                        .build()
                )
                setDataSource(this@AlarmForegroundService, uri)
                isLooping = true
                prepare()
                start()
            }
        } catch (e: Exception) {
            Log.w(TAG, "ringtone failed: ${e.message}")
        }
    }

    private fun stopRingtone() {
        try {
            mediaPlayer?.let {
                if (it.isPlaying) it.stop()
                it.release()
            }
        } catch (_: Exception) { }
        mediaPlayer = null
    }

    @Suppress("DEPRECATION")
    private fun startVibration() {
        val vibrator: Vibrator? = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            (getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as VibratorManager?)?.defaultVibrator
        } else {
            getSystemService(Context.VIBRATOR_SERVICE) as Vibrator?
        }
        if (vibrator == null || !vibrator.hasVibrator()) return
        try {
            val pattern = longArrayOf(0L, 600L, 400L, 600L, 400L)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                vibrator.vibrate(VibrationEffect.createWaveform(pattern, 0))
            } else {
                vibrator.vibrate(pattern, 0)
            }
        } catch (e: Exception) {
            Log.w(TAG, "vibrate failed: ${e.message}")
        }
    }

    private fun stopVibration() {
        try {
            val vibrator: Vibrator? = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                (getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as VibratorManager?)?.defaultVibrator
            } else {
                @Suppress("DEPRECATION")
                getSystemService(Context.VIBRATOR_SERVICE) as Vibrator?
            }
            vibrator?.cancel()
        } catch (_: Exception) { }
    }

    companion object {
        private const val TAG = "AlarmService"
        const val ACTION_STOP = "com.example.todoalarm.ACTION_STOP_ALARM"
        const val CHANNEL_ID = "alarm_strong"
        private const val NOTIF_ID_ONGOING = 0xA1A2

        fun ensureChannel(ctx: Context) {
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
            val mgr = ctx.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            if (mgr.getNotificationChannel(CHANNEL_ID) != null) return
            val ch = NotificationChannel(
                CHANNEL_ID,
                "强提醒",
                NotificationManager.IMPORTANCE_HIGH,
            ).apply {
                description = "提醒到点 / 响铃 / 全屏弹窗"
                enableVibration(true)
                setSound(null, null)   // 我们自己用 MediaPlayer 控制响铃
                setBypassDnd(true)
                lockscreenVisibility = Notification.VISIBILITY_PUBLIC
            }
            mgr.createNotificationChannel(ch)
        }

        fun stopIntent(ctx: Context): PendingIntent {
            val intent = Intent(ctx, AlarmForegroundService::class.java).apply { action = ACTION_STOP }
            return PendingIntent.getService(
                ctx, 1, intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
        }

        private fun buildPersistentNotification(ctx: Context, ruleId: Long, title: String): Notification {
            val openIntent = Intent(ctx, AlarmActivity::class.java).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP)
                putExtra(AlarmScheduler.EXTRA_RULE_ID, ruleId)
                putExtra(AlarmScheduler.EXTRA_TITLE, title)
            }
            val openPi = PendingIntent.getActivity(
                ctx, ruleId.toInt(), openIntent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            return NotificationCompat.Builder(ctx, CHANNEL_ID)
                .setContentTitle(title)
                .setContentText("⏰ 提醒响起 — 点击打开")
                .setSmallIcon(android.R.drawable.ic_popup_reminder)
                .setPriority(NotificationCompat.PRIORITY_MAX)
                .setCategory(NotificationCompat.CATEGORY_ALARM)
                .setOngoing(true)
                .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
                .setFullScreenIntent(openPi, true)
                .setContentIntent(openPi)
                .addAction(0, "停止响铃", stopIntent(ctx))
                .build()
        }
    }
}
