package com.example.todoalarm.ui.screens

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.example.todoalarm.AppContainer
import com.example.todoalarm.data.local.TodoCacheEntity
import com.example.todoalarm.data.repository.Result
import com.example.todoalarm.util.DateTimeFmt
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

enum class TaskFilter(val label: String, val server: String?) {
    Today("今天", "today"),
    Tomorrow("明天", "tomorrow"),
    ThisWeek("本周", "this_week"),
    Overdue("已逾期", "overdue"),
    NoDate("无日期", "nodate"),
    All("全部", null),
    Completed("已完成", "completed"),
}

data class TasksUiState(
    val isRefreshing: Boolean = false,
    val error: String? = null,
    val filter: TaskFilter = TaskFilter.Today,
    val items: List<TodoCacheEntity> = emptyList(),
    /** 用户时区,用于本地过滤 */
    val tz: String = "UTC",
    /** 服务端是否在线(影响"无网络"提示) */
    val isOffline: Boolean = false,
    val searchQuery: String = "",
)

class TasksViewModel(private val container: AppContainer) : ViewModel() {

    private val filterFlow = MutableStateFlow(TaskFilter.Today)
    private val errorFlow = MutableStateFlow<String?>(null)
    private val refreshingFlow = MutableStateFlow(false)
    private val offlineFlow = MutableStateFlow(false)
    private val searchFlow = MutableStateFlow("")

    val state: StateFlow<TasksUiState> = combine(
        listOf(
            container.todoRepository.observeAll(),
            filterFlow,
            refreshingFlow,
            errorFlow,
            offlineFlow,
            searchFlow,
        )
    ) { values ->
        @Suppress("UNCHECKED_CAST")
        val all = values[0] as List<TodoCacheEntity>
        val filter = values[1] as TaskFilter
        val refreshing = values[2] as Boolean
        val err = values[3] as String?
        val offline = values[4] as Boolean
        val search = values[5] as String
        TasksUiState(
            items = applyFilter(all, filter, container.tokenManager.current().timezone, search),
            filter = filter,
            isRefreshing = refreshing,
            error = err,
            tz = container.tokenManager.current().timezone,
            isOffline = offline,
            searchQuery = search,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), TasksUiState())

    init { refresh() }

    fun setFilter(f: TaskFilter) { filterFlow.value = f }

    fun setSearch(q: String) {
        searchFlow.value = q
        // 服务端搜索是可选优化;Web 端做了 debounce + 服务端,这里先做纯本地过滤即可。
        // 如果用户希望命中"已归档"等没缓存的内容,可以再触发一次 refresh(search=q) — 但本地已经够用。
    }

    fun refresh() {
        refreshingFlow.value = true
        errorFlow.value = null
        viewModelScope.launch {
            val r = container.todoRepository.refreshAll(
                filter = filterFlow.value.server,
                listId = null,
                search = null,
            )
            refreshingFlow.value = false
            when (r) {
                is Result.Error -> {
                    if (r.code == "network") {
                        offlineFlow.value = true
                    } else {
                        errorFlow.value = r.message
                    }
                }
                is Result.Success -> { offlineFlow.value = false }
            }
        }
    }

    fun complete(id: Long, completed: Boolean) {
        viewModelScope.launch {
            val r = if (completed) container.todoRepository.complete(id)
                    else container.todoRepository.uncomplete(id)
            if (r is Result.Error && r.code != "network") {
                errorFlow.value = r.message
            }
        }
    }

    fun delete(id: Long) {
        viewModelScope.launch {
            val r = container.todoRepository.delete(id)
            if (r is Result.Error && r.code != "network") {
                errorFlow.value = r.message
            }
        }
    }

    private fun applyFilter(all: List<TodoCacheEntity>, f: TaskFilter, tz: String, search: String): List<TodoCacheEntity> {
        val byFilter = when (f) {
            TaskFilter.Today -> all.filter { !it.is_completed && DateTimeFmt.isToday(it.due_at, tz) }
            TaskFilter.Tomorrow -> {
                val tomorrow = DateTimeFmt.nowLocalDate(tz).plusDays(1)
                all.filter {
                    !it.is_completed && it.due_at != null &&
                        DateTimeFmt.localDate(it.due_at, tz) == tomorrow.toString()
                }
            }
            TaskFilter.ThisWeek -> {
                val today = DateTimeFmt.nowLocalDate(tz)
                val end = today.plusDays(7)
                all.filter { e ->
                    !e.is_completed && e.due_at != null && run {
                        val d = DateTimeFmt.localDate(e.due_at, tz)
                        d.isNotBlank() && d >= today.toString() && d < end.toString()
                    }
                }
            }
            TaskFilter.Overdue -> all.filter { !it.is_completed && DateTimeFmt.isOverdue(it.due_at, it.completed_at) }
            TaskFilter.NoDate -> all.filter { !it.is_completed && it.due_at.isNullOrBlank() }
            TaskFilter.All -> all.filter { !it.is_completed }
            TaskFilter.Completed -> all.filter { it.is_completed }
        }
        if (search.isBlank()) return byFilter
        val q = search.trim().lowercase()
        return byFilter.filter {
            it.title.lowercase().contains(q) || it.description.lowercase().contains(q)
        }
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = TasksViewModel(container) as T
    }
}
