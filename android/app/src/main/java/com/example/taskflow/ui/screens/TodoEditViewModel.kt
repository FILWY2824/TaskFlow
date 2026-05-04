package com.example.taskflow.ui.screens

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.example.taskflow.AppContainer
import com.example.taskflow.data.local.ReminderCacheEntity
import com.example.taskflow.data.local.SubtaskCacheEntity
import com.example.taskflow.data.local.TodoCacheEntity
import com.example.taskflow.data.remote.ReminderInput
import com.example.taskflow.data.remote.TodoInput
import com.example.taskflow.data.repository.Result
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

data class TodoEditUiState(
    val loading: Boolean = false,
    val saving: Boolean = false,
    val deleting: Boolean = false,
    val error: String? = null,
    val saved: Boolean = false,
    val deleted: Boolean = false,

    val title: String = "",
    val description: String = "",
    val priority: Int = 0,
    val durationMinutes: Int = 30,
    val dueAtIso: String? = null,
    val dueAllDay: Boolean = false,
    val timezone: String = "Asia/Shanghai",

    val todoId: Long? = null,
    val subtasks: List<SubtaskCacheEntity> = emptyList(),
    val reminders: List<ReminderCacheEntity> = emptyList(),
)

class TodoEditViewModel(
    private val container: AppContainer,
    private val initialTodoId: Long?,
) : ViewModel() {

    private val _state = MutableStateFlow(
        TodoEditUiState(
            todoId = initialTodoId,
            timezone = container.tokenManager.current().timezone,
        )
    )
    val state: StateFlow<TodoEditUiState> = _state.asStateFlow()

    init {
        if (initialTodoId != null) load(initialTodoId)
    }

    private fun load(id: Long) {
        _state.value = _state.value.copy(loading = true)
        viewModelScope.launch {
            val cached: TodoCacheEntity? = container.db.todoDao().byId(id)
            if (cached != null) {
                _state.value = _state.value.copy(
                    title = cached.title,
                    description = cached.description,
                    priority = cached.priority,
                    durationMinutes = cached.duration_minutes,
                    dueAtIso = cached.due_at,
                    dueAllDay = cached.due_all_day,
                    timezone = cached.timezone.ifBlank { _state.value.timezone },
                )
            }
            // 触发一次刷新,让本地 Room 拿到最新子任务
            container.subtaskRepository.refresh(id)
            val rems = container.db.reminderDao().byTodo(id)
            _state.value = _state.value.copy(loading = false, reminders = rems)
        }
        // subtasks 走 Flow,Room upsert 后会自动推到 UI;reminders 由本地修改后手动刷新
        viewModelScope.launch {
            container.subtaskRepository.observeFor(id).collect { subs ->
                _state.value = _state.value.copy(subtasks = subs)
            }
        }
    }

    fun setTitle(v: String) { _state.value = _state.value.copy(title = v, error = null) }
    fun clearError() { _state.value = _state.value.copy(error = null) }
    fun setDescription(v: String) { _state.value = _state.value.copy(description = v) }
    fun setPriority(v: Int) { _state.value = _state.value.copy(priority = v) }
    fun setDurationMinutes(v: Int) {
        _state.value = _state.value.copy(durationMinutes = v.coerceIn(0, 1440))
    }
    fun setDueAt(iso: String?, allDay: Boolean) { _state.value = _state.value.copy(dueAtIso = iso, dueAllDay = allDay) }

    fun save() {
        val s = _state.value
        if (s.title.isBlank()) {
            _state.value = s.copy(error = "标题不能为空")
            return
        }
        if (!container.isOnline()) {
            _state.value = s.copy(error = "当前无网络，离线状态下无法保存任务")
            return
        }
        _state.value = s.copy(saving = true, error = null)
        val input = TodoInput(
            title = s.title.trim(), description = s.description, priority = s.priority,
            duration_minutes = s.durationMinutes.coerceIn(0, 1440),
            due_at = s.dueAtIso, due_all_day = s.dueAllDay,
            timezone = s.timezone,
        )
        viewModelScope.launch {
            val r = if (s.todoId == null) container.todoRepository.create(input)
                    else container.todoRepository.update(s.todoId, input)
            _state.value = when (r) {
                is Result.Success -> _state.value.copy(saving = false, saved = true, todoId = r.data.id)
                is Result.Error -> _state.value.copy(saving = false, error = r.message)
            }
        }
    }

    fun delete() {
        val id = _state.value.todoId ?: return
        if (!container.isOnline()) {
            _state.value = _state.value.copy(error = "当前无网络，离线状态下无法删除任务")
            return
        }
        _state.value = _state.value.copy(deleting = true, error = null)
        viewModelScope.launch {
            val r = container.todoRepository.delete(id)
            _state.value = when (r) {
                is Result.Success -> _state.value.copy(deleting = false, deleted = true)
                is Result.Error -> _state.value.copy(deleting = false, error = r.message)
            }
        }
    }

    fun addSubtask(title: String) {
        val id = _state.value.todoId ?: return
        if (title.isBlank()) return
        viewModelScope.launch {
            val r = container.subtaskRepository.create(id, title.trim())
            if (r is Result.Error) _state.value = _state.value.copy(error = r.message)
        }
    }

    fun toggleSubtask(s: SubtaskCacheEntity) {
        viewModelScope.launch {
            val r = container.subtaskRepository.toggle(s)
            if (r is Result.Error) _state.value = _state.value.copy(error = r.message)
        }
    }

    fun deleteSubtask(id: Long) {
        viewModelScope.launch {
            val r = container.subtaskRepository.delete(id)
            if (r is Result.Error) _state.value = _state.value.copy(error = r.message)
        }
    }

    /** 添加单次提醒 */
    fun addReminder(triggerAtIso: String, title: String) {
        val id = _state.value.todoId ?: return
        viewModelScope.launch {
            val r = container.reminderRepository.create(ReminderInput(
                todo_id = id, title = title, trigger_at = triggerAtIso,
                timezone = _state.value.timezone,
            ))
            if (r is Result.Success) {
                _state.value = _state.value.copy(reminders = container.db.reminderDao().byTodo(id))
            } else if (r is Result.Error) {
                _state.value = _state.value.copy(error = r.message)
            }
        }
    }

    fun deleteReminder(rid: Long) {
        viewModelScope.launch {
            val r = container.reminderRepository.delete(rid)
            val tid = _state.value.todoId
            if (tid != null) {
                _state.value = _state.value.copy(reminders = container.db.reminderDao().byTodo(tid))
            }
            if (r is Result.Error) _state.value = _state.value.copy(error = r.message)
        }
    }

    class Factory(private val container: AppContainer, private val todoId: Long?) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = TodoEditViewModel(container, todoId) as T
    }
}
