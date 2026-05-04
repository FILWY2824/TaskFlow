package com.example.taskflow.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.taskflow.AppContainer
import com.example.taskflow.util.DateTimeFmt
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZonedDateTime

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TodoEditScreen(
    container: AppContainer,
    todoId: Long?,
    onBack: () -> Unit,
) {
    val vm: TodoEditViewModel = viewModel(
        key = "todo-edit-${todoId ?: "new"}",
        factory = TodoEditViewModel.Factory(container, todoId),
    )
    val state by vm.state.collectAsState()

    LaunchedEffect(state.saved, state.deleted) {
        if (state.saved || state.deleted) onBack()
    }

    var newSubtask by remember { mutableStateOf("") }
    var showReminderDialog by remember { mutableStateOf(false) }
    var showDateDialog by remember { mutableStateOf(false) }

    TaskFlowErrorDialog(message = state.error, onDismiss = vm::clearError)

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(if (todoId == null) "新建任务" else "编辑任务") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
                actions = {
                    if (state.todoId != null) IconButton(onClick = vm::delete) {
                        Icon(Icons.Default.Delete, "删除", tint = MaterialTheme.colorScheme.error)
                    }
                    TextButton(onClick = vm::save, enabled = !state.saving) {
                        Text(if (state.saving) "..." else "保存")
                    }
                },
            )
        },
    ) { padding ->
        Column(
            Modifier
                .padding(padding)
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
        ) {
            OutlinedTextField(
                value = state.title, onValueChange = vm::setTitle,
                label = { Text("标题 *") }, modifier = Modifier.fillMaxWidth(),
                isError = state.error != null,
            )
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                value = state.description, onValueChange = vm::setDescription,
                label = { Text("描述") }, modifier = Modifier.fillMaxWidth().heightIn(min = 100.dp),
                maxLines = 6,
            )
            Spacer(Modifier.height(12.dp))

            Text("优先级", style = MaterialTheme.typography.labelMedium)
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                listOf(0, 1, 2, 3, 4).forEach { p ->
                    FilterChip(
                        selected = state.priority == p,
                        onClick = { vm.setPriority(p) },
                        label = { Text(if (p == 0) "无" else "P$p") },
                    )
                }
            }

            Spacer(Modifier.height(12.dp))
            ListItem(
                headlineContent = { Text("截止时间") },
                supportingContent = {
                    Text(state.dueAtIso?.let { DateTimeFmt.localDateTime(it, state.timezone) } ?: "未设置")
                },
                trailingContent = {
                    Row {
                        TextButton(onClick = { showDateDialog = true }) { Text("选择") }
                        if (state.dueAtIso != null) TextButton(onClick = { vm.setDueAt(null, false) }) { Text("清除") }
                    }
                },
            )

            // === 子任务 ===
            if (state.todoId != null) {
                Spacer(Modifier.height(20.dp))
                Text("子任务", style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(8.dp))
                state.subtasks.forEach { s ->
                    Row(
                        Modifier.fillMaxWidth().padding(vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Checkbox(checked = s.is_completed, onCheckedChange = { vm.toggleSubtask(s) })
                        Text(
                            s.title, modifier = Modifier.weight(1f),
                            textDecoration = if (s.is_completed) TextDecoration.LineThrough else null,
                        )
                        IconButton(onClick = { vm.deleteSubtask(s.id) }) {
                            Icon(Icons.Default.Delete, "删除")
                        }
                    }
                }
                Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                    OutlinedTextField(
                        value = newSubtask, onValueChange = { newSubtask = it },
                        label = { Text("添加子任务") },
                        modifier = Modifier.weight(1f),
                        singleLine = true,
                    )
                    Spacer(Modifier.width(8.dp))
                    Button(onClick = {
                        if (newSubtask.isNotBlank()) {
                            vm.addSubtask(newSubtask)
                            newSubtask = ""
                        }
                    }) { Text("加") }
                }

                // === 提醒 ===
                Spacer(Modifier.height(20.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("提醒", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                    TextButton(onClick = { showReminderDialog = true }) { Text("+ 新建") }
                }
                state.reminders.forEach { r ->
                    Row(
                        Modifier.fillMaxWidth().padding(vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Column(Modifier.weight(1f)) {
                            Text(r.title.ifEmpty { "(无标题)" })
                            Text(
                                "下次:" + (r.next_fire_at?.let { DateTimeFmt.localDateTime(it, r.timezone) } ?: "—"),
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        IconButton(onClick = { vm.deleteReminder(r.id) }) {
                            Icon(Icons.Default.Delete, "删除")
                        }
                    }
                }
            } else {
                Spacer(Modifier.height(12.dp))
                Text("保存后可以添加子任务和提醒", color = MaterialTheme.colorScheme.onSurfaceVariant,
                    style = MaterialTheme.typography.bodySmall)
            }

            Spacer(Modifier.height(40.dp))
        }
    }

    if (showDateDialog) {
        DueDateDialog(
            timezone = state.timezone,
            initial = state.dueAtIso,
            onDismiss = { showDateDialog = false },
            onConfirm = { iso, allDay ->
                vm.setDueAt(iso, allDay)
                showDateDialog = false
            },
        )
    }

    if (showReminderDialog) {
        AddReminderDialog(
            timezone = state.timezone,
            onDismiss = { showReminderDialog = false },
            onConfirm = { iso, title ->
                vm.addReminder(iso, title)
                showReminderDialog = false
            },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DueDateDialog(
    timezone: String,
    initial: String?,
    onDismiss: () -> Unit,
    onConfirm: (iso: String?, allDay: Boolean) -> Unit,
) {
    val tz = try { ZoneId.of(timezone) } catch (_: Exception) { ZoneId.systemDefault() }
    val initZdt = initial?.let { try { ZonedDateTime.ofInstant(Instant.parse(it), tz) } catch (_: Exception) { null } }
    var date by remember { mutableStateOf(initZdt?.toLocalDate() ?: LocalDate.now(tz)) }
    var hour by remember { mutableIntStateOf(initZdt?.hour ?: 9) }
    var minute by remember { mutableIntStateOf(initZdt?.minute ?: 0) }
    var dateText by remember { mutableStateOf(date.toString()) }
    var hhmm by remember { mutableStateOf("%02d:%02d".format(hour, minute)) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("设置截止时间") },
        text = {
            Column {
                OutlinedTextField(
                    value = dateText, onValueChange = {
                        dateText = it
                        runCatching { date = LocalDate.parse(it) }
                    },
                    label = { Text("日期 (YYYY-MM-DD)") }, singleLine = true,
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = hhmm, onValueChange = {
                        hhmm = it
                        val parts = it.split(":")
                        if (parts.size == 2) {
                            parts[0].toIntOrNull()?.let { h -> if (h in 0..23) hour = h }
                            parts[1].toIntOrNull()?.let { m -> if (m in 0..59) minute = m }
                        }
                    },
                    label = { Text("时间 (HH:mm)") }, singleLine = true,
                )
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val iso = ZonedDateTime.of(date, LocalTime.of(hour, minute), tz)
                    .toInstant().toString()
                onConfirm(iso, false)
            }) { Text("确定") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AddReminderDialog(
    timezone: String,
    onDismiss: () -> Unit,
    onConfirm: (triggerAtIso: String, title: String) -> Unit,
) {
    val tz = try { ZoneId.of(timezone) } catch (_: Exception) { ZoneId.systemDefault() }
    var date by remember { mutableStateOf(LocalDate.now(tz)) }
    var hour by remember { mutableIntStateOf(LocalTime.now(tz).hour) }
    var minute by remember { mutableIntStateOf(((LocalTime.now(tz).minute / 5) + 1) * 5 % 60) }
    var dateText by remember { mutableStateOf(date.toString()) }
    var hhmm by remember { mutableStateOf("%02d:%02d".format(hour, minute)) }
    var title by remember { mutableStateOf("") }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("新建提醒") },
        text = {
            Column {
                OutlinedTextField(value = title, onValueChange = { title = it },
                    label = { Text("标题(可选)") }, singleLine = true)
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(value = dateText, onValueChange = {
                    dateText = it; runCatching { date = LocalDate.parse(it) }
                }, label = { Text("日期 (YYYY-MM-DD)") }, singleLine = true)
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(value = hhmm, onValueChange = {
                    hhmm = it
                    val parts = it.split(":")
                    if (parts.size == 2) {
                        parts[0].toIntOrNull()?.let { h -> if (h in 0..23) hour = h }
                        parts[1].toIntOrNull()?.let { m -> if (m in 0..59) minute = m }
                    }
                }, label = { Text("时间 (HH:mm)") }, singleLine = true)
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val iso = ZonedDateTime.of(date, LocalTime.of(hour, minute), tz)
                    .toInstant().toString()
                onConfirm(iso, title)
            }) { Text("添加") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}
