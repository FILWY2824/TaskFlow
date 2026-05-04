package com.example.taskflow.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
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
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.example.taskflow.util.Permissions

/**
 * 权限自检页(规格 §6)。
 *
 * 不会自动跳转 — 仅展示状态 + 给一个跳转 Settings 的按钮。
 * 当用户从 Settings 返回时,onResume 触发 LifecycleEventObserver 重新读权限,自动刷新 UI。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PermissionCheckScreen(onBack: () -> Unit) {
    val ctx = LocalContext.current
    val lifecycle = LocalLifecycleOwner.current.lifecycle

    var notif by remember { mutableStateOf(false) }
    var notifEnabled by remember { mutableStateOf(false) }
    var exact by remember { mutableStateOf(true) }
    var battery by remember { mutableStateOf(false) }
    var fullscreen by remember { mutableStateOf(true) }

    fun reload() {
        notif = Permissions.hasPostNotifications(ctx)
        notifEnabled = Permissions.areNotificationsEnabled(ctx)
        exact = Permissions.canScheduleExactAlarms(ctx)
        battery = Permissions.isIgnoringBatteryOptimizations(ctx)
        fullscreen = Permissions.canUseFullScreenIntent(ctx)
    }

    DisposableEffect(lifecycle) {
        reload()
        val obs = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) reload()
        }
        lifecycle.addObserver(obs)
        onDispose { lifecycle.removeObserver(obs) }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("权限自检") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).padding(16.dp).fillMaxSize().verticalScroll(rememberScrollState())) {
            PermissionRow(
                title = "通知权限 (Android 13+)",
                ok = notif,
                hint = "POST_NOTIFICATIONS,运行时权限",
                actionLabel = "去授权",
                onAction = { Permissions.openAppNotificationSettings(ctx) },
            )
            PermissionRow(
                title = "通知未被关闭",
                ok = notifEnabled,
                hint = "App 整体通知未被用户关闭",
                actionLabel = "去检查",
                onAction = { Permissions.openAppNotificationSettings(ctx) },
            )
            PermissionRow(
                title = "精确闹钟",
                ok = exact,
                hint = "Android 12+ 必须开启,否则提醒会被延迟",
                actionLabel = "去授权",
                onAction = { Permissions.openExactAlarmSettings(ctx) },
            )
            PermissionRow(
                title = "全屏意图",
                ok = fullscreen,
                hint = "Android 14+ 锁屏全屏弹窗,默认不给,需手动放开",
                actionLabel = "去设置",
                onAction = { Permissions.openFullScreenIntentSettings(ctx) },
            )
            PermissionRow(
                title = "电池优化白名单",
                ok = battery,
                hint = "强烈建议关闭,否则系统在 Doze / 强制睡眠下会延迟提醒",
                actionLabel = if (battery) "已加入" else "请求加入",
                onAction = { Permissions.openBatteryOptimizationSettings(ctx) },
            )

            Spacer(Modifier.height(24.dp))
            Text(
                "提示:不同厂商系统(MIUI / EMUI / OriginOS / 鸿蒙)还要在 \"自启动\" / \"后台保活\" / \"锁屏显示\" 列表里再放一遍 TaskFlow 才能保证 100% 准点。",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun PermissionRow(
    title: String,
    ok: Boolean,
    hint: String,
    actionLabel: String,
    onAction: () -> Unit,
) {
    Card(Modifier.fillMaxWidth().padding(vertical = 6.dp)) {
        Row(
            Modifier.padding(16.dp).fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            if (ok) Icon(Icons.Default.Check, "ok",
                tint = MaterialTheme.colorScheme.primary)
            else Icon(Icons.Default.Warning, "missing",
                tint = MaterialTheme.colorScheme.error)
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(title, style = MaterialTheme.typography.titleSmall)
                Text(hint, style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            if (!ok) {
                TextButton(onClick = onAction) { Text(actionLabel) }
            }
        }
    }
}
