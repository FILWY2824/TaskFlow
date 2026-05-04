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
        .sortedWith(compareBy<TodoCacheEntity> { taskStartAt(it) == null }.thenBy { taskStartAt(it) ?: "" }.thenBy { it.id })
        .toList()
    return dateScoped
}

private fun passDateFilter(todo: TodoCacheEntity, filter: TaskDateFilter, timezone: String, today: LocalDate): Boolean {
    val start = taskStartLocalDate(todo, timezone)
    return when (filter) {
        TaskDateFilter.Today -> passCompoundDate(todo, start, today, today.plusDays(1), timezone)
        TaskDateFilter.Tomorrow -> start == today.plusDays(1)
        TaskDateFilter.ThisWeek -> {
            val weekStart = today.minusDays(((today.dayOfWeek.value - 1).toLong()).coerceAtLeast(0))
            val weekEnd = weekStart.plusDays(7)
            passCompoundDate(todo, start, weekStart, weekEnd, timezone)
        }
        TaskDateFilter.RecentWeek -> passCompoundDate(todo, start, today, today.plusDays(7), timezone)
        TaskDateFilter.RecentMonth -> passCompoundDate(todo, start, today, today.plusDays(30), timezone)
        TaskDateFilter.All -> true
        TaskDateFilter.Scheduled -> start != null
        TaskDateFilter.NoDate -> start == null
    }
}

private fun passCompoundDate(
    todo: TodoCacheEntity,
    start: LocalDate?,
    from: LocalDate,
    toExclusive: LocalDate,
    timezone: String,
): Boolean {
    if (start == null) return false
    if (start >= from && start < toExclusive) return true
    if (start < from && !todo.is_completed) return true
    val completed = completedLocalDate(todo, timezone)
    return start < from && completed != null && completed >= from && completed < toExclusive
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
    val start = taskStartAt(todo)
    if (todo.completed_at != null || start.isNullOrBlank()) return false
    return runCatching { Instant.parse(start).isBefore(now) }.getOrDefault(false)
}

private fun passSearch(todo: TodoCacheEntity, search: String): Boolean {
    val q = search.trim().lowercase()
    if (q.isBlank()) return true
    return todo.title.lowercase().contains(q) || todo.description.lowercase().contains(q)
}

fun taskStartAt(todo: TodoCacheEntity): String? = todo.start_at ?: todo.due_at

private fun taskStartLocalDate(todo: TodoCacheEntity, timezone: String): LocalDate? =
    taskStartAt(todo)?.let { runCatching { LocalDate.parse(DateTimeFmt.localDate(it, timezone)) }.getOrNull() }

private fun completedLocalDate(todo: TodoCacheEntity, timezone: String): LocalDate? =
    todo.completed_at?.let { runCatching { LocalDate.parse(DateTimeFmt.localDate(it, timezone)) }.getOrNull() }

fun taskPriorityLabel(priority: Int): String = when (priority.coerceIn(0, 4)) {
    1 -> "低"
    2 -> "中"
    3 -> "高"
    4 -> "紧急"
    else -> "无"
}
