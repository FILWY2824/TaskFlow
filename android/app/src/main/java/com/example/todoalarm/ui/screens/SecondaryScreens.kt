package com.example.todoalarm.ui.screens

import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.todoalarm.AppContainer
import com.example.todoalarm.util.DateTimeFmt

// ================================================================
// NotificationsScreen
// ================================================================

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NotificationsScreen(container: AppContainer, onBack: () -> Unit) {
    val vm: NotificationsViewModel = viewModel(factory = NotificationsViewModel.Factory(container))
    val state by vm.state.collectAsState()

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("通知 (${state.unreadCount} 未读)") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.Default.ArrowBack, "返回") } },
                actions = { TextButton(onClick = vm::markAllRead) { Text("全部已读") } },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).fillMaxSize()) {
            if (state.error != null) Text(state.error!!,
                color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(12.dp))
            if (state.items.isEmpty() && !state.loading) {
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Text("还没有通知", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            } else {
                LazyColumn {
                    items(state.items, key = { it.id }) { n ->
                        ListItem(
                            headlineContent = { Text(n.title) },
                            supportingContent = {
                                Column {
                                    if (n.body.isNotEmpty()) Text(n.body)
                                    Text(DateTimeFmt.localDateTime(n.fire_at, container.tokenManager.current().timezone),
                                        style = MaterialTheme.typography.bodySmall,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                            },
                            trailingContent = if (!n.is_read) {
                                { TextButton(onClick = { vm.markRead(n.id) }) { Text("已读") } }
                            } else null,
                        )
                        Divider()
                    }
                }
            }
        }
    }
}

// ================================================================
// TelegramBindScreen
// ================================================================

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TelegramBindScreen(container: AppContainer, onBack: () -> Unit) {
    val vm: TelegramViewModel = viewModel(factory = TelegramViewModel.Factory(container))
    val state by vm.state.collectAsState()
    val ctx = LocalContext.current

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Telegram 绑定") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.Default.ArrowBack, "返回") } },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).padding(16.dp).fillMaxSize().verticalScroll(rememberScrollState())) {
            Text("规格 §8:绑定通过 Telegram 深链发起,无需任何聊天 ID / 密码 / 验证码。",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(16.dp))

            if (state.bindings.isEmpty()) {
                Text("当前没有绑定", style = MaterialTheme.typography.titleMedium)
            } else {
                Text("已绑定", style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(8.dp))
                state.bindings.forEach { b ->
                    Card(Modifier.fillMaxWidth().padding(vertical = 4.dp)) {
                        Column(Modifier.padding(12.dp)) {
                            Text("@${b.username}", style = MaterialTheme.typography.titleSmall)
                            Text("chat_id: ${b.chat_id}", style = MaterialTheme.typography.bodySmall)
                            Text("绑定时间: ${DateTimeFmt.localDateTime(b.created_at, container.tokenManager.current().timezone)}",
                                style = MaterialTheme.typography.bodySmall)
                            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                                TextButton(onClick = { vm.sendTest(b.id) }) { Text("发测试消息") }
                                TextButton(onClick = { vm.unbind(b.id) }) { Text("解绑") }
                            }
                        }
                    }
                }
            }

            Spacer(Modifier.height(16.dp))
            Divider()
            Spacer(Modifier.height(16.dp))

            // 发起绑定
            if (state.activeBindToken == null) {
                Button(onClick = vm::startBind, modifier = Modifier.fillMaxWidth(),
                    enabled = !state.loading) {
                    Text(if (state.loading) "..." else "新增绑定")
                }
            } else {
                val tok = state.activeBindToken!!
                Card(Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(16.dp)) {
                        Text("点击下方按钮跳转 Telegram,在 @${tok.bot_username} 里发送 /start。",
                            style = MaterialTheme.typography.bodyMedium)
                        Spacer(Modifier.height(8.dp))
                        Button(
                            onClick = {
                                openTelegramDeepLink(ctx, tok.deep_link_app, tok.deep_link_web)
                            },
                            modifier = Modifier.fillMaxWidth(),
                        ) { Text("打开 Telegram 完成绑定") }
                        Spacer(Modifier.height(8.dp))
                        OutlinedButton(onClick = vm::checkBindStatus, modifier = Modifier.fillMaxWidth()) {
                            Text("我已发送 /start,刷新状态")
                        }
                        Text("有效期至: ${DateTimeFmt.localDateTime(tok.expires_at, container.tokenManager.current().timezone)}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }

            if (state.info != null) {
                Spacer(Modifier.height(8.dp))
                Text(state.info!!, color = MaterialTheme.colorScheme.primary)
            }
            if (state.error != null) {
                Spacer(Modifier.height(8.dp))
                Text(state.error!!, color = MaterialTheme.colorScheme.error)
            }
        }
    }
}

private fun openTelegramDeepLink(ctx: android.content.Context, appUrl: String, webUrl: String) {
    val app = Intent(Intent.ACTION_VIEW, Uri.parse(appUrl))
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    try {
        ctx.startActivity(app)
        return
    } catch (_: ActivityNotFoundException) {}

    // Fallback: 浏览器 https://t.me/...
    val web = Intent(Intent.ACTION_VIEW, Uri.parse(webUrl))
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    try { ctx.startActivity(web) } catch (_: Exception) {}
}

// ================================================================
// StatsScreen
// ================================================================

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StatsScreen(container: AppContainer, onBack: () -> Unit) {
    val vm: StatsViewModel = viewModel(factory = StatsViewModel.Factory(container))
    val state by vm.state.collectAsState()

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("统计") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.Default.ArrowBack, "返回") } },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).padding(16.dp).fillMaxSize().verticalScroll(rememberScrollState())) {
            if (state.error != null) Text(state.error!!, color = MaterialTheme.colorScheme.error)
            val s = state.summary
            if (s != null) {
                StatCard("今日完成", s.completed_today.toString())
                StatCard("本周完成", s.completed_this_week.toString())
                StatCard("待办总数", s.todos_open.toString())
                StatCard("已逾期", s.todos_overdue.toString(), MaterialTheme.colorScheme.errorContainer)
                StatCard("今日到期", s.todos_due_today.toString())
                StatCard("今日番茄(分)", (s.pomodoro_today_seconds / 60).toString())
                StatCard("本周番茄(分)", (s.pomodoro_this_week_seconds / 60).toString())
            } else if (state.loading) {
                CircularProgressIndicator()
            }
        }
    }
}

