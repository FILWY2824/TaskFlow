package com.example.taskflow.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowLeft
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.taskflow.AppContainer
import com.example.taskflow.data.local.TodoCacheEntity
import com.example.taskflow.data.repository.Result
import com.example.taskflow.util.DateTimeFmt
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import java.time.LocalDate
import java.time.YearMonth

/**
 * 一个 6×7 的月历视图,每格上显示日期,如果该天有开始任务就在右下角点一个圆点。
 * 选中某天显示"该日所有 todo"列表(本地缓存,无网也能用)。
 *
 * 没引入第三方日历库;Compose + java.time 完全够用。
 */

data class CalendarUiState(
    val ym: YearMonth = YearMonth.now(),
    val selected: LocalDate = LocalDate.now(),
    val todos: List<TodoCacheEntity> = emptyList(),
    val tz: String = "Asia/Shanghai",
    val error: String? = null,
)

class CalendarViewModel(private val container: AppContainer) : ViewModel() {

    private val ymFlow = MutableStateFlow(YearMonth.now())
    private val selectedFlow = MutableStateFlow(LocalDate.now())
    private val errorFlow = MutableStateFlow<String?>(null)

    val state: StateFlow<CalendarUiState> = combine(
        listOf(
            container.todoRepository.observeAll(),
            ymFlow,
            selectedFlow,
            errorFlow,
        )
    ) { values ->
        @Suppress("UNCHECKED_CAST")
        val todos = values[0] as List<TodoCacheEntity>
        val ym = values[1] as YearMonth
        val selected = values[2] as LocalDate
        val err = values[3] as String?
        CalendarUiState(
            ym = ym,
            selected = selected,
            todos = todos,
            tz = container.tokenManager.current().timezone,
            error = err,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), CalendarUiState())

    init {
        // 进来先拉一次"全部"任务到本地缓存,日历需要看到 completed 的 + 未来的
        viewModelScope.launch {
            val r = container.todoRepository.refreshAll(filter = "all")
            if (r is Result.Error && r.code != "network") {
                errorFlow.value = r.message
            }
        }
    }

    fun nextMonth() { ymFlow.value = ymFlow.value.plusMonths(1) }
    fun prevMonth() { ymFlow.value = ymFlow.value.minusMonths(1) }
    fun gotoToday() {
        ymFlow.value = YearMonth.now()
        selectedFlow.value = LocalDate.now()
    }
    fun select(d: LocalDate) {
        selectedFlow.value = d
        // 跳到该月
        ymFlow.value = YearMonth.from(d)
    }

    fun clearError() { errorFlow.value = null }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = CalendarViewModel(container) as T
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CalendarScreen(
    container: AppContainer,
    onBack: () -> Unit,
    onOpenTodo: (Long) -> Unit,
) {
    val vm: CalendarViewModel = viewModel(factory = CalendarViewModel.Factory(container))
    val state by vm.state.collectAsState()

    TaskFlowErrorDialog(message = state.error, onDismiss = vm::clearError)

    val daysOfMonth = remember(state.todos, state.ym, state.tz) {
        groupTodosByDay(state.todos, state.ym, state.tz)
    }
    val selectedDay = state.selected.toString()
    val daysList = state.todos.filter { DateTimeFmt.localDate(taskStartAt(it), state.tz) == selectedDay }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("日历") },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "返回") } },
                actions = { TextButton(onClick = vm::gotoToday) { Text("今天") } },
            )
        },
    ) { padding ->
        Column(Modifier.padding(padding).fillMaxSize()) {
            // 月份切换
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IconButton(onClick = vm::prevMonth) { Icon(Icons.AutoMirrored.Filled.KeyboardArrowLeft, "上月") }
                Text(
                    "${state.ym.year}-${"%02d".format(state.ym.monthValue)}",
                    style = MaterialTheme.typography.titleLarge,
                    modifier = Modifier.weight(1f),
                    textAlign = TextAlign.Center,
                )
                IconButton(onClick = vm::nextMonth) { Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, "下月") }
            }

            // 星期表头
            Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp)) {
                listOf("一", "二", "三", "四", "五", "六", "日").forEach { d ->
                    Text(
                        d,
                        modifier = Modifier.weight(1f).padding(vertical = 4.dp),
                        textAlign = TextAlign.Center,
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }

            // 月历网格
            MonthGrid(
                ym = state.ym,
                selected = state.selected,
                daysWithTodos = daysOfMonth,
                onSelect = vm::select,
            )

            HorizontalDivider(Modifier.padding(top = 8.dp))

            // 选中日的任务
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    state.selected.toString(),
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                Text("${daysList.size} 项",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    style = MaterialTheme.typography.bodySmall)
            }
            if (daysList.isEmpty()) {
                Box(Modifier.fillMaxWidth().padding(20.dp), contentAlignment = Alignment.Center) {
                    Text("当天没有任务", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            } else {
                LazyColumn(modifier = Modifier.fillMaxSize()) {
                    items(daysList, key = { it.id }) { t ->
                        ListItem(
                            headlineContent = {
                                Text(
                                    t.title,
                                    textDecoration = if (t.is_completed) TextDecoration.LineThrough else null,
                                )
                            },
                            supportingContent = {
                                val startAt = taskStartAt(t)
                                if (!startAt.isNullOrBlank()) Text(
                                    DateTimeFmt.localTime(startAt, state.tz),
                                    style = MaterialTheme.typography.bodySmall,
                                )
                            },
                            modifier = Modifier.clickable { onOpenTodo(t.id) },
                        )
                        HorizontalDivider()
                    }
                }
            }
        }
    }
}

