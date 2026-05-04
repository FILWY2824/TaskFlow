package com.example.taskflow.ui.screens

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.example.taskflow.AppContainer
import com.example.taskflow.data.local.TodoCacheEntity
import com.example.taskflow.data.repository.Result
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

data class TasksUiState(
    val isRefreshing: Boolean = false,
    val error: String? = null,
    val dateFilter: TaskDateFilter = TaskDateFilter.Today,
    val statusFilter: TaskStatusFilter = TaskStatusFilter.All,
    val items: List<TodoCacheEntity> = emptyList(),
    val allItems: List<TodoCacheEntity> = emptyList(),
    /** 用户时区,用于本地过滤 */
    val tz: String = "Asia/Shanghai",
    /** 服务端是否在线(影响"无网络"提示) */
    val isOffline: Boolean = false,
    val searchQuery: String = "",
)

class TasksViewModel(private val container: AppContainer) : ViewModel() {

    private val dateFilterFlow = MutableStateFlow(TaskDateFilter.Today)
    private val statusFilterFlow = MutableStateFlow(TaskStatusFilter.All)
    private val errorFlow = MutableStateFlow<String?>(null)
    private val refreshingFlow = MutableStateFlow(false)
    private val offlineFlow = MutableStateFlow(false)
    private val searchFlow = MutableStateFlow("")

    val state: StateFlow<TasksUiState> = combine(
        listOf(
            container.todoRepository.observeAll(),
            dateFilterFlow,
            statusFilterFlow,
            refreshingFlow,
            errorFlow,
            offlineFlow,
            searchFlow,
        )
    ) { values ->
        @Suppress("UNCHECKED_CAST")
        val all = values[0] as List<TodoCacheEntity>
        val dateFilter = values[1] as TaskDateFilter
        val statusFilter = values[2] as TaskStatusFilter
        val refreshing = values[3] as Boolean
        val err = values[4] as String?
        val offline = values[5] as Boolean
        val search = values[6] as String
        val timezone = container.tokenManager.current().timezone
        TasksUiState(
            items = filterTodosForAndroid(all, dateFilter, statusFilter, timezone, search),
            dateFilter = dateFilter,
            statusFilter = statusFilter,
            isRefreshing = refreshing,
            error = err,
            tz = timezone,
            isOffline = offline,
            searchQuery = search,
            allItems = all,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), TasksUiState())

    init { refresh() }

    fun setDateFilter(f: TaskDateFilter) {
        if (dateFilterFlow.value == f) return
        dateFilterFlow.value = f
        refresh()
    }

    fun setStatusFilter(f: TaskStatusFilter) {
        statusFilterFlow.value = f
    }

    fun clearError() { errorFlow.value = null }

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
                filter = dateFilterFlow.value.server,
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
        if (!container.isOnline()) {
            offlineFlow.value = true
            errorFlow.value = "当前无网络，离线状态下不能修改任务完成状态。请联网后再确认。"
            return
        }
        viewModelScope.launch {
            val r = if (completed) container.todoRepository.complete(id)
                    else container.todoRepository.uncomplete(id)
            if (r is Result.Error) {
                if (r.code == "network") offlineFlow.value = true
                errorFlow.value = r.message
            }
        }
    }

    fun delete(id: Long) {
        if (!container.isOnline()) {
            errorFlow.value = "当前无网络，离线状态下无法删除任务"
            return
        }
        viewModelScope.launch {
            val r = container.todoRepository.delete(id)
            if (r is Result.Error && r.code != "network") {
                errorFlow.value = r.message
            }
        }
    }

    class Factory(private val container: AppContainer) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = TasksViewModel(container) as T
    }
}
