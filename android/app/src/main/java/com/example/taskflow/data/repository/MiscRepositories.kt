package com.example.taskflow.data.repository

import com.example.taskflow.data.remote.ApiClient
import com.example.taskflow.data.remote.NotificationDto
import com.example.taskflow.data.remote.PomodoroSessionDto
import com.example.taskflow.data.remote.PomodoroStartRequest
import com.example.taskflow.data.remote.StatsSummaryDto
import com.example.taskflow.data.remote.TelegramBindStatus
import com.example.taskflow.data.remote.TelegramBindToken
import com.example.taskflow.data.remote.TelegramBinding
import com.example.taskflow.data.remote.TelegramTestRequest
import com.example.taskflow.data.remote.TelegramUnbindRequest

class NotificationRepository(private val client: ApiClient) {
    suspend fun list(onlyUnread: Boolean = false, limit: Int = 50): Result<Pair<List<NotificationDto>, Int>> {
        val r = safeCall(client.moshi) {
            client.api.notifications(onlyUnread = onlyUnread.takeIf { it }, limit = limit)
        }
        return r.map { (it.items.orEmpty()) to it.unread_count }
    }

    suspend fun unreadCount(): Result<Int> = safeCall(client.moshi) { client.api.unreadCount() }
        .map { it.count }

    suspend fun markRead(id: Long): Result<Unit> = safeCall(client.moshi) { client.api.notificationMarkRead(id) }
    suspend fun markAllRead(): Result<Unit> = safeCall(client.moshi) { client.api.notificationsMarkAllRead() }
}

class TelegramRepository(private val client: ApiClient) {
    suspend fun createBindToken(): Result<TelegramBindToken> =
        safeCall(client.moshi) { client.api.telegramBindToken() }

    suspend fun bindStatus(token: String): Result<TelegramBindStatus> =
        safeCall(client.moshi) { client.api.telegramBindStatus(token) }

    suspend fun bindings(): Result<List<TelegramBinding>> =
        safeCall(client.moshi) { client.api.telegramBindings() }.map { it.items.orEmpty() }

    suspend fun unbind(id: Long): Result<Unit> =
        safeCall(client.moshi) { client.api.telegramUnbind(TelegramUnbindRequest(id)) }

    suspend fun sendTest(bindingId: Long): Result<Unit> =
        safeCall(client.moshi) { client.api.telegramTest(TelegramTestRequest(bindingId)) }
}

class StatsRepository(private val client: ApiClient) {
    suspend fun summary(): Result<StatsSummaryDto> = safeCall(client.moshi) { client.api.statsSummary() }
}

class PomodoroRepository(private val client: ApiClient) {
    suspend fun list(limit: Int = 50): Result<List<PomodoroSessionDto>> =
        safeCall(client.moshi) { client.api.pomodoroList(limit) }.map { it.items.orEmpty() }

    suspend fun start(plannedSeconds: Int, kind: String, todoId: Long?, note: String): Result<PomodoroSessionDto> =
        safeCall(client.moshi) {
            client.api.pomodoroCreate(PomodoroStartRequest(
                todo_id = todoId, planned_duration_seconds = plannedSeconds,
                kind = kind, note = note,
            ))
        }

    suspend fun complete(id: Long): Result<PomodoroSessionDto> =
        safeCall(client.moshi) { client.api.pomodoroComplete(id) }

    suspend fun abandon(id: Long): Result<PomodoroSessionDto> =
        safeCall(client.moshi) { client.api.pomodoroAbandon(id) }
}
