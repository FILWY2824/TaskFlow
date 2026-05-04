package com.example.taskflow.ui.screens

import android.content.Intent
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.material.icons.filled.Login
import androidx.compose.material.icons.filled.Security
import androidx.compose.material.icons.filled.Sync
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.taskflow.AppContainer
import com.example.taskflow.MainActivity

/**
 * 登录页(OAuth-only)。
 *
 * Android 走 Custom Tabs + 服务端 poll。poll 到 handoff 并 finalize 后，会主动
 * 拉起 MainActivity 到前台，避免用户留在浏览器里以为登录没有完成。
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

    // 监听 pendingOpenUrl，出现一次就打开一次浏览器。
    LaunchedEffect(state.pendingOpenUrl) {
        val url = state.pendingOpenUrl ?: return@LaunchedEffect
        try {
            val intent = CustomTabsIntent.Builder().build()
            intent.launchUrl(context, Uri.parse(url))
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
                .padding(20.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.Center,
        ) {
            ProductCard(tonal = true) {
                Surface(
                    modifier = Modifier.size(64.dp),
                    shape = CircleShape,
                    color = MaterialTheme.colorScheme.primary,
                ) {
                    Icon(
                        Icons.Default.Security,
                        contentDescription = null,
                        modifier = Modifier.padding(16.dp),
                        tint = MaterialTheme.colorScheme.onPrimary,
                    )
                }
                Text(
                    "欢迎回来",
                    style = MaterialTheme.typography.headlineMedium,
                    fontWeight = FontWeight.SemiBold,
                )
                Text(
                    "TaskFlow 使用统一认证中心登录。登录与注册都在浏览器中完成，成功后应用会自动回到前台。",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                StatusPill("默认服务端已配置")

                when (state.phase) {
                    OAuthLoginState.Phase.IDLE -> {
                        Button(
                            onClick = vm::startOAuth,
                            modifier = Modifier.fillMaxWidth().height(52.dp),
                        ) {
                            Icon(Icons.Default.Login, contentDescription = null)
                            Spacer(Modifier.size(8.dp))
                            Text("通过认证中心登录")
                        }
                    }
                    OAuthLoginState.Phase.LAUNCHING -> LoginProgressRow("正在打开系统浏览器")
                    OAuthLoginState.Phase.WAITING -> {
                        Surface(
                            shape = RoundedCornerShape(8.dp),
                            color = MaterialTheme.colorScheme.surface.copy(alpha = 0.82f),
                        ) {
                            Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                                    Icon(Icons.Default.Sync, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                                    Column(Modifier.weight(1f)) {
                                        Text("等待浏览器授权", style = MaterialTheme.typography.titleSmall)
                                        Text(
                                            "请在浏览器中完成登录。本应用会持续接收登录结果。",
                                            style = MaterialTheme.typography.bodySmall,
                                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                                        )
                                    }
                                }
                                OutlinedButton(onClick = vm::cancelOAuth, modifier = Modifier.fillMaxWidth()) {
                                    Text("取消登录")
                                }
                            }
                        }
                    }
                    OAuthLoginState.Phase.FINALIZING -> LoginProgressRow("正在完成登录")
                }
            }
        }
    }
}

@Composable
private fun LoginProgressRow(text: String) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.Center,
        modifier = Modifier.fillMaxWidth().height(52.dp),
    ) {
        CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
        Spacer(Modifier.size(10.dp))
        Text(text)
    }
}
