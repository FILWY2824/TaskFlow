package com.example.taskflow.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
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
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Send
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.ShowChart
import androidx.compose.material.icons.filled.Timer
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
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
import androidx.compose.ui.graphics.vector.ImageVector
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

    var showMenu by remember { mutableStateOf(false) }
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
                            Text("TaskFlow", style = MaterialTheme.typography.titleMedium)
                            Text(
                                if (state.isOffline) "离线缓存模式" else "今日工作台",
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
                    IconButton(onClick = vm::refresh) { Icon(Icons.Default.Refresh, "刷新") }
                    IconButton(onClick = { showMenu = true }) { Icon(Icons.Default.MoreVert, "更多") }
                    DropdownMenu(expanded = showMenu, onDismissRequest = { showMenu = false }) {
                        DropdownMenuItem(text = { Text("日历") }, onClick = { showMenu = false; onOpenCalendar() })
                        DropdownMenuItem(text = { Text("通知") }, onClick = { showMenu = false; onOpenNotifications() })
                        DropdownMenuItem(text = { Text("番茄钟") }, onClick = { showMenu = false; onOpenPomodoro() })
                        DropdownMenuItem(text = { Text("统计") }, onClick = { showMenu = false; onOpenStats() })
                        DropdownMenuItem(text = { Text("Telegram") }, onClick = { showMenu = false; onOpenTelegram() })
                        DropdownMenuItem(text = { Text("权限自检") }, onClick = { showMenu = false; onOpenPermissions() })
                        HorizontalDivider()
                        DropdownMenuItem(text = { Text("设置") }, onClick = { showMenu = false; onOpenSettings() })
                    }
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
        LazyColumn(
            modifier = Modifier.padding(padding).fillMaxSize(),
            contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 10.dp, bottom = 96.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item {
                HomeSummary(
                    state = state,
                    onOpenCalendar = onOpenCalendar,
                    onOpenNotifications = onOpenNotifications,
                    onOpenPomodoro = onOpenPomodoro,
                    onOpenStats = onOpenStats,
                )
            }
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
                FilterRow(current = state.filter, onSelect = vm::setFilter)
            }

            if (state.items.isEmpty() && !state.isRefreshing) {
                item {
                    EmptyProductState(
                        title = if (state.searchQuery.isBlank()) "这一栏暂时很安静" else "没有匹配的任务",
                        body = if (state.searchQuery.isBlank()) {
                            "可以先新建一个任务，或者切换到“全部”查看其他待办。"
                        } else {
                            "换一个关键词试试，或者清空搜索条件。"
                        },
                        action = {
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                Button(onClick = { onOpenTodo(null) }, enabled = !state.isOffline) {
                                    Text("新建任务")
                                }
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

@Composable
private fun HomeSummary(
    state: TasksUiState,
    onOpenCalendar: () -> Unit,
    onOpenNotifications: () -> Unit,
    onOpenPomodoro: () -> Unit,
    onOpenStats: () -> Unit,
) {
    val all = state.allItems
    val open = all.count { !it.is_completed }
    val today = all.count { !it.is_completed && DateTimeFmt.isToday(it.due_at, state.tz) }
    val overdue = all.count { !it.is_completed && DateTimeFmt.isOverdue(it.due_at, it.completed_at) }
    val done = all.count { it.is_completed }

    ProductCard(tonal = true) {
        ScreenIntro(
            title = "今天要处理什么？",
            subtitle = if (today == 0) "没有必须今天完成的任务，可以从容安排。" else "今天有 $today 项任务需要关注。",
            trailing = {
                StatusPill(if (state.isRefreshing) "同步中" else "已同步")
            },
        )
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            MetricTile("待办", open.toString(), Modifier.weight(1f))
            MetricTile("今天", today.toString(), Modifier.weight(1f), MaterialTheme.colorScheme.tertiary)
            MetricTile("逾期", overdue.toString(), Modifier.weight(1f), MaterialTheme.colorScheme.error)
            MetricTile("完成", done.toString(), Modifier.weight(1f), MaterialTheme.colorScheme.secondary)
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            QuickAction("日历", Icons.Default.CalendarMonth, onOpenCalendar, Modifier.weight(1f))
            QuickAction("通知", Icons.Default.Notifications, onOpenNotifications, Modifier.weight(1f))
            QuickAction("番茄", Icons.Default.Timer, onOpenPomodoro, Modifier.weight(1f))
            QuickAction("统计", Icons.Default.ShowChart, onOpenStats, Modifier.weight(1f))
        }
    }
}

@Composable
private fun QuickAction(
    label: String,
    icon: ImageVector,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Surface(
        modifier = modifier
            .height(70.dp)
            .clickable { onClick() },
        shape = RoundedCornerShape(8.dp),
        color = MaterialTheme.colorScheme.surface.copy(alpha = 0.82f),
    ) {
        Column(
            Modifier.padding(10.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.height(4.dp))
            Text(label, style = MaterialTheme.typography.labelMedium)
        }
    }
}

@Composable
private fun FilterRow(current: TaskFilter, onSelect: (TaskFilter) -> Unit) {
    LazyRow(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        contentPadding = PaddingValues(vertical = 2.dp),
    ) {
        items(TaskFilter.entries) { f ->
            FilterChip(
                selected = current == f,
                onClick = { onSelect(f) },
                label = { Text(f.label, maxLines = 1) },
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
                "当前显示的是本地缓存。为避免下次同步覆盖服务端，离线时不会执行新增、删除或完成任务。",
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
                if (todo.duration_minutes > 0) {
                    Text(
                        durationLabel(todo.duration_minutes),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.primary,
                    )
                    Spacer(Modifier.height(6.dp))
                }
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    if (isOverdue) StatusPill("已逾期", MaterialTheme.colorScheme.error)
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
    priority == 3 -> Color(0xFFEA580C)
    priority == 2 -> Color(0xFF0F766E)
    priority == 1 -> Color(0xFF2563EB)
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
