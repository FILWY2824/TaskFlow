package com.example.taskflow.ui.screens

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
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.taskflow.AppContainer
import com.example.taskflow.util.DateTimeFmt

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
//
// 规格 §17 阶段 13:Android 端的"设置"承载了所有 Android-only 的通知与提醒开关
// (scope='android'),以及从浏览器搬过来的"账号 / 时区 / 服务端"等通用项。
// Web / Windows 的对应项在各自客户端展示,这里看不到 —— 但在数据库里它们各占一行,
// 用户切换到任一端都能看到完整的本端设置。
//
// 这里同时把"权限自检"内联到 SettingsScreen,而不是再让用户跳到独立子页:
// Android 上要让强提醒真正生效,五项权限缺一不可,所以放在第一屏直观提示。
//

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(container: AppContainer, onBack: () -> Unit, onLoggedOut: () -> Unit) {
    val vm: SettingsViewModel = viewModel(factory = SettingsViewModel.Factory(container))
    val state by vm.state.collectAsState()
    val ctx = LocalContext.current
    val lifecycle = LocalLifecycleOwner.current.lifecycle

    // 五项权限的实时状态。每次 onResume 重读(用户从系统设置回来后立刻反映)。
    var permPostNotif by remember { mutableStateOf(true) }
    var permNotifEnabled by remember { mutableStateOf(true) }
    var permExactAlarm by remember { mutableStateOf(true) }
    var permFullScreen by remember { mutableStateOf(true) }
    var permBattery by remember { mutableStateOf(true) }

    fun reloadPerms() {
        permPostNotif    = com.example.taskflow.util.Permissions.hasPostNotifications(ctx)
        permNotifEnabled = com.example.taskflow.util.Permissions.areNotificationsEnabled(ctx)
        permExactAlarm   = com.example.taskflow.util.Permissions.canScheduleExactAlarms(ctx)
        permFullScreen   = com.example.taskflow.util.Permissions.canUseFullScreenIntent(ctx)
        permBattery      = com.example.taskflow.util.Permissions.isIgnoringBatteryOptimizations(ctx)
    }
    DisposableEffect(lifecycle) {
        reloadPerms()
        val obs = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                reloadPerms()
                vm.refreshPrefs()
            }
        }
        lifecycle.addObserver(obs)
        onDispose { lifecycle.removeObserver(obs) }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("设置") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.Default.ArrowBack, "返回") } },
            )
        },
    ) { padding ->
        Column(
            Modifier
                .padding(padding)
                .padding(16.dp)
                .fillMaxSize()
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(20.dp),
        ) {
            // ---------- 账号 ----------
            SettingsCard(title = "账号") {
                Text(state.email, style = MaterialTheme.typography.bodyLarge)
                Spacer(Modifier.height(4.dp))
                Text(
                    "时区: ${state.timezone}",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            // ---------- 权限自检(Android 专属,内联) ----------
            SettingsCard(
                title = "Android 通知与强提醒权限",
                hint = "Android 端要让到点真的能叫醒你,这五项权限都需要打开。下面任何一项缺失都会延迟或丢失提醒。",
            ) {
                PermissionRow(
                    title = "通知权限 (Android 13+)",
                    ok = permPostNotif,
                    hint = "POST_NOTIFICATIONS 运行时权限,没有的话连状态栏图标都不出现。",
                    onAction = { com.example.taskflow.util.Permissions.openAppNotificationSettings(ctx) },
                )
                PermissionRow(
                    title = "通知未被关闭",
                    ok = permNotifEnabled,
                    hint = "整个应用的通知开关没被用户关掉。",
                    onAction = { com.example.taskflow.util.Permissions.openAppNotificationSettings(ctx) },
                )
                PermissionRow(
                    title = "精确闹钟",
                    ok = permExactAlarm,
                    hint = "Android 12+ 必须开,否则 AlarmManager 会被 Doze 延迟到几分钟甚至几小时之后。",
                    onAction = { com.example.taskflow.util.Permissions.openExactAlarmSettings(ctx) },
                )
                PermissionRow(
                    title = "全屏意图",
                    ok = permFullScreen,
                    hint = "Android 14+ 锁屏全屏弹窗,默认不给,需手动放开。",
                    onAction = { com.example.taskflow.util.Permissions.openFullScreenIntentSettings(ctx) },
                )
                PermissionRow(
                    title = "电池优化白名单",
                    ok = permBattery,
                    hint = "建议加入,否则系统在 Doze / 强制睡眠下会延迟提醒。",
                    onAction = { com.example.taskflow.util.Permissions.openBatteryOptimizationSettings(ctx) },
                )
                Spacer(Modifier.height(8.dp))
                Text(
                    "提示:不同厂商系统(MIUI / EMUI / OriginOS / 鸿蒙)还要在\"自启动\"/\"后台保活\"/\"锁屏显示\"里再放一遍 TaskFlow 才能保证 100% 准点。",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            // ---------- Android 通知开关(scope='android') ----------
            SettingsCard(
                title = "Android 通知行为",
                hint = "这一组开关只对当前 Android 端生效。Web / Windows 客户端的同类设置在各自客户端里管理,但都会同步保存到你的账户。",
            ) {
                ToggleRow(
                    title = "锁屏全屏强提醒",
                    desc = "到点拉起全屏 Activity,直到你点击\"完成\"或\"稍后\"。需要\"全屏意图\"权限放开。",
                    checked = state.prefs.fullScreenAlarm,
                    onChange = { v -> vm.togglePref { it.copy(fullScreenAlarm = v) } },
                )
                ToggleRow(
                    title = "震动",
                    desc = "强提醒响铃同时震动。",
                    checked = state.prefs.vibrate,
                    onChange = { v -> vm.togglePref { it.copy(vibrate = v) } },
                )
                ToggleRow(
                    title = "应用内提示",
                    desc = "应用前台时,在底部弹出小卡片(独立于系统通知)。",
                    checked = state.prefs.inAppToast,
                    onChange = { v -> vm.togglePref { it.copy(inAppToast = v) } },
                )
                ToggleRow(
                    title = "任务截止本地弹窗",
                    desc = "任务到了截止时间时,本地弹窗(独立于服务端 reminder)。",
                    checked = state.prefs.todoDueLocalAlarm,
                    onChange = { v -> vm.togglePref { it.copy(todoDueLocalAlarm = v) } },
                )
                ToggleRow(
                    title = "番茄到点响铃",
                    desc = "通过 Foreground Service 播放铃声,直到用户点完成或自动停止。",
                    checked = state.prefs.pomodoroSound,
                    onChange = { v -> vm.togglePref { it.copy(pomodoroSound = v) } },
                )
                ToggleRow(
                    title = "番茄到点自动结束",
                    desc = "关闭后停留在 0:00 等手动确认;开启后倒计时结束直接入库。",
                    checked = state.prefs.pomodoroAutoComplete,
                    onChange = { v -> vm.togglePref { it.copy(pomodoroAutoComplete = v) } },
                )
                Divider(Modifier.padding(vertical = 8.dp))

                // ---- 系统时钟双保险 ----
                ToggleRow(
                    title = "同步到系统\"时钟\"应用(双保险)",
                    desc = "每条 reminder 创建/更新时,同时往系统时钟里写一条单次闹钟。系统时钟享受全部白名单,绝不漏响 —— 但条目会需要你手动到时钟应用里清理。建议只在重要场景开启。",
                    checked = state.prefs.useSystemAlarmClock,
                    onChange = { v -> vm.togglePref { it.copy(useSystemAlarmClock = v) } },
                )
                if (state.prefs.useSystemAlarmClock) {
                    Spacer(Modifier.height(4.dp))
                    OutlinedButton(
                        onClick = { com.example.taskflow.util.SystemAlarmClock.openClockApp(ctx) },
                        modifier = Modifier.fillMaxWidth(),
                    ) { Text("打开系统时钟应用查看 / 清理闹钟") }
                }

                Spacer(Modifier.height(12.dp))
                Text("响铃自动停止时长", style = MaterialTheme.typography.titleSmall)
                Text(
                    "强提醒响铃 ${state.prefs.autoSnoozeMinutes} 分钟后,如果用户没点任何按钮,自动停响。",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(4.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    listOf(1, 5, 10, 15, 30).forEach { m ->
                        FilterChip(
                            selected = state.prefs.autoSnoozeMinutes == m,
                            onClick = { vm.setAutoSnooze(m) },
                            label = { Text("${m}m") },
                        )
                    }
                }
            }

            if (state.error != null) {
                Text(
                    state.error!!,
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            // ---------- 检测更新 ----------
            SettingsCard(title = "检测更新") {
                Text(
                    "当前版本 v0.4.0",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(8.dp))
                OutlinedButton(
                    onClick = { vm.checkUpdate() },
                    enabled = !state.updateChecking,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text(if (state.updateChecking) "检测中…" else "检查新版本")
                }
                if (state.updateError != null) {
                    Spacer(Modifier.height(6.dp))
                    Text(state.updateError!!, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                }
                if (state.updateHasNew == true) {
                    Spacer(Modifier.height(8.dp))
                    Text("发现新版本 v${state.updateVersion}", style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary)
                    if (state.updateNotes != null) {
                        Text(state.updateNotes!!, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    if (state.updateUrl != null) {
                        Spacer(Modifier.height(6.dp))
                        val uriHandler = androidx.compose.ui.platform.LocalUriHandler.current
                        Button(onClick = { uriHandler.openUri(state.updateUrl!!) }, modifier = Modifier.fillMaxWidth()) { Text("下载新版本") }
                    }
                } else if (state.updateHasNew == false) {
                    Spacer(Modifier.height(6.dp))
                    Text("✓ 当前已是最新版本", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }

            // ---------- 退出登录 ----------
            OutlinedButton(
                onClick = { vm.logout(onLoggedOut) },
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.outlinedButtonColors(contentColor = MaterialTheme.colorScheme.error),
            ) { Text("退出登录") }

            Spacer(Modifier.height(8.dp))
            Text(
                "TaskFlow Android · v0.4.0",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}

@Composable
private fun SettingsCard(
    title: String,
    hint: String? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium)
            if (hint != null) {
                Spacer(Modifier.height(4.dp))
                Text(
                    hint,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Spacer(Modifier.height(12.dp))
            content()
        }
    }
}

@Composable
private fun ToggleRow(
    title: String,
    desc: String,
    checked: Boolean,
    onChange: (Boolean) -> Unit,
) {
    Row(
        Modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            Text(
                desc,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Switch(checked = checked, onCheckedChange = onChange)
    }
}

@Composable
private fun PermissionRow(
    title: String,
    ok: Boolean,
    hint: String,
    onAction: () -> Unit,
) {
    Row(
        Modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (ok) Icon(
            Icons.Default.Check, "ok",
            tint = MaterialTheme.colorScheme.primary,
        ) else Icon(
            Icons.Default.Warning, "missing",
            tint = MaterialTheme.colorScheme.error,
        )
        Spacer(Modifier.width(8.dp))
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.titleSmall)
            Text(
                hint,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (!ok) {
            TextButton(onClick = onAction) { Text("去授权") }
        }
    }
}
