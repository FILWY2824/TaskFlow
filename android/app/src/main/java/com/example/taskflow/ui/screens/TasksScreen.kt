package com.example.taskflow.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.taskflow.AppContainer
import com.example.taskflow.data.local.TodoCacheEntity
import com.example.taskflow.util.DateTimeFmt

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TasksScreen(
    container: AppContainer,
    onOpenTodo: (Long?) -> Unit,
    onOpenSettings: () -> Unit,
    onOpenNotifications: () -> Unit,
    onOpenStats: () -> Unit,
    onOpenPomodoro: () -> Unit,
    onOpenTelegram: () -> Unit,
    onOpenPermissions: () -> Unit,
    onOpenCalendar: () -> Unit,
) {
    val vm: TasksViewModel = viewModel(factory = TasksViewModel.Factory(container))
    val state by vm.state.collectAsState()

    var showMenu by remember { mutableStateOf(false) }
    var showSearch by remember { mutableStateOf(false) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    if (showSearch) {
                        OutlinedTextField(
                            value = state.searchQuery,
                            onValueChange = vm::setSearch,
                            placeholder = { Text("搜索标题 / 描述") },
                            singleLine = true,
                            modifier = Modifier.fillMaxWidth(),
                            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                            trailingIcon = if (state.searchQuery.isNotEmpty()) {
                                { IconButton(onClick = { vm.setSearch("") }) { Icon(Icons.Default.Clear, "清除") } }
                            } else null,
                        )
                    } else {
                        Text(state.filter.label)
                    }
                },
                actions = {
                    IconButton(onClick = {
                        if (showSearch) { vm.setSearch(""); showSearch = false } else showSearch = true
                    }) { Icon(if (showSearch) Icons.Default.Clear else Icons.Default.Search, "搜索") }
                    IconButton(onClick = vm::refresh) { Icon(Icons.Default.Refresh, "刷新") }
                    IconButton(onClick = { showMenu = true }) { Text("⋯") }
                    DropdownMenu(expanded = showMenu, onDismissRequest = { showMenu = false }) {
                        DropdownMenuItem(text = { Text("日历") }, onClick = { showMenu = false; onOpenCalendar() })
                        DropdownMenuItem(text = { Text("通知") }, onClick = { showMenu = false; onOpenNotifications() })
                        DropdownMenuItem(text = { Text("番茄钟") }, onClick = { showMenu = false; onOpenPomodoro() })
                        DropdownMenuItem(text = { Text("统计") }, onClick = { showMenu = false; onOpenStats() })
                        DropdownMenuItem(text = { Text("Telegram") }, onClick = { showMenu = false; onOpenTelegram() })
                        DropdownMenuItem(text = { Text("权限自检") }, onClick = { showMenu = false; onOpenPermissions() })
                        Divider()
                        DropdownMenuItem(text = { Text("设置") }, onClick = { showMenu = false; onOpenSettings() })
                    }
                },
            )
        },
        floatingActionButton = {
            if (!state.isOffline) {
                FloatingActionButton(onClick = { onOpenTodo(null) }) {
                    Icon(Icons.Default.Add, "添加")
                }
            }
        },
    ) { padding ->
        Column(Modifier.padding(padding).fillMaxSize()) {
            FilterRow(
                current = state.filter,
                onSelect = vm::setFilter,
            )

            if (state.isOffline) {
                OfflineBanner()
            }
            if (state.error != null) {
                ErrorBanner(state.error!!)
            }

            if (state.items.isEmpty() && !state.isRefreshing) {
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Text("空空如也", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            } else {
                LazyColumn(modifier = Modifier.fillMaxSize(), contentPadding = PaddingValues(vertical = 8.dp)) {
                    items(state.items, key = { it.id }) { todo ->
                        TodoRow(
                            todo = todo,
                            tz = state.tz,
                            isOffline = state.isOffline,
                            onToggle = { vm.complete(todo.id, !todo.is_completed) },
                            onClick = { onOpenTodo(todo.id) },
                            onDelete = { vm.delete(todo.id) },
                        )
                        Divider()
                    }
                }
            }
        }
    }
}

@Composable
private fun FilterRow(current: TaskFilter, onSelect: (TaskFilter) -> Unit) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        TaskFilter.entries.forEach { f ->
            FilterChip(
                selected = current == f,
                onClick = { onSelect(f) },
                label = { Text(f.label, maxLines = 1) },
            )
        }
    }
}

@Composable
private fun OfflineBanner() {
    Surface(color = MaterialTheme.colorScheme.errorContainer) {
        Text(
            "⚠ 当前离线 — 显示的是本地缓存，新增 / 删除任务已停用",
            color = MaterialTheme.colorScheme.onErrorContainer,
            modifier = Modifier.fillMaxWidth().padding(12.dp),
        )
    }
}

@Composable
private fun ErrorBanner(msg: String) {
    Surface(color = MaterialTheme.colorScheme.errorContainer) {
        Text(msg, modifier = Modifier.padding(12.dp), color = MaterialTheme.colorScheme.onErrorContainer)
    }
}

@Composable
private fun TodoRow(
    todo: TodoCacheEntity,
    tz: String,
    isOffline: Boolean,
    onToggle: () -> Unit,
    onClick: () -> Unit,
    onDelete: () -> Unit,
) {
    Row(
        Modifier
            .fillMaxWidth()
            .clickable { onClick() }
            .padding(vertical = 10.dp, horizontal = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Checkbox(checked = todo.is_completed, onCheckedChange = { onToggle() })
        Spacer(Modifier.width(8.dp))
        Column(Modifier.weight(1f)) {
            Text(
                todo.title,
                style = MaterialTheme.typography.bodyLarge,
                textDecoration = if (todo.is_completed) TextDecoration.LineThrough else null,
                color = if (todo.is_completed) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
            )
            if (!todo.due_at.isNullOrBlank()) {
                Spacer(Modifier.height(2.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    val isOverdue = DateTimeFmt.isOverdue(todo.due_at, todo.completed_at)
                    Text(
                        DateTimeFmt.localDateTime(todo.due_at, tz),
                        style = MaterialTheme.typography.bodySmall,
                        color = if (isOverdue) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    if (todo.priority > 0) {
                        Spacer(Modifier.width(8.dp))
                        Box(
                            Modifier
                                .background(MaterialTheme.colorScheme.primaryContainer, RoundedCornerShape(4.dp))
                                .padding(horizontal = 6.dp, vertical = 2.dp),
                        ) {
                            Text("P${todo.priority}", style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onPrimaryContainer)
                        }
                    }
                }
            }
        }
        if (!isOffline) {
            IconButton(onClick = onDelete) {
                Icon(Icons.Default.Delete, "删除", tint = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}
