package com.example.taskflow.ui.screens

import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.taskflow.AppContainer
import com.example.taskflow.BuildConfig
import com.example.taskflow.util.DateTimeFmt

// ================================================================
// NotificationsScreen
// ================================================================

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NotificationsScreen(container: AppContainer, onBack: () -> Unit) {
    val vm: NotificationsViewModel = viewModel(factory = NotificationsViewModel.Factory(container))
    val state by vm.state.collectAsState()

    TaskFlowErrorDialog(message = state.error, onDismiss = vm::clearError)

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("通知 (${state.unreadCount} 未读)") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
                actions = { TextButton(onClick = vm::markAllRead) { Text("全部已读") } },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).fillMaxSize()) {
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
                        HorizontalDivider()
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

    TaskFlowErrorDialog(message = state.error, onDismiss = vm::clearError)

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Telegram 绑定") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
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
            HorizontalDivider()
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

private enum class StatsPanel(val label: String) {
    Overview("概览"),
    Tasks("任务"),
    Focus("专注"),
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StatsScreen(container: AppContainer, onBack: () -> Unit) {
    val vm: StatsViewModel = viewModel(factory = StatsViewModel.Factory(container))
    val state by vm.state.collectAsState()
    var panel by remember { mutableStateOf(StatsPanel.Overview) }

    TaskFlowErrorDialog(message = state.error, onDismiss = vm::clearError)

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("统计") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
            )
        },
    ) { padding ->
        LazyColumn(
            Modifier.padding(padding).fillMaxSize(),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            val s = state.summary
            item {
                StatsPanelFilter(current = panel, onSelect = { panel = it })
            }
            if (s == null && state.loading) {
                item {
                    Box(Modifier.fillMaxWidth().padding(32.dp), contentAlignment = Alignment.Center) {
                        CircularProgressIndicator()
                    }
                }
            } else if (s != null) {
                when (panel) {
                    StatsPanel.Overview -> {
                        item {
                            ProductCard(tonal = true) {
                                Text("今天的推进感", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    MetricTile("今日完成", s.completed_today.toString(), Modifier.weight(1f), MaterialTheme.colorScheme.primary)
                                    MetricTile("今日到期", s.todos_due_today.toString(), Modifier.weight(1f), MaterialTheme.colorScheme.tertiary)
                                }
                                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    MetricTile("本周完成", s.completed_this_week.toString(), Modifier.weight(1f), MaterialTheme.colorScheme.secondary)
                                    MetricTile("逾期", s.todos_overdue.toString(), Modifier.weight(1f), MaterialTheme.colorScheme.error)
                                }
                            }
                        }
                        item {
                            ProductCard {
                                Text("专注概览", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    MetricTile("今日专注", "${s.pomodoro_today_seconds / 60} 分钟", Modifier.weight(1f), MaterialTheme.colorScheme.primary)
                                    MetricTile("本周专注", "${s.pomodoro_this_week_seconds / 60} 分钟", Modifier.weight(1f), MaterialTheme.colorScheme.secondary)
                                }
                            }
                        }
                    }
                    StatsPanel.Tasks -> {
                        item { StatCard("待办总数", s.todos_open.toString(), "还需要推进的任务") }
                        item { StatCard("今日到期", s.todos_due_today.toString(), "今天需要收口的事项", MaterialTheme.colorScheme.tertiaryContainer) }
                        item { StatCard("已逾期", s.todos_overdue.toString(), "建议优先处理", MaterialTheme.colorScheme.errorContainer) }
                    }
                    StatsPanel.Focus -> {
                        item {
                            ProductCard {
                                Text("专注时间", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    MetricTile("今日", "${s.pomodoro_today_seconds / 60} 分钟", Modifier.weight(1f), MaterialTheme.colorScheme.primary)
                                    MetricTile("本周", "${s.pomodoro_this_week_seconds / 60} 分钟", Modifier.weight(1f), MaterialTheme.colorScheme.secondary)
                                }
                                Text(
                                    "筛选到专注后，只看番茄相关指标，减少任务数字的干扰。",
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun StatsPanelFilter(current: StatsPanel, onSelect: (StatsPanel) -> Unit) {
    LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        items(StatsPanel.entries) { item ->
            FilterChip(
                selected = current == item,
                onClick = { onSelect(item) },
                label = { Text(item.label) },
            )
        }
    }
}

@Composable
private fun StatCard(
    title: String,
    value: String,
    desc: String,
    bg: androidx.compose.ui.graphics.Color = MaterialTheme.colorScheme.surfaceVariant,
) {
    Card(Modifier.fillMaxWidth().padding(vertical = 4.dp), colors = CardDefaults.cardColors(containerColor = bg)) {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Column(Modifier.weight(1f)) {
                Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
                Text(desc, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
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
    var historyFilter by remember { mutableStateOf("all") }

    TaskFlowErrorDialog(message = state.error, onDismiss = vm::clearError)

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
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
            )
        },
    ) { padding ->
        LazyColumn(
            Modifier.padding(padding).fillMaxSize(),
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            if (state.active == null) {
                item {
                    ProductCard(tonal = true) {
                        Text("开始一个新的专注", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                        Text("选择时长和类型，开始后页面会变成实时计时盘。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            items(listOf(15, 25, 45, 60)) { m ->
                                FilterChip(
                                    selected = state.plannedMinutes == m,
                                    onClick = { vm.setPlanned(m) },
                                    label = { Text("${m}m") },
                                )
                            }
                        }
                        LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            items(listOf("focus" to "专注", "short_break" to "短休", "long_break" to "长休")) { (k, label) ->
                                FilterChip(
                                    selected = state.kind == k,
                                    onClick = { vm.setKind(k) },
                                    label = { Text(label) },
                                )
                            }
                        }
                        Button(onClick = vm::start, modifier = Modifier.fillMaxWidth()) { Text("开始") }
                    }
                }
            } else {
                item {
                    val a = state.active!!
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

                    ProductCard(tonal = true) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.fillMaxWidth()) {
                            StatusPill(
                                when (a.kind) { "focus" -> "专注中"; "short_break" -> "短休中"; "long_break" -> "长休中"; else -> a.kind },
                                MaterialTheme.colorScheme.primary,
                            )
                            Spacer(Modifier.height(10.dp))
                            Text(
                                "%02d:%02d".format(mm, ss),
                                fontSize = 64.sp,
                                fontWeight = FontWeight.Light,
                                color = if (remainingSec == 0) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurface,
                            )
                            LinearProgressIndicator(
                                progress = { progress },
                                modifier = Modifier.fillMaxWidth().height(8.dp),
                            )
                            Text(
                                "计划 ${a.planned_duration_seconds / 60} 分钟 · 已过 ${elapsedSec / 60} 分 ${elapsedSec % 60} 秒",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            if (remainingSec == 0) {
                                Text("时间到，点完成结算", color = MaterialTheme.colorScheme.primary, style = MaterialTheme.typography.titleSmall)
                            }
                        }
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Button(onClick = vm::complete, modifier = Modifier.weight(1f)) { Text("完成") }
                            OutlinedButton(onClick = vm::abandon, modifier = Modifier.weight(1f)) { Text("放弃") }
                        }
                    }
                }
            }

            item {
                Text("最近记录", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            }
            item {
                LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(listOf("all" to "全部", "completed" to "完成", "abandoned" to "放弃", "active" to "进行中")) { (k, label) ->
                        FilterChip(selected = historyFilter == k, onClick = { historyFilter = k }, label = { Text(label) })
                    }
                }
            }
            val recent = state.recent
                .filter { historyFilter == "all" || it.status == historyFilter }
                .take(12)
            if (recent.isEmpty()) {
                item {
                    EmptyProductState(
                        title = "还没有记录",
                        body = "完成一次专注后，这里会显示最近的时长和状态。",
                    )
                }
            } else {
                items(recent, key = { it.id }) { p ->
                    ProductCard {
                        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(
                                    when (p.kind) { "focus" -> "专注"; "short_break" -> "短休"; "long_break" -> "长休"; else -> p.kind },
                                    style = MaterialTheme.typography.titleSmall,
                                    fontWeight = FontWeight.SemiBold,
                                )
                                Text(
                                    "${p.actual_duration_seconds / 60} 分钟 · 计划 ${p.planned_duration_seconds / 60} 分钟",
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            StatusPill(
                                when (p.status) { "completed" -> "完成"; "abandoned" -> "放弃"; "active" -> "进行中"; else -> p.status },
                                if (p.status == "completed") MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.outline,
                            )
                        }
                    }
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
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(container: AppContainer, onBack: () -> Unit, onLoggedOut: () -> Unit) {
    val vm: SettingsViewModel = viewModel(factory = SettingsViewModel.Factory(container))
    val state by vm.state.collectAsState()
    val uriHandler = androidx.compose.ui.platform.LocalUriHandler.current
    var showTimezonePicker by remember { mutableStateOf(false) }

    TaskFlowErrorDialog(message = state.error, onDismiss = vm::clearError)

    if (showTimezonePicker) {
        TimezonePickerDialog(
            current = state.timezone,
            onDismiss = { showTimezonePicker = false },
            onSelect = {
                showTimezonePicker = false
                vm.setTimezone(it)
            },
        )
    }

    val updateDialog = state.updateDialog
    if (updateDialog != null) {
        AlertDialog(
            onDismissRequest = vm::dismissUpdateDialog,
            title = {
                Text(
                    when {
                        updateDialog.error != null -> "检查失败"
                        updateDialog.hasNew -> "发现新版本"
                        else -> "当前已是最新版本"
                    },
                )
            },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (updateDialog.version != null) {
                        Text("最新版本: v${updateDialog.version}")
                    }
                    Text("当前版本: v${BuildConfig.VERSION_NAME}", color = MaterialTheme.colorScheme.onSurfaceVariant)
                    if (!updateDialog.notes.isNullOrBlank()) {
                        Text(updateDialog.notes, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            },
            confirmButton = {
                if (updateDialog.hasNew && updateDialog.url != null) {
                    Button(onClick = {
                        uriHandler.openUri(updateDialog.url)
                        vm.dismissUpdateDialog()
                    }) {
                        Text("安装新版本")
                    }
                } else {
                    TextButton(onClick = vm::dismissUpdateDialog) {
                        Text("知道了")
                    }
                }
            },
            dismissButton = {
                if (updateDialog.hasNew && updateDialog.url != null) {
                    TextButton(onClick = vm::dismissUpdateDialog) {
                        Text("稍后")
                    }
                }
            },
        )
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("设置") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
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
                Spacer(Modifier.height(12.dp))
                Surface(
                    shape = RoundedCornerShape(8.dp),
                    color = MaterialTheme.colorScheme.primaryContainer.copy(alpha = 0.42f),
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable(enabled = !state.timezoneSaving) { showTimezonePicker = true },
                ) {
                    Column(Modifier.padding(12.dp)) {
                        Text(
                            "当前时区",
                            style = MaterialTheme.typography.labelMedium,
                            color = MaterialTheme.colorScheme.onPrimaryContainer,
                        )
                        Text(
                            timezoneLabel(state.timezone),
                            style = MaterialTheme.typography.titleMedium,
                            fontWeight = FontWeight.SemiBold,
                        )
                        Text(
                            "点此选择其他时区。所有任务、提醒和统计都会按这个时区展示，并保存到你的账户。",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.72f),
                        )
                        Spacer(Modifier.height(8.dp))
                        Text(
                            if (state.timezoneSaving) "保存中..." else "本机时区: ${timezoneLabel(state.systemTimezone)}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.78f),
                        )
                        if (state.shouldSuggestSystemTimezone && !state.timezoneSaving) {
                            TextButton(onClick = vm::syncSystemTimezone) {
                                Text("同步为本机时区")
                            }
                        }
                    }
                }
            }

            // ---------- 检测更新 ----------
            SettingsCard(title = "检测更新") {
                Text(
                    "当前版本 v${BuildConfig.VERSION_NAME}",
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
            }

            // ---------- 退出登录 ----------
            OutlinedButton(
                onClick = { vm.logout(onLoggedOut) },
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.outlinedButtonColors(contentColor = MaterialTheme.colorScheme.error),
            ) { Text("退出登录") }

            Spacer(Modifier.height(8.dp))
            Text(
                "TaskFlow Android · v${BuildConfig.VERSION_NAME}",
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
private fun TimezonePickerDialog(
    current: String,
    onDismiss: () -> Unit,
    onSelect: (String) -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("选择时区") },
        text = {
            LazyColumn(
                modifier = Modifier.fillMaxWidth().height(420.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                TIMEZONE_GROUPS.forEach { group ->
                    item {
                        Text(
                            group.label,
                            style = MaterialTheme.typography.labelLarge,
                            color = MaterialTheme.colorScheme.primary,
                        )
                    }
                    items(group.options, key = { it.value }) { option ->
                        Surface(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable { onSelect(option.value) },
                            shape = RoundedCornerShape(8.dp),
                            color = if (option.value == current) {
                                MaterialTheme.colorScheme.primaryContainer
                            } else {
                                MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.5f)
                            },
                        ) {
                            Row(
                                modifier = Modifier.padding(horizontal = 12.dp, vertical = 10.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Column(Modifier.weight(1f)) {
                                    Text(option.label, fontWeight = FontWeight.SemiBold)
                                    Text(
                                        option.value,
                                        style = MaterialTheme.typography.bodySmall,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    )
                                }
                                if (option.value == current) {
                                    Icon(Icons.Default.Check, contentDescription = "当前")
                                }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) {
                Text("取消")
            }
        },
    )
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
