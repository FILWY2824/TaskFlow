package com.example.todoalarm.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.todoalarm.AppContainer

@Composable
fun RegisterScreen(
    container: AppContainer,
    onSuccess: () -> Unit,
    onGotoLogin: () -> Unit,
) {
    val vm: RegisterViewModel = viewModel(factory = RegisterViewModel.Factory(container))
    val state by vm.state.collectAsState()

    LaunchedEffect(state.success) { if (state.success) onSuccess() }

    Surface(modifier = Modifier.fillMaxSize()) {
        Column(
            Modifier.fillMaxSize().padding(24.dp).verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.Center,
        ) {
            Text("注册账号", style = MaterialTheme.typography.headlineMedium)
            Spacer(Modifier.height(20.dp))

            OutlinedTextField(value = state.serverUrl, onValueChange = vm::setServerUrl, label = { Text("服务端 URL") },
                modifier = Modifier.fillMaxWidth(), singleLine = true)
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(value = state.email, onValueChange = vm::setEmail, label = { Text("邮箱") }, singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                modifier = Modifier.fillMaxWidth())
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(value = state.password, onValueChange = vm::setPassword, label = { Text("密码 (≥8 位)") },
                singleLine = true, visualTransformation = PasswordVisualTransformation(),
                modifier = Modifier.fillMaxWidth())
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(value = state.displayName, onValueChange = vm::setDisplayName, label = { Text("昵称(可选)") },
                singleLine = true, modifier = Modifier.fillMaxWidth())
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(value = state.timezone, onValueChange = vm::setTimezone, label = { Text("时区 (IANA)") },
                singleLine = true, modifier = Modifier.fillMaxWidth())

            if (state.error != null) {
                Spacer(Modifier.height(12.dp))
                Text(state.error!!, color = MaterialTheme.colorScheme.error)
            }

            Spacer(Modifier.height(20.dp))
            Button(onClick = vm::register, enabled = !state.isLoading,
                modifier = Modifier.fillMaxWidth().height(48.dp)) {
                if (state.isLoading) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onPrimary)
                else Text("注册")
            }

            Spacer(Modifier.height(12.dp))
            TextButton(onClick = onGotoLogin, modifier = Modifier.fillMaxWidth()) { Text("已有账号?去登录") }
        }
    }
}
