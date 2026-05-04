package com.example.taskflow.ui.screens

import com.example.taskflow.data.local.TodoCacheEntity
import kotlin.test.Test
import kotlin.test.assertEquals

class TaskFiltersTest {
    private val tz = "Asia/Shanghai"
    private val today = "2026-05-04"

    @Test
    fun todayScopeUsesStartAtAndKeepsPastIncompleteVisible() {
        val items = listOf(
            todo(1, "今日未完成", startAt = "2026-05-04T02:00:00Z"),
            todo(2, "昨日未完成", startAt = "2026-05-03T02:00:00Z"),
            todo(3, "明日未完成", startAt = "2026-05-05T02:00:00Z"),
            todo(4, "昨日已完成", startAt = "2026-05-03T02:00:00Z", completed = true, completedAt = "2026-05-03T08:00:00Z"),
        )

        val filtered = filterTodosForAndroid(
            all = items,
            dateFilter = TaskDateFilter.Today,
            statusFilter = TaskStatusFilter.All,
            timezone = tz,
            search = "",
            todayLocalDate = today,
            nowIso = "2026-05-04T04:00:00Z",
        )

        assertEquals(listOf(2L, 1L), filtered.map { it.id })
    }

    @Test
    fun completedPastTaskMovesIntoDoneForTheDayItWasCompleted() {
        val items = listOf(
            todo(1, "昨日过期今天完成", startAt = "2026-05-03T02:00:00Z", completed = true, completedAt = "2026-05-04T08:00:00Z"),
            todo(2, "昨日已经完成", startAt = "2026-05-03T02:00:00Z", completed = true, completedAt = "2026-05-03T08:00:00Z"),
            todo(3, "今日完成", startAt = "2026-05-04T02:00:00Z", completed = true, completedAt = "2026-05-04T09:00:00Z"),
        )

        val done = filterTodosForAndroid(
            all = items,
            dateFilter = TaskDateFilter.Today,
            statusFilter = TaskStatusFilter.Done,
            timezone = tz,
            search = "",
            todayLocalDate = today,
            nowIso = "2026-05-04T10:00:00Z",
        )

        assertEquals(listOf(1L, 3L), done.map { it.id })
    }

    @Test
    fun statusOpenAndExpiredAreMutuallyExclusive() {
        val items = listOf(
            todo(1, "还来得及", startAt = "2026-05-04T12:00:00Z"),
            todo(2, "已超时", startAt = "2026-05-03T02:00:00Z"),
            todo(3, "已完成", startAt = "2026-05-04T02:00:00Z", completed = true),
        )

        val open = filterTodosForAndroid(items, TaskDateFilter.Today, TaskStatusFilter.Open, tz, "", today, "2026-05-04T04:00:00Z")
        val expired = filterTodosForAndroid(items, TaskDateFilter.Today, TaskStatusFilter.Expired, tz, "", today, "2026-05-04T04:00:00Z")
        val done = filterTodosForAndroid(items, TaskDateFilter.Today, TaskStatusFilter.Done, tz, "", today, "2026-05-04T04:00:00Z")

        assertEquals(listOf(1L), open.map { it.id })
        assertEquals(listOf(2L), expired.map { it.id })
        assertEquals(listOf(3L), done.map { it.id })
    }

    @Test
    fun scheduledAndNoDateScopesAreExclusive() {
        val items = listOf(
            todo(1, "有日期", startAt = "2026-05-04T02:00:00Z"),
            todo(2, "无日期", startAt = null),
            todo(3, "已完成无日期", startAt = null, completed = true),
        )

        val scheduled = filterTodosForAndroid(items, TaskDateFilter.Scheduled, TaskStatusFilter.All, tz, "", today, "2026-05-04T04:00:00Z")
        val noDate = filterTodosForAndroid(items, TaskDateFilter.NoDate, TaskStatusFilter.All, tz, "", today, "2026-05-04T04:00:00Z")
        val noDateDone = filterTodosForAndroid(items, TaskDateFilter.NoDate, TaskStatusFilter.Done, tz, "", today, "2026-05-04T04:00:00Z")

        assertEquals(listOf(1L), scheduled.map { it.id })
        assertEquals(listOf(2L, 3L), noDate.map { it.id })
        assertEquals(listOf(3L), noDateDone.map { it.id })
    }

    @Test
    fun startAtHasPriorityOverLegacyDueAtWhenFiltering() {
        val item = todo(
            id = 1,
            title = "开始时间在今天",
            startAt = "2026-05-04T02:00:00Z",
            dueAt = "2026-05-05T02:00:00Z",
        )

        val todayItems = filterTodosForAndroid(listOf(item), TaskDateFilter.Today, TaskStatusFilter.All, tz, "", today, "2026-05-04T04:00:00Z")
        val tomorrowItems = filterTodosForAndroid(listOf(item), TaskDateFilter.Tomorrow, TaskStatusFilter.All, tz, "", today, "2026-05-04T04:00:00Z")

        assertEquals(listOf(1L), todayItems.map { it.id })
        assertEquals(emptyList(), tomorrowItems.map { it.id })
    }

    @Test
    fun priorityLabelsMatchWeb() {
        assertEquals(listOf("无", "低", "中", "高", "紧急"), (0..4).map { taskPriorityLabel(it) })
    }

    private fun todo(
        id: Long,
        title: String,
        startAt: String?,
        dueAt: String? = null,
        completed: Boolean = false,
        completedAt: String? = if (completed) "2026-05-04T08:00:00Z" else null,
    ) = TodoCacheEntity(
        id = id,
        user_id = 1,
        list_id = null,
        title = title,
        description = "",
        priority = 0,
        effort = 0,
        duration_minutes = 0,
        start_at = startAt,
        due_at = dueAt,
        due_all_day = false,
        is_completed = completed,
        completed_at = completedAt,
        sort_order = 0,
        timezone = tz,
        created_at = "2026-05-01T00:00:00Z",
        updated_at = "2026-05-01T00:00:00Z",
    )
}