@Composable
private fun StatCard(title: String, value: String, bg: androidx.compose.ui.graphics.Color = MaterialTheme.colorScheme.surfaceVariant) {
    Card(Modifier.fillMaxWidth().padding(vertical = 4.dp), colors = CardDefaults.cardColors(containerColor = bg)) {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(title, modifier = Modifier.weight(1f))
            Text(value, style = MaterialTheme.typography.headlineSmall)
        }
    }
}

// ================================================================
// PomodoroScreen
// ================================================================

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PomodoroScreen(container: AppContainer, onBack: () -> Unit) {
    val vm: PomodoroViewModel = viewModel(factory = PomodoroViewModel.Factory(container))
    val state by vm.state.collectAsState()

    // 实时倒计时:每秒重算一次"剩余秒数"
    var nowMs by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(state.active?.id) {
        while (state.active != null) {
            nowMs = System.currentTimeMillis()
            kotlinx.coroutines.delay(1000)
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("番茄钟") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.Default.ArrowBack, "返回") } },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).padding(16.dp).fillMaxSize().verticalScroll(rememberScrollState())) {
            if (state.active == null) {
                Text("开始一个新的番茄", style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(8.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    listOf(15, 25, 45, 60).forEach { m ->
                        FilterChip(selected = state.plannedMinutes == m, onClick = { vm.setPlanned(m) },
                            label = { Text("${m}m") })
                    }
                }
                Spacer(Modifier.height(8.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    listOf("focus" to "专注", "short_break" to "短休", "long_break" to "长休").forEach { (k, label) ->
                        FilterChip(selected = state.kind == k, onClick = { vm.setKind(k) },
                            label = { Text(label) })
                    }
                }
                Spacer(Modifier.height(16.dp))
                Button(onClick = vm::start, modifier = Modifier.fillMaxWidth()) { Text("开始") }
            } else {
                val a = state.active!!
                // 计算剩余秒数
                val startedMs = runCatching {
                    java.time.Instant.parse(a.started_at).toEpochMilli()
                }.getOrNull() ?: nowMs
                val elapsedSec = ((nowMs - startedMs) / 1000).toInt().coerceAtLeast(0)
                val remainingSec = (a.planned_duration_seconds - elapsedSec).coerceAtLeast(0)
                val mm = remainingSec / 60
                val ss = remainingSec % 60
                val progress = if (a.planned_duration_seconds > 0)
                    (1f - remainingSec.toFloat() / a.planned_duration_seconds.toFloat()).coerceIn(0f, 1f)
                else 1f

                Card(Modifier.fillMaxWidth()) {
                    Column(
                        Modifier.padding(20.dp).fillMaxWidth(),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        Text(
                            when (a.kind) { "focus" -> "专注中"; "short_break" -> "短休中"; "long_break" -> "长休中"; else -> a.kind },
                            style = MaterialTheme.typography.titleMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        Spacer(Modifier.height(12.dp))
                        Text(
                            "%02d:%02d".format(mm, ss),
                            fontSize = 64.sp,
                            fontWeight = FontWeight.Light,
                            color = if (remainingSec == 0) MaterialTheme.colorScheme.primary
                                else MaterialTheme.colorScheme.onSurface,
                        )
                        Spacer(Modifier.height(8.dp))
                        LinearProgressIndicator(
                            progress = { progress },
                            modifier = Modifier.fillMaxWidth().height(6.dp),
                        )
                        Spacer(Modifier.height(4.dp))
                        Text(
                            "计划 ${a.planned_duration_seconds / 60} 分钟 · 已过 ${elapsedSec / 60} 分 ${elapsedSec % 60} 秒",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        if (remainingSec == 0) {
                            Spacer(Modifier.height(8.dp))
                            Text("⏰ 时间到 — 点完成结算", color = MaterialTheme.colorScheme.primary,
                                style = MaterialTheme.typography.titleSmall)
                        }
                    }
                }
                Spacer(Modifier.height(16.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = vm::complete, modifier = Modifier.weight(1f)) { Text("完成") }
                    OutlinedButton(onClick = vm::abandon, modifier = Modifier.weight(1f)) { Text("放弃") }
                }
            }

            Spacer(Modifier.height(24.dp))
            Text("最近", style = MaterialTheme.typography.titleMedium)
            state.recent.take(8).forEach { p ->
                Row(Modifier.fillMaxWidth().padding(vertical = 6.dp)) {
                    Text("${p.kind} · ${p.actual_duration_seconds / 60} min", modifier = Modifier.weight(1f))
                    Text(p.status, style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

// ================================================================
// SettingsScreen
// ================================================================

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(container: AppContainer, onBack: () -> Unit, onLoggedOut: () -> Unit) {
    val vm: SettingsViewModel = viewModel(factory = SettingsViewModel.Factory(container))
    val state by vm.state.collectAsState()

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("设置") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.Default.ArrowBack, "返回") } },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).padding(16.dp).fillMaxSize().verticalScroll(rememberScrollState())) {
            Text("当前账号", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(4.dp))
            Text(state.email, style = MaterialTheme.typography.bodyLarge)
            Text("时区: ${state.timezone}", color = MaterialTheme.colorScheme.onSurfaceVariant)

            Spacer(Modifier.height(24.dp))
            Text("服务端", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(8.dp))
            OutlinedTextField(value = state.serverUrl, onValueChange = vm::setServerUrl,
                label = { Text("服务端 URL") }, singleLine = true,
                modifier = Modifier.fillMaxWidth())
            Spacer(Modifier.height(8.dp))
            Button(onClick = vm::saveServerUrl) { Text("保存") }

            if (state.info != null) {
                Spacer(Modifier.height(8.dp))
                Text(state.info!!, color = MaterialTheme.colorScheme.primary)
            }
            if (state.error != null) {
                Spacer(Modifier.height(8.dp))
                Text(state.error!!, color = MaterialTheme.colorScheme.error)
            }

            Spacer(Modifier.height(32.dp))
            OutlinedButton(
                onClick = { vm.logout(onLoggedOut) },
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.outlinedButtonColors(contentColor = MaterialTheme.colorScheme.error),
            ) { Text("退出登录") }
        }
    }
}