@Composable
private fun MonthGrid(
    ym: YearMonth,
    selected: LocalDate,
    daysWithTodos: Map<LocalDate, Int>,
    onSelect: (LocalDate) -> Unit,
) {
    val firstDay = ym.atDay(1)
    // ISO: Monday=1 .. Sunday=7
    val firstDow = firstDay.dayOfWeek.value     // 1..7
    val daysInMonth = ym.lengthOfMonth()
    // 网格起点:本月 1 号往前推到周一
    val gridStart = firstDay.minusDays((firstDow - 1).toLong())

    val today = LocalDate.now()

    Column(Modifier.padding(horizontal = 8.dp)) {
        // 6 行 × 7 列 = 42 格,足够覆盖任何月
        for (row in 0 until 6) {
            Row(Modifier.fillMaxWidth()) {
                for (col in 0 until 7) {
                    val date = gridStart.plusDays((row * 7 + col).toLong())
                    val inMonth = date.month == ym.month && date.year == ym.year
                    val isSelected = date == selected
                    val isToday = date == today
                    val count = daysWithTodos[date] ?: 0

                    val bg = when {
                        isSelected -> MaterialTheme.colorScheme.primary
                        isToday -> MaterialTheme.colorScheme.primaryContainer
                        else -> androidx.compose.ui.graphics.Color.Transparent
                    }
                    val fg = when {
                        isSelected -> MaterialTheme.colorScheme.onPrimary
                        isToday -> MaterialTheme.colorScheme.onPrimaryContainer
                        !inMonth -> MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.4f)
                        else -> MaterialTheme.colorScheme.onSurface
                    }

                    Box(
                        modifier = Modifier
                            .weight(1f)
                            .padding(2.dp)
                            .aspectRatio(1f)
                            .clip(RoundedCornerShape(8.dp))
                            .background(bg)
                            .clickable { onSelect(date) },
                        contentAlignment = Alignment.Center,
                    ) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            Text(
                                date.dayOfMonth.toString(),
                                color = fg,
                                fontWeight = if (isToday || isSelected) FontWeight.Bold else FontWeight.Normal,
                                fontSize = 14.sp,
                            )
                            if (count > 0 && inMonth) {
                                Spacer(Modifier.height(2.dp))
                                Box(
                                    Modifier
                                        .size(4.dp)
                                        .clip(CircleShape)
                                        .background(
                                            if (isSelected) MaterialTheme.colorScheme.onPrimary
                                            else MaterialTheme.colorScheme.primary
                                        ),
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

private fun groupTodosByDay(todos: List<TodoCacheEntity>, ym: YearMonth, tz: String): Map<LocalDate, Int> {
    val out = mutableMapOf<LocalDate, Int>()
    for (t in todos) {
        val d = DateTimeFmt.localDate(taskStartAt(t), tz)
        if (d.isBlank()) continue
        runCatching { LocalDate.parse(d) }.getOrNull()?.let { date ->
            // 只统计当前显示的月份 ± 1 周(网格里能看到的所有格)
            if (date.year == ym.year && date.month == ym.month) {
                out[date] = (out[date] ?: 0) + 1
            }
        }
    }
    return out
}
