package com.example.taskflow.ui.screens

import android.content.Intent
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Login
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Security
import androidx.compose.material.icons.filled.Sync
import androidx.compose.material.icons.filled.Timer
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.taskflow.AppContainer
import com.example.taskflow.MainActivity

/**
 * 登录页兼产品首屏。
 *
 * Android 走 Custom Tabs 完成登录。拿到登录结果后主动拉起 MainActivity 到前台，
 * 避免用户留在浏览器里以为登录没有完成。
 */
@Composable
fun LoginScreen(
    container: AppContainer,
    onSuccess: () -> Unit,
) {
    val vm: LoginViewModel = viewModel(factory = LoginViewModel.Factory(container))
    val state by vm.state.collectAsState()
    val context = LocalContext.current

    TaskFlowErrorDialog(message = state.error, onDismiss = vm::clearError, title = "登录失败")

    LaunchedEffect(state.success) {
        if (state.success) {
            val intent = Intent(context, MainActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP)
            context.startActivity(intent)
            onSuccess()
        }
    }

    LaunchedEffect(state.pendingOpenUrl) {
        val url = state.pendingOpenUrl ?: return@LaunchedEffect
        try {
            CustomTabsIntent.Builder().build().launchUrl(context, Uri.parse(url))
        } catch (_: Throwable) {
            try {
                context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
            } catch (_: Throwable) {
                vm.reportError("无法打开系统浏览器，请先安装 Chrome 或其他浏览器后重试。")
            }
        }
        vm.consumePendingOpenUrl()
    }

    Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        Column(
            Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 20.dp, vertical = 28.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            ProductHero()
            FeatureRow(
                icon = Icons.Default.Notifications,
                title = "重要事项准时提醒",
                body = "锁屏强提醒、通知和本地响铃组合保障，不让开始时间悄悄溜走。",
            )
            FeatureRow(
                icon = Icons.Default.CalendarMonth,
                title = "待办和日程合在一起",
                body = "任务、预计时长、提醒和专注记录会统一呈现，打开就知道今天怎么安排。",
            )
            FeatureRow(
                icon = Icons.Default.Sync,
                title = "三端同步",
                body = "手机、网页和 Windows 客户端保持一致，换设备也能继续处理。",
            )
            LoginPanel(
                phase = state.phase,
                onLogin = vm::startOAuth,
                onCancel = vm::cancelOAuth,
            )
        }
    }
}

@Composable
private fun ProductHero() {
    ProductCard(tonal = true) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            Box(
                Modifier
                    .size(58.dp)
                    .clip(CircleShape)
                    .background(MaterialTheme.colorScheme.primary),
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    Icons.Default.Security,
                    contentDescription = null,
                    modifier = Modifier.size(30.dp),
                    tint = MaterialTheme.colorScheme.onPrimary,
                )
            }
            Column(Modifier.weight(1f)) {
                Text(
                    "TaskFlow",
                    style = MaterialTheme.typography.headlineMedium,
                    fontWeight = FontWeight.SemiBold,
                )
                Text(
                    "把待办、日程、强提醒和专注时间放在一个安静好用的工作台。",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            StatusPill("强提醒")
            StatusPill("三端同步", MaterialTheme.colorScheme.secondary)
            StatusPill("中文体验", MaterialTheme.colorScheme.tertiary)
        }
    }
}

@Composable
private fun FeatureRow(
    icon: ImageVector,
    title: String,
    body: String,
) {
    ProductCard {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Surface(
                shape = RoundedCornerShape(8.dp),
                color = MaterialTheme.colorScheme.primary.copy(alpha = 0.12f),
            ) {
                Icon(
                    icon,
                    contentDescription = null,
                    modifier = Modifier.padding(10.dp).size(22.dp),
                    tint = MaterialTheme.colorScheme.primary,
                )
            }
            Column(Modifier.weight(1f)) {
                Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
                Text(
                    body,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun LoginPanel(
    phase: OAuthLoginState.Phase,
    onLogin: () -> Unit,
    onCancel: () -> Unit,
) {
    ProductCard {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Icon(Icons.Default.CheckCircle, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
            Column(Modifier.weight(1f)) {
                Text("开始使用", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                Text(
                    "点击后会打开浏览器完成登录，成功后自动回到 TaskFlow。",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        when (phase) {
            OAuthLoginState.Phase.IDLE -> {
                Button(
                    onClick = onLogin,
                    modifier = Modifier.fillMaxWidth().height(54.dp),
                    shape = RoundedCornerShape(8.dp),
                ) {
                    Icon(Icons.AutoMirrored.Filled.Login, contentDescription = null)
                    Spacer(Modifier.size(8.dp))
                    Text("继续登录")
                }
            }
            OAuthLoginState.Phase.LAUNCHING -> LoginProgressRow("正在打开浏览器")
            OAuthLoginState.Phase.WAITING -> WaitingLoginCard(onCancel)
            OAuthLoginState.Phase.FINALIZING -> LoginProgressRow("正在完成登录")
        }
    }
}

@Composable
private fun WaitingLoginCard(onCancel: () -> Unit) {
    Surface(
        shape = RoundedCornerShape(8.dp),
        color = MaterialTheme.colorScheme.primaryContainer.copy(alpha = 0.45f),
    ) {
        Column(
            Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                Icon(Icons.Default.Timer, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                Column(Modifier.weight(1f)) {
                    Text("请在浏览器中完成登录", style = MaterialTheme.typography.titleSmall)
                    Text(
                        "完成后这里会自动刷新，你会直接进入工作台。",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            OutlinedButton(onClick = onCancel, modifier = Modifier.fillMaxWidth()) {
                Text("取消登录")
            }
        }
    }
}

@Composable
private fun LoginProgressRow(text: String) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.Center,
        modifier = Modifier.fillMaxWidth().height(54.dp),
    ) {
        CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
        Spacer(Modifier.size(10.dp))
        Text(text)
    }
}
