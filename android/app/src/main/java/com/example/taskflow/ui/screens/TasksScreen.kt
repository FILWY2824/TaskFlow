package com.example.taskflow.ui.screens

import androidx.compose.foundation.BorderStroke
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
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
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
                    if (showSearch) {
                        TextField(
                            value = state.searchQuery,
                            onValueChange = vm::setSearch,
                            placeholder = { Text("搜索任务") },
                            singleLine = true,
                            modifier = Modifier.fillMaxWidth(),
                            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                            trailingIcon = {
                                if (state.searchQuery.isNotEmpty()) {
                                    IconButton(onClick = { vm.setSearch("") }) {
                                        Icon(Icons.Default.Clear, "清除搜索")
                                    }
                                }
                            },
                            colors = TextFieldDefaults.colors(
                                focusedContainerColor = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.34f),
                                unfocusedContainerColor = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.34f),
                                focusedIndicatorColor = Color.Transparent,
                                unfocusedIndicatorColor = Color.Transparent,
                            ),
                            shape = RoundedCornerShape(18.dp),
                        )
                    } else {
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
                    }) {
                        Icon(if (showSearch) Icons.Default.Clear else Icons.Default.Search, "搜索")
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
                item {
                    TaskFilterBar(
                        dateFilter = state.dateFilter,
                        statusFilter = state.statusFilter,
                        onDate = vm::setDateFilter,
                        onStatus = vm::setStatusFilter,
                    )
                }

                if (state.items.isEmpty() && !state.isRefreshing) {
                    item {
                        CenterHint(
                            text = if (state.searchQuery.isBlank()) "没有任务" else "没有匹配结果",
                            modifier = Modifier.padding(top = 40.dp),
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
private fun TaskFilterBar(
    dateFilter: TaskDateFilter,
    statusFilter: TaskStatusFilter,
    onDate: (TaskDateFilter) -> Unit,
    onStatus: (TaskStatusFilter) -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        DateFilterSelector(
            current = dateFilter,
            onSelect = onDate,
            modifier = Modifier.weight(1f),
        )
        StatusFilterSelector(
            current = statusFilter,
            onSelect = onStatus,
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun DateFilterSelector(
    current: TaskDateFilter,
    onSelect: (TaskDateFilter) -> Unit,
    modifier: Modifier = Modifier,
) {
    val options = listOf(
        TaskDateFilter.Today,
        TaskDateFilter.Tomorrow,
        TaskDateFilter.ThisWeek,
        TaskDateFilter.RecentWeek,
        TaskDateFilter.RecentMonth,
        TaskDateFilter.All,
        TaskDateFilter.Scheduled,
        TaskDateFilter.NoDate,
    )
    SelectorPill(
        label = "日期",
        value = current.label,
        options = options.map { it.label },
        selectedIndex = options.indexOf(current).coerceAtLeast(0),
        onSelectIndex = { onSelect(options[it]) },
        modifier = modifier,
        accent = MaterialTheme.colorScheme.primary,
    )
}

@Composable
private fun StatusFilterSelector(
    current: TaskStatusFilter,
    onSelect: (TaskStatusFilter) -> Unit,
    modifier: Modifier = Modifier,
) {
    val options = TaskStatusFilter.entries
    SelectorPill(
        label = "状态",
        value = current.label,
        options = options.map { it.label },
        selectedIndex = options.indexOf(current).coerceAtLeast(0),
        onSelectIndex = { onSelect(options[it]) },
        modifier = modifier,
        accent = MaterialTheme.colorScheme.secondary,
    )
}

@Composable
private fun SelectorPill(
    label: String,
    value: String,
    options: List<String>,
    selectedIndex: Int,
    onSelectIndex: (Int) -> Unit,
    modifier: Modifier = Modifier,
    accent: Color = MaterialTheme.colorScheme.primary,
) {
    var expanded by remember { mutableStateOf(false) }
    Box(modifier = modifier) {
        Surface(
            modifier = Modifier.fillMaxWidth().clickable { expanded = true },
            shape = RoundedCornerShape(18.dp),
            color = MaterialTheme.colorScheme.surface,
            border = BorderStroke(1.dp, accent.copy(alpha = 0.22f)),
            shadowElevation = 0.dp,
        ) {
            Row(
                modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Column(Modifier.weight(1f)) {
                    Text(
                        label,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        value,
                        style = MaterialTheme.typography.labelLarge,
                        color = accent,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                Icon(Icons.Default.KeyboardArrowDown, contentDescription = null, tint = accent)
            }
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            options.forEachIndexed { index, option ->
                DropdownMenuItem(
                    text = {
                        Text(
                            option,
                            fontWeight = if (index == selectedIndex) FontWeight.SemiBold else FontWeight.Normal,
                            color = if (index == selectedIndex) accent else MaterialTheme.colorScheme.onSurface,
                        )
                    },
                    onClick = {
                        onSelectIndex(index)
                        expanded = false
                    },
                )
            }
        }
    }
}

@Composable
private fun OfflineNotice() {
    ProductCard {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatusPill("离线", MaterialTheme.colorScheme.error)
            Text(
                "当前显示本地缓存。离线时不能新增、删除或完成任务，避免下次同步覆盖云端状态。",
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
    val startAt = taskStartAt(todo)
    val isOverdue = DateTimeFmt.isOverdue(startAt, todo.completed_at)
    val accent = priorityColor(todo.priority, isOverdue)

    ElevatedCard(
        modifier = Modifier.fillMaxWidth().clickable { onClick() },
        shape = RoundedCornerShape(8.dp),
        colors = CardDefaults.elevatedCardColors(containerColor = MaterialTheme.colorScheme.surface),
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
                    priorityBadgeText(todo.priority),
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
                    if (!startAt.isNullOrBlank()) {
                        Text(
                            DateTimeFmt.localTime(startAt, tz),
                            style = MaterialTheme.typography.labelMedium,
                            color = if (isOverdue) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                Spacer(Modifier.height(4.dp))
                Text(
                    todo.description.ifBlank {
                        startAt?.let { "开始 ${DateTimeFmt.localDateTime(it, tz)}" } ?: "未设置开始时间"
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Spacer(Modifier.height(8.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    StatusPill(taskPriorityLabel(todo.priority), accent)
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

private fun priorityBadgeText(priority: Int): String = when (priority.coerceIn(0, 4)) {
    4 -> "急"
    else -> taskPriorityLabel(priority)
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
