package com.example.taskflow.ui.screens

import com.example.taskflow.data.local.TodoCacheEntity
import com.example.taskflow.util.DateTimeFmt
import java.time.Instant
import java.time.LocalDate

enum class TaskDateFilter(val label: String, val server: String?) {
    Today("今日", "today"),
    Tomorrow("明天", "tomorrow"),
    ThisWeek("本周", "this_week"),
    RecentWeek("近7天", "recent_week"),
    RecentMonth("近30天", "recent_month"),
    All("全部", "all"),
    Scheduled("有日期", "scheduled"),
    NoDate("无日期", "no_date"),
}

enum class TaskStatusFilter(val label: String) {
    All("全部"),
    Open("未完成"),
    Expired("已过期"),
    Done("已完成"),
}

fun filterTodosForAndroid(
    all: List<TodoCacheEntity>,
    dateFilter: TaskDateFilter,
    statusFilter: TaskStatusFilter,
    timezone: String,
    search: String,
    todayLocalDate: String = DateTimeFmt.nowLocalDate(timezone).toString(),
    nowIso: String = Instant.now().toString(),
): List<TodoCacheEntity> {
    val today = runCatching { LocalDate.parse(todayLocalDate) }.getOrElse { DateTimeFmt.nowLocalDate(timezone) }
    val now = runCatching { Instant.parse(nowIso) }.getOrElse { Instant.now() }
    val dateScoped = all.asSequence()
        .filter { passDateFilter(it, dateFilter, timezone, today) }
        .filter { passStatusFilter(it, statusFilter, now) }
        .filter { passSearch(it, search) }
        .sortedWith(compareBy<TodoCacheEntity> { it.due_at == null }.thenBy { it.due_at ?: "" }.thenBy { it.id })
        .toList()
    return dateScoped
}

private fun passDateFilter(todo: TodoCacheEntity, filter: TaskDateFilter, timezone: String, today: LocalDate): Boolean {
    val due = todo.due_at?.let { runCatching { LocalDate.parse(DateTimeFmt.localDate(it, timezone)) }.getOrNull() }
    return when (filter) {
        TaskDateFilter.Today -> due != null && due < today.plusDays(1) && (due >= today || !todo.is_completed)
        TaskDateFilter.Tomorrow -> due == today.plusDays(1) && !todo.is_completed
        TaskDateFilter.ThisWeek -> {
            val weekStart = today.minusDays(((today.dayOfWeek.value - 1).toLong()).coerceAtLeast(0))
            val weekEnd = weekStart.plusDays(7)
            due != null && due < weekEnd && (due >= weekStart || !todo.is_completed)
        }
        TaskDateFilter.RecentWeek -> due != null && due < today.plusDays(7) && (due >= today || !todo.is_completed)
        TaskDateFilter.RecentMonth -> due != null && due < today.plusDays(30) && (due >= today || !todo.is_completed)
        TaskDateFilter.All -> true
        TaskDateFilter.Scheduled -> due != null
        TaskDateFilter.NoDate -> due == null && !todo.is_completed
    }
}

private fun passStatusFilter(todo: TodoCacheEntity, filter: TaskStatusFilter, now: Instant): Boolean {
    val overdue = isOverdueAt(todo, now)
    return when (filter) {
        TaskStatusFilter.All -> true
        TaskStatusFilter.Open -> !todo.is_completed && !overdue
        TaskStatusFilter.Expired -> overdue
        TaskStatusFilter.Done -> todo.is_completed
    }
}

private fun isOverdueAt(todo: TodoCacheEntity, now: Instant): Boolean {
    if (todo.completed_at != null || todo.due_at.isNullOrBlank()) return false
    return runCatching { Instant.parse(todo.due_at).isBefore(now) }.getOrDefault(false)
}

private fun passSearch(todo: TodoCacheEntity, search: String): Boolean {
    val q = search.trim().lowercase()
    if (q.isBlank()) return true
    return todo.title.lowercase().contains(q) || todo.description.lowercase().contains(q)
}
