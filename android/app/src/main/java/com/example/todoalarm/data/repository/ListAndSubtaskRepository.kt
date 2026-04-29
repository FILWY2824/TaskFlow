package com.example.todoalarm.data.repository

import com.example.todoalarm.data.auth.TokenManager
import com.example.todoalarm.data.local.AppDatabase
import com.example.todoalarm.data.local.ListCacheEntity
import com.example.todoalarm.data.local.SubtaskCacheEntity
import com.example.todoalarm.data.remote.ApiClient
import com.example.todoalarm.data.remote.ListDto
import com.example.todoalarm.data.remote.ListInput
import com.example.todoalarm.data.remote.SubtaskDto
import com.example.todoalarm.data.remote.SubtaskInput
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOf

class ListRepository(
    private val client: ApiClient,
    private val db: AppDatabase,
    private val tokenManager: TokenManager,
) {
    fun observeAll(): Flow<List<ListCacheEntity>> {
        val uid = tokenManager.current().userId ?: return flowOf(emptyList())
        return db.listDao().all(uid)
    }

    suspend fun refreshAll(): Result<List<ListDto>> {
        val r = safeCall(client.moshi) { client.api.listsAll() }
        if (r is Result.Success) {
            val items = r.data.items.orEmpty()
            db.listDao().upsert(items.map { it.toEntity() })
            return Result.Success(items)
        }
        return r as Result.Error
    }

    suspend fun create(name: String, color: String?): Result<ListDto> {
        val r = safeCall(client.moshi) { client.api.listCreate(ListInput(name = name, color = color)) }
        if (r is Result.Success) db.listDao().upsert(listOf(r.data.toEntity()))
        return r
    }

    suspend fun update(id: Long, input: ListInput): Result<ListDto> {
        val r = safeCall(client.moshi) { client.api.listUpdate(id, input) }
        if (r is Result.Success) db.listDao().upsert(listOf(r.data.toEntity()))
        return r
    }

    suspend fun delete(id: Long): Result<Unit> {
        val r = safeCall(client.moshi) { client.api.listDelete(id) }
        if (r is Result.Success) db.listDao().deleteById(id)
        return r
    }

    private fun ListDto.toEntity() = ListCacheEntity(
        id = id, user_id = user_id, name = name, color = color, icon = icon,
        sort_order = sort_order, is_default = is_default, is_archived = is_archived,
        created_at = created_at, updated_at = updated_at,
    )
}

class SubtaskRepository(
    private val client: ApiClient,
    private val db: AppDatabase,
) {
    fun observeFor(todoId: Long): Flow<List<SubtaskCacheEntity>> = db.subtaskDao().byTodo(todoId)

    suspend fun refresh(todoId: Long): Result<List<SubtaskDto>> {
        val r = safeCall(client.moshi) { client.api.subtasksByTodo(todoId) }
        if (r is Result.Success) db.subtaskDao().upsert(r.data.items.orEmpty().map { it.toEntity() })
        return r.map { it.items.orEmpty() }
    }

    suspend fun create(todoId: Long, title: String): Result<SubtaskDto> {
        val r = safeCall(client.moshi) { client.api.subtaskCreate(todoId, SubtaskInput(title)) }
        if (r is Result.Success) db.subtaskDao().upsert(listOf(r.data.toEntity()))
        return r
    }

    suspend fun toggle(s: SubtaskCacheEntity): Result<SubtaskDto> {
        val r = if (s.is_completed) safeCall(client.moshi) { client.api.subtaskUncomplete(s.id) }
        else safeCall(client.moshi) { client.api.subtaskComplete(s.id) }
        if (r is Result.Success) db.subtaskDao().upsert(listOf(r.data.toEntity()))
        return r
    }

    suspend fun delete(id: Long): Result<Unit> {
        val r = safeCall(client.moshi) { client.api.subtaskDelete(id) }
        if (r is Result.Success) db.subtaskDao().deleteById(id)
        return r
    }

    private fun SubtaskDto.toEntity() = SubtaskCacheEntity(
        id = id, user_id = user_id, todo_id = todo_id, title = title,
        is_completed = is_completed, sort_order = sort_order,
        created_at = created_at, updated_at = updated_at,
    )
}
