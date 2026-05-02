package com.example.taskflow.ui.screens

import android.content.Intent
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.taskflow.AppContainer

/**
 * 登录页(OAuth-only)。
 *
 * 三端都强制走 OAuth,Android 这边的流程是:
 *   - 用户(可选)修改服务器 URL
 *   - 点 "通过认证中心登录"
 *   - ViewModel 把 OAuth start URL 推到 pendingOpenUrl,这里用 Custom Tabs 打开
 *     (没装支持的浏览器就 fallback 普通 Intent)
 *   - 期间显示 "已在系统浏览器中打开认证中心,请完成登录"
 *   - 拿到 handoff + finalize 后跳到主界面(MainActivity 监听 success)
 */
@Composable
fun LoginScreen(
    container: AppContainer,
    onSuccess: () -> Unit,
) {
    val vm: LoginViewModel = viewModel(factory = LoginViewModel.Factory(container))
    val state by vm.state.collectAsState()
    val context = LocalContext.current

    LaunchedEffect(state.success) { if (state.success) onSuccess() }

    // 监听 pendingOpenUrl,出现一次就打开一次浏览器
    LaunchedEffect(state.pendingOpenUrl) {
        val url = state.pendingOpenUrl ?: return@LaunchedEffect
        try {
            // 优先 Custom Tabs(同进程,体验更好)
            val intent = CustomTabsIntent.Builder().build()
            intent.launchUrl(context, Uri.parse(url))
        } catch (_: Throwable) {
            // 没有 Chrome / Custom Tabs Service 时 fallback 普通浏览器 Intent
            try {
                context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
            } catch (_: Throwable) {
                // 用户机器上一个浏览器都没有 —— 这种情况只能让 ViewModel 重置
            }
        }
        vm.consumePendingOpenUrl()
    }

    Surface(modifier = Modifier.fillMaxSize()) {
        Column(
            Modifier
                .fillMaxSize()
                .padding(24.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.Center,
        ) {
            Text("TaskFlow", style = MaterialTheme.typography.displaySmall)
            Spacer(Modifier.height(8.dp))
            Text(
                "登录到你的服务端",
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(24.dp))

            OutlinedTextField(
                value = state.serverUrl,
                onValueChange = vm::setServerUrl,
                label = { Text("服务端 URL") },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
                supportingText = {
                    Text("例如 https://taskflow.example.com 或 http://10.0.2.2:8080(模拟器)")
                },
                enabled = state.phase == OAuthLoginState.Phase.IDLE,
            )

            if (state.error != null) {
                Spacer(Modifier.height(12.dp))
                Text(state.error!!, color = MaterialTheme.colorScheme.error)
            }

            Spacer(Modifier.height(20.dp))

            when (state.phase) {
                OAuthLoginState.Phase.IDLE -> {
                    Button(
                        onClick = vm::startOAuth,
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(48.dp),
                    ) {
                        Text("通过认证中心登录")
                    }
                }
                OAuthLoginState.Phase.LAUNCHING -> {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.Center,
                        modifier = Modifier.fillMaxWidth().height(48.dp),
                    ) {
                        CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.width(10.dp))
                        Text("正在打开系统浏览器…")
                    }
                }
                OAuthLoginState.Phase.WAITING -> {
                    Text(
                        "已在系统浏览器中打开认证中心。\n登录完成后,本应用会自动接收登录态并进入主界面。",
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Spacer(Modifier.height(12.dp))
                    OutlinedButton(
                        onClick = vm::cancelOAuth,
                        modifier = Modifier.fillMaxWidth(),
                    ) { Text("取消") }
                }
                OAuthLoginState.Phase.FINALIZING -> {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.Center,
                        modifier = Modifier.fillMaxWidth().height(48.dp),
                    ) {
                        CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.width(10.dp))
                        Text("正在完成登录…")
                    }
                }
            }

            Spacer(Modifier.height(12.dp))
            Text(
                "登录与注册都在认证中心完成;首次登录会自动在 TaskFlow 创建账号。",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
