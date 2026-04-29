package com.example.todoalarm.data.repository

import com.example.todoalarm.data.auth.TokenManager
import com.example.todoalarm.data.local.AppDatabase
import com.example.todoalarm.data.local.TodoCacheEntity
import com.example.todoalarm.data.remote.ApiClient
import com.example.todoalarm.data.remote.TodoDto
import com.example.todoalarm.data.remote.TodoInput
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOf

class TodoRepository(
    private val client: ApiClient,
    private val db: AppDatabase,
    private val tokenManager: TokenManager,
) {
    /** 本地缓存(Room)的 Flow,用户离线也能看到。 */
    fun observeAll(): Flow<List<TodoCacheEntity>> {
        val uid = tokenManager.current().userId ?: return flowOf(emptyList())
        return db.todoDao().all(uid)
    }

    suspend fun refreshAll(filter: String? = null, listId: Long? = null, search: String? = null) : Result<List<TodoDto>> {
        val r = safeCall(client.moshi) {
            client.api.todosList(filter = filter, listId = listId, search = search,
                limit = 200, includeDone = filter == "completed" || filter == "all")
        }
        return when (r) {
            is Result.Success -> {
                val items = r.data.items.orEmpty()
                cacheUpsert(items)
                Result.Success(items)
            }
            is Result.Error -> r
        }
    }

    suspend fun create(input: TodoInput): Result<TodoDto> {
        val r = safeCall(client.moshi) { client.api.todoCreate(input) }
        if (r is Result.Success) cacheUpsert(listOf(r.data))
        return r
    }

    suspend fun update(id: Long, input: TodoInput): Result<TodoDto> {
        val r = safeCall(client.moshi) { client.api.todoUpdate(id, input) }
        if (r is Result.Success) cacheUpsert(listOf(r.data))
        return r
    }

    suspend fun delete(id: Long): Result<Unit> {
        val r = safeCall(client.moshi) { client.api.todoDelete(id) }
        if (r is Result.Success) db.todoDao().deleteById(id)
        return r
    }

    suspend fun complete(id: Long): Result<TodoDto> {
        val r = safeCall(client.moshi) { client.api.todoComplete(id) }
        if (r is Result.Success) cacheUpsert(listOf(r.data))
        return r
    }

    suspend fun uncomplete(id: Long): Result<TodoDto> {
        val r = safeCall(client.moshi) { client.api.todoUncomplete(id) }
        if (r is Result.Success) cacheUpsert(listOf(r.data))
        return r
    }

    private suspend fun cacheUpsert(items: List<TodoDto>) {
        val ents = items.map {
            TodoCacheEntity(
                id = it.id, user_id = it.user_id, list_id = it.list_id,
                title = it.title, description = it.description,
                priority = it.priority, effort = it.effort,
                due_at = it.due_at, due_all_day = it.due_all_day,
                is_completed = it.is_completed, completed_at = it.completed_at,
                sort_order = it.sort_order, timezone = it.timezone,
                created_at = it.created_at, updated_at = it.updated_at,
            )
        }
        db.todoDao().upsert(ents)
    }
}
