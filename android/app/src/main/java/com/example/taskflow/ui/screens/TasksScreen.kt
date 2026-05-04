package com.example.taskflow.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
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
    var showSearch by remember { mutableStateOf(false) }

    TaskFlowErrorDialog(message = state.error, onDismiss = vm::clearError)

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            modifier = Modifier
                                .size(34.dp)
                                .clip(CircleShape)
                                .background(MaterialTheme.colorScheme.primary),
                            contentAlignment = Alignment.Center,
                        ) {
                            Text("T", color = MaterialTheme.colorScheme.onPrimary, fontWeight = FontWeight.Bold)
                        }
                        Spacer(Modifier.width(10.dp))
                        Column {
                            Text("TaskFlow", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                            Text(
                                if (state.isOffline) "离线缓存模式" else "下拉刷新，轻扫筛选",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                },
                actions = {
                    IconButton(onClick = {
                        if (showSearch) {
                            vm.setSearch("")
                            showSearch = false
                        } else {
                            showSearch = true
                        }
                    }) { Icon(if (showSearch) Icons.Default.Clear else Icons.Default.Search, "搜索") }
                },
            )
        },
        floatingActionButton = {
            if (!state.isOffline) {
                FloatingActionButton(onClick = { onOpenTodo(null) }) {
                    Icon(Icons.Default.Add, "添加任务")
                }
            }
        },
        containerColor = MaterialTheme.colorScheme.background,
    ) { padding ->
        PullToRefreshBox(
            isRefreshing = state.isRefreshing,
            onRefresh = vm::refresh,
            modifier = Modifier.padding(padding).fillMaxSize(),
        ) {
            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 10.dp, bottom = 96.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                if (state.isOffline) {
                    item { OfflineNotice() }
                }
                if (showSearch) {
                    item {
                        OutlinedTextField(
                            value = state.searchQuery,
                            onValueChange = vm::setSearch,
                            placeholder = { Text("搜索标题、描述") },
                            singleLine = true,
                            modifier = Modifier.fillMaxWidth(),
                            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                            trailingIcon = if (state.searchQuery.isNotEmpty()) {
                                { IconButton(onClick = { vm.setSearch("") }) { Icon(Icons.Default.Clear, "清除") } }
                            } else null,
                        )
                    }
                }
                item {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        DateFilterRow(current = state.dateFilter, onSelect = vm::setDateFilter)
                        StatusFilterRow(current = state.statusFilter, onSelect = vm::setStatusFilter)
                    }
                }

                if (state.items.isEmpty() && !state.isRefreshing) {
                    item {
                        EmptyProductState(
                            title = if (state.searchQuery.isBlank()) "这一栏暂时很安静" else "没有匹配的任务",
                            body = if (state.searchQuery.isBlank()) {
                                "可以新建任务，或切换上面的日期和状态筛选查看其他事项。"
                            } else {
                                "换一个关键词，或清空搜索条件再试。"
                            },
                            action = {
                                Button(onClick = { onOpenTodo(null) }, enabled = !state.isOffline) {
                                    Text("新建任务")
                                }
                            },
                        )
                    }
                } else {
                    items(state.items, key = { it.id }) { todo ->
                        TodoRow(
                            todo = todo,
                            tz = state.tz,
                            isOffline = state.isOffline,
                            onToggle = { vm.complete(todo.id, !todo.is_completed) },
                            onClick = { onOpenTodo(todo.id) },
                            onDelete = { vm.delete(todo.id) },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun DateFilterRow(current: TaskDateFilter, onSelect: (TaskDateFilter) -> Unit) {
    val visible = listOf(
        TaskDateFilter.Today,
        TaskDateFilter.Tomorrow,
        TaskDateFilter.ThisWeek,
        TaskDateFilter.RecentWeek,
        TaskDateFilter.RecentMonth,
        TaskDateFilter.All,
        TaskDateFilter.Scheduled,
        TaskDateFilter.NoDate,
    )
    LazyRow(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        contentPadding = PaddingValues(vertical = 2.dp),
    ) {
        items(visible) { f ->
            FilterChip(
                selected = current == f,
                onClick = { onSelect(f) },
                label = { Text(f.label, maxLines = 1) },
                colors = FilterChipDefaults.filterChipColors(
                    selectedContainerColor = MaterialTheme.colorScheme.primaryContainer,
                    selectedLabelColor = MaterialTheme.colorScheme.onPrimaryContainer,
                ),
            )
        }
    }
}

@Composable
private fun StatusFilterRow(current: TaskStatusFilter, onSelect: (TaskStatusFilter) -> Unit) {
    LazyRow(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        contentPadding = PaddingValues(vertical = 2.dp),
    ) {
        items(TaskStatusFilter.entries) { f ->
            FilterChip(
                selected = current == f,
                onClick = { onSelect(f) },
                label = { Text(f.label, maxLines = 1) },
                colors = FilterChipDefaults.filterChipColors(
                    selectedContainerColor = MaterialTheme.colorScheme.secondaryContainer,
                    selectedLabelColor = MaterialTheme.colorScheme.onSecondaryContainer,
                ),
            )
        }
    }
}

@Composable
private fun OfflineNotice() {
    ProductCard {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatusPill("离线", MaterialTheme.colorScheme.error)
            Text(
                "当前显示本地缓存。离线时不会新增、删除或完成任务，避免下次同步覆盖云端状态。",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
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
    val isOverdue = DateTimeFmt.isOverdue(todo.due_at, todo.completed_at)
    val accent = priorityColor(todo.priority, isOverdue)

    ElevatedCard(
        modifier = Modifier.fillMaxWidth().clickable { onClick() },
        shape = RoundedCornerShape(8.dp),
    ) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                modifier = Modifier
                    .size(44.dp)
                    .clip(CircleShape)
                    .background(accent.copy(alpha = 0.14f)),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    if (todo.priority > 0) "P${todo.priority}" else todo.title.take(1).uppercase(),
                    color = accent,
                    fontWeight = FontWeight.SemiBold,
                    style = MaterialTheme.typography.labelLarge,
                )
            }
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        todo.title,
                        modifier = Modifier.weight(1f),
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.SemiBold,
                        textDecoration = if (todo.is_completed) TextDecoration.LineThrough else null,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    if (!todo.due_at.isNullOrBlank()) {
                        Text(
                            DateTimeFmt.localTime(todo.due_at, tz),
                            style = MaterialTheme.typography.labelMedium,
                            color = if (isOverdue) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                Spacer(Modifier.height(4.dp))
                Text(
                    todo.description.ifBlank {
                        todo.due_at?.let { "截止 ${DateTimeFmt.localDateTime(it, tz)}" } ?: "没有截止时间"
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Spacer(Modifier.height(8.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    if (todo.duration_minutes > 0) StatusPill(durationLabel(todo.duration_minutes), MaterialTheme.colorScheme.primary)
                    if (isOverdue) StatusPill("已过期", MaterialTheme.colorScheme.error)
                    if (todo.due_all_day) StatusPill("全天")
                    if (todo.is_completed) StatusPill("已完成", MaterialTheme.colorScheme.secondary)
                }
            }
            Spacer(Modifier.width(8.dp))
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Checkbox(checked = todo.is_completed, onCheckedChange = { onToggle() }, enabled = !isOffline)
                IconButton(onClick = onDelete, enabled = !isOffline) {
                    Icon(Icons.Default.Delete, "删除", tint = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

@Composable
private fun priorityColor(priority: Int, overdue: Boolean): Color = when {
    overdue -> MaterialTheme.colorScheme.error
    priority >= 4 -> Color(0xFFE11D48)
    priority == 3 -> Color(0xFFF97316)
    priority == 2 -> Color(0xFF10B981)
    priority == 1 -> Color(0xFF0EA5E9)
    else -> MaterialTheme.colorScheme.primary
}

private fun durationLabel(minutes: Int): String {
    val safe = minutes.coerceAtLeast(0)
    if (safe == 0) return "未设置"
    val hours = safe / 60
    val mins = safe % 60
    return when {
        hours > 0 && mins > 0 -> "${hours}小时${mins}分钟"
        hours > 0 -> "${hours}小时"
        else -> "${mins}分钟"
    }
}
