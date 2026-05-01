package com.example.todoalarm.alarm

import android.app.KeyguardManager
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.lifecycleScope
import com.example.todoalarm.TaskFlowApp
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * 强提醒全屏 Activity。
 *
 * - showWhenLocked / turnScreenOn 在 manifest 里打开,锁屏也能弹出。
 * - 用户操作:
 *     "停止响铃"  -> 关本 Activity + 给 Service 发 ACTION_STOP
 *     "稍后提醒"  -> 同上(本地不会改 next_fire_at,5 分钟后由本地调度器再次触发)
 *     "完成任务"  -> 在线时把 todo 状态推到服务端;离线时仅本地停止响铃 + 提示
 *
 * 规格 §4 / §6.离线策略:离线"完成"时仅停响铃,不偷偷把状态推到服务端。
 */
class AlarmActivity : ComponentActivity() {

    private var ruleId: Long = -1L
    private var title: String = ""
    private var fireAt: String = ""

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setShowWhenLockedAndTurnScreenOn()
        readExtras(intent)
        setContent { AlarmScreen() }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        readExtras(intent)
    }

    private fun readExtras(intent: Intent?) {
        ruleId = intent?.getLongExtra(AlarmScheduler.EXTRA_RULE_ID, -1L) ?: -1L
        title = intent?.getStringExtra(AlarmScheduler.EXTRA_TITLE) ?: "提醒"
        fireAt = intent?.getStringExtra(AlarmScheduler.EXTRA_FIRE_AT) ?: ""
    }

    @Suppress("DEPRECATION")
    private fun setShowWhenLockedAndTurnScreenOn() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O_MR1) {
            setShowWhenLocked(true)
            setTurnScreenOn(true)
            val km = getSystemService(Context.KEYGUARD_SERVICE) as KeyguardManager?
            km?.requestDismissKeyguard(this, null)
        } else {
            window.addFlags(
                android.view.WindowManager.LayoutParams.FLAG_SHOW_WHEN_LOCKED or
                    android.view.WindowManager.LayoutParams.FLAG_TURN_SCREEN_ON or
                    android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON
            )
        }
        // 锁屏可见 + 屏幕亮着
        window.addFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
    }

    private fun stopAlarm() {
        val stop = Intent(this, AlarmForegroundService::class.java).apply {
            action = AlarmForegroundService.ACTION_STOP
        }
        startService(stop)
    }

    @Composable
    fun AlarmScreen() {
        var status by remember { mutableStateOf("") }

        // 简化:用 lifecycleScope 在按钮里查网络
        val container = (application as TaskFlowApp).container

        MaterialTheme {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(
                        Brush.verticalGradient(
                            colors = listOf(Color(0xFF1F6FEB), Color(0xFF1858BB))
                        )
                    ),
                contentAlignment = Alignment.Center,
            ) {
                Card(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(24.dp),
                    shape = RoundedCornerShape(20.dp),
                    colors = CardDefaults.cardColors(containerColor = Color.White),
                ) {
                    Column(
                        Modifier
                            .padding(28.dp)
                            .fillMaxWidth(),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        Text("⏰", fontSize = 56.sp)
                        Spacer(Modifier.height(12.dp))
                        Text(
                            title.ifEmpty { "提醒" },
                            fontSize = 22.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = Color(0xFF111827),
                        )
                        Spacer(Modifier.height(8.dp))
                        if (fireAt.isNotEmpty()) {
                            Text(
                                "触发时间:" + formatLocal(fireAt),
                                fontSize = 13.sp,
                                color = Color(0xFF6B7280),
                            )
                        }
                        if (status.isNotEmpty()) {
                            Spacer(Modifier.height(12.dp))
                            Text(status, fontSize = 13.sp, color = Color(0xFF1858BB))
                        }
                        Spacer(Modifier.height(20.dp))

                        Button(
                            onClick = {
                                stopAlarm()
                                finishAndRemoveTask()
                            },
                            modifier = Modifier.fillMaxWidth(),
                            colors = ButtonDefaults.buttonColors(
                                containerColor = Color(0xFF1F6FEB),
                                contentColor = Color.White,
                            ),
                        ) { Text("停止响铃") }

                        Spacer(Modifier.height(8.dp))

                        OutlinedButton(
                            onClick = {
                                // "稍后再提醒":本地停响铃。本地调度器还有这条规则,5 分钟后会再次触发。
                                stopAlarm()
                                finishAndRemoveTask()
                            },
                            modifier = Modifier.fillMaxWidth(),
                        ) { Text("稍后再提醒") }

                        Spacer(Modifier.height(8.dp))

                        OutlinedButton(
                            enabled = ruleId > 0,
                            onClick = {
                                lifecycleScope.launch {
                                    val ok = container.reminderRepository.tryCompleteFromAlarm(ruleId)
                                    if (ok) {
                                        status = "已标记完成 ✓"
                                    } else {
                                        status = "当前离线 — 已停止响铃,联网后请在主界面再次确认完成"
                                    }
                                    stopAlarm()
                                    finishAndRemoveTask()
                                }
                            },
                            modifier = Modifier.fillMaxWidth(),
                        ) { Text("完成任务") }
                    }
                }
            }
        }
    }

    companion object {
        private val LOCAL_FMT: DateTimeFormatter = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm")
            .withZone(ZoneId.systemDefault())

        private fun formatLocal(iso: String): String =
            try { LOCAL_FMT.format(Instant.parse(iso)) } catch (_: Exception) { iso }
    }
}
