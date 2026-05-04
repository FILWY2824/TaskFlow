package com.example.taskflow.ui.screens

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DatePicker
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TimePicker
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.material3.rememberTimePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.taskflow.AppContainer
import com.example.taskflow.util.DateTimeFmt
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import java.time.ZoneOffset
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
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回")
                    }
                },
                actions = {
                    if (state.todoId != null) {
                        IconButton(onClick = vm::delete) {
                            Icon(Icons.Default.Delete, "删除", tint = MaterialTheme.colorScheme.error)
                        }
                    }
                    TextButton(onClick = vm::save, enabled = !state.saving) {
                        Text(if (state.saving) "保存中" else "保存")
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
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            OutlinedTextField(
                value = state.title,
                onValueChange = vm::setTitle,
                label = { Text("标题 *") },
                modifier = Modifier.fillMaxWidth(),
                isError = state.error != null,
                singleLine = true,
            )
            OutlinedTextField(
                value = state.description,
                onValueChange = vm::setDescription,
                label = { Text("描述") },
                modifier = Modifier.fillMaxWidth().heightIn(min = 100.dp),
                maxLines = 6,
            )

            Text("优先级", style = MaterialTheme.typography.labelMedium)
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                listOf(0, 1, 2, 3, 4).forEach { p ->
                    FilterChip(
                        selected = state.priority == p,
                        onClick = { vm.setPriority(p) },
                        label = { Text(taskPriorityLabel(p)) },
                    )
                }
            }

            Text("预计时长", style = MaterialTheme.typography.labelMedium)
            DurationPicker(minutes = state.durationMinutes, onChange = vm::setDurationMinutes)

            ListItem(
                headlineContent = { Text("开始时间") },
                supportingContent = {
                    Text(state.startAtIso?.let { DateTimeFmt.localDateTime(it, state.timezone) } ?: "未设置")
                },
                trailingContent = {
                    Row {
                        TextButton(onClick = { showDateDialog = true }) { Text("选择") }
                        if (state.startAtIso != null) {
                            TextButton(onClick = { vm.setStartAt(null, false) }) { Text("清除") }
                        }
                    }
                },
            )

            if (state.todoId != null) {
                Spacer(Modifier.height(8.dp))
                Text("子任务", style = MaterialTheme.typography.titleMedium)
                state.subtasks.forEach { subtask ->
                    Row(
                        Modifier.fillMaxWidth().padding(vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Checkbox(
                            checked = subtask.is_completed,
                            onCheckedChange = { vm.toggleSubtask(subtask) },
                        )
                        Text(
                            subtask.title,
                            modifier = Modifier.weight(1f),
                            textDecoration = if (subtask.is_completed) TextDecoration.LineThrough else null,
                        )
                        IconButton(onClick = { vm.deleteSubtask(subtask.id) }) {
                            Icon(Icons.Default.Delete, "删除")
                        }
                    }
                }
                Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                    OutlinedTextField(
                        value = newSubtask,
                        onValueChange = { newSubtask = it },
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
                    }) { Text("添加") }
                }

                Spacer(Modifier.height(8.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("提醒", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                    TextButton(onClick = { showReminderDialog = true }) { Text("+ 新建") }
                }
                state.reminders.forEach { reminder ->
                    Row(
                        Modifier.fillMaxWidth().padding(vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Column(Modifier.weight(1f)) {
                            Text(reminder.title.ifEmpty { "未命名提醒" })
                            Text(
                                "下次: " + (reminder.next_fire_at?.let {
                                    DateTimeFmt.localDateTime(it, reminder.timezone)
                                } ?: "-"),
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        IconButton(onClick = { vm.deleteReminder(reminder.id) }) {
                            Icon(Icons.Default.Delete, "删除")
                        }
                    }
                }
            } else {
                Text(
                    "保存后可以继续添加子任务和强提醒。",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            Spacer(Modifier.height(40.dp))
        }
    }

    if (showDateDialog) {
        DateTimeDialDialog(
            timezone = state.timezone,
            initial = state.startAtIso,
            title = "选择开始时间",
            confirmText = "确定",
            onDismiss = { showDateDialog = false },
            onConfirm = { iso ->
                vm.setStartAt(iso, false)
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

@Composable
private fun DurationPicker(
    minutes: Int,
    onChange: (Int) -> Unit,
) {
    val options = listOf(0, 15, 30, 45, 60, 90, 120)
    var custom by remember(minutes) { mutableStateOf(if (minutes == 0) "" else minutes.toString()) }

    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            options.forEach { value ->
                FilterChip(
                    selected = minutes == value,
                    onClick = { onChange(value) },
                    label = { Text(if (value == 0) "不设置" else durationText(value)) },
                )
            }
        }
        OutlinedTextField(
            value = custom,
            onValueChange = { raw ->
                val clean = raw.filter { it.isDigit() }.take(4)
                custom = clean
                onChange((clean.toIntOrNull() ?: 0).coerceIn(0, 1440))
            },
            label = { Text("自定义分钟数") },
            supportingText = { Text("最多 24 小时，用来规划这个任务预计占用的时间") },
            modifier = Modifier.fillMaxWidth(),
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        )
    }
}

private fun durationText(minutes: Int): String {
    val safe = minutes.coerceAtLeast(0)
    if (safe == 0) return "不设置"
    val hours = safe / 60
    val mins = safe % 60
    return when {
        hours > 0 && mins > 0 -> "${hours}小时${mins}分钟"
        hours > 0 -> "${hours}小时"
        else -> "${mins}分钟"
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DateTimeDialDialog(
    timezone: String,
    initial: String?,
    title: String,
    confirmText: String,
    onDismiss: () -> Unit,
    onConfirm: (iso: String) -> Unit,
) {
    val tz = try { ZoneId.of(timezone) } catch (_: Exception) { ZoneId.systemDefault() }
    val initZdt = initial?.let {
        runCatching { ZonedDateTime.ofInstant(Instant.parse(it), tz) }.getOrNull()
    }
    val initialDate = initZdt?.toLocalDate() ?: LocalDate.now(tz)
    val dateState = rememberDatePickerState(initialSelectedDateMillis = datePickerMillis(initialDate))
    val timeState = rememberTimePickerState(
        initialHour = initZdt?.hour ?: 9,
        initialMinute = initZdt?.minute ?: 0,
        is24Hour = true,
    )

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column(
                Modifier
                    .heightIn(max = 560.dp)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                DatePicker(state = dateState, showModeToggle = true)
                Surface(
                    shape = MaterialTheme.shapes.medium,
                    color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.45f),
                ) {
                    Column(
                        Modifier.fillMaxWidth().padding(12.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        Text("时间", style = MaterialTheme.typography.labelLarge)
                        Spacer(Modifier.height(8.dp))
                        TimePicker(state = timeState)
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val date = selectedLocalDate(dateState.selectedDateMillis, initialDate)
                val iso = ZonedDateTime.of(date, LocalTime.of(timeState.hour, timeState.minute), tz)
                    .toInstant()
                    .toString()
                onConfirm(iso)
            }) { Text(confirmText) }
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
    val now = LocalTime.now(tz)
    val initialDate = LocalDate.now(tz)
    val dateState = rememberDatePickerState(initialSelectedDateMillis = datePickerMillis(initialDate))
    val timeState = rememberTimePickerState(
        initialHour = now.hour,
        initialMinute = (((now.minute / 5) + 1) * 5).coerceAtMost(59),
        is24Hour = true,
    )
    var title by remember { mutableStateOf("") }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("新建提醒") },
        text = {
            Column(
                Modifier
                    .heightIn(max = 600.dp)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                OutlinedTextField(
                    value = title,
                    onValueChange = { title = it },
                    label = { Text("标题，可选") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                )
                DatePicker(state = dateState, showModeToggle = true)
                Surface(
                    shape = MaterialTheme.shapes.medium,
                    color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.45f),
                ) {
                    Column(
                        Modifier.fillMaxWidth().padding(12.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        Text("提醒开始时间", style = MaterialTheme.typography.labelLarge)
                        Spacer(Modifier.height(8.dp))
                        TimePicker(state = timeState)
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val date = selectedLocalDate(dateState.selectedDateMillis, initialDate)
                val iso = ZonedDateTime.of(date, LocalTime.of(timeState.hour, timeState.minute), tz)
                    .toInstant()
                    .toString()
                onConfirm(iso, title)
            }) { Text("添加") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}

private fun datePickerMillis(date: LocalDate): Long =
    date.atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli()

private fun selectedLocalDate(millis: Long?, fallback: LocalDate): LocalDate =
    millis?.let { Instant.ofEpochMilli(it).atZone(ZoneOffset.UTC).toLocalDate() } ?: fallback
