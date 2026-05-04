package com.example.taskflow.data.local

import androidx.room.ColumnInfo
import androidx.room.Dao
import androidx.room.Database
import androidx.room.Entity
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.RoomDatabase
import androidx.room.TypeConverter
import androidx.room.TypeConverters
import kotlinx.coroutines.flow.Flow

// =============================================================
// Cached entities
//
// 设计原则(规格 §4):
//   - 这里只缓存"已同步到本地的状态",绝不充当离线写队列。
//   - todos / lists / subtasks 这些纯展示型数据缓存,断网时进入只读模式。
//   - reminder_rules_cache 是关键:Android 离线必须能本地触发提醒,
//     AlarmManager 注册依赖此表。
// =============================================================

@Entity(tableName = "todos_cache")
data class TodoCacheEntity(
    @PrimaryKey val id: Long,
    val user_id: Long,
    val list_id: Long?,
    val title: String,
    val description: String,
    val priority: Int,
    val effort: Int,
    val duration_minutes: Int,
    val start_at: String?,
    val due_at: String?,
    val due_all_day: Boolean,
    val is_completed: Boolean,
    val completed_at: String?,
    val sort_order: Int,
    val timezone: String,
    val created_at: String,
    val updated_at: String,
)

@Entity(tableName = "lists_cache")
data class ListCacheEntity(
    @PrimaryKey val id: Long,
    val user_id: Long,
    val name: String,
    val color: String,
    val icon: String,
    val sort_order: Int,
    val is_default: Boolean,
    val is_archived: Boolean,
    val created_at: String,
    val updated_at: String,
)

@Entity(tableName = "subtasks_cache")
data class SubtaskCacheEntity(
    @PrimaryKey val id: Long,
    val user_id: Long,
    val todo_id: Long,
    val title: String,
    val is_completed: Boolean,
    val sort_order: Int,
    val created_at: String,
    val updated_at: String,
)

@Entity(tableName = "reminders_cache")
data class ReminderCacheEntity(
    @PrimaryKey val id: Long,
    val user_id: Long,
    val todo_id: Long?,
    val title: String,
    val trigger_at: String?,
    val rrule: String,
    val dtstart: String?,
    val timezone: String,
    val channel_local: Boolean,
    val channel_telegram: Boolean,
    val is_enabled: Boolean,
    val next_fire_at: String?,
    val last_fired_at: String?,
    val ringtone: String,
    val vibrate: Boolean,
    val fullscreen: Boolean,
    /** 用于本地调度状态:已注册到 AlarmManager 的目标时间(RFC3339 UTC) */
    val scheduled_for: String?,
    val created_at: String,
    val updated_at: String,
)

@Entity(tableName = "sync_meta")
data class SyncMetaEntity(
    @PrimaryKey val user_id: Long,
    @ColumnInfo(name = "cursor") val cursor: Long,
    val updated_at: String,
)

@Entity(tableName = "local_alarm_log", primaryKeys = ["rule_id", "fire_at"])
data class LocalAlarmLogEntity(
    val rule_id: Long,
    val fire_at: String,    // RFC3339 UTC
    val fired_at: String,
    val acked_at: String?,
)

// =============================================================
// DAOs
// =============================================================

@Dao
interface TodoDao {
    @Query("SELECT * FROM todos_cache WHERE user_id = :userId ORDER BY start_at IS NULL, start_at ASC, id ASC")
    fun all(userId: Long): Flow<List<TodoCacheEntity>>

    @Query("SELECT * FROM todos_cache WHERE id = :id")
    suspend fun byId(id: Long): TodoCacheEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(items: List<TodoCacheEntity>)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertOne(item: TodoCacheEntity)

    @Query("DELETE FROM todos_cache WHERE id = :id")
    suspend fun deleteById(id: Long)

    @Query("DELETE FROM todos_cache WHERE user_id = :userId")
    suspend fun clearForUser(userId: Long)
}

@Dao
interface ListDao {
    @Query("SELECT * FROM lists_cache WHERE user_id = :userId ORDER BY sort_order, id")
    fun all(userId: Long): Flow<List<ListCacheEntity>>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(items: List<ListCacheEntity>)

    @Query("DELETE FROM lists_cache WHERE id = :id")
    suspend fun deleteById(id: Long)

    @Query("DELETE FROM lists_cache WHERE user_id = :userId")
    suspend fun clearForUser(userId: Long)
}

@Dao
interface SubtaskDao {
    @Query("SELECT * FROM subtasks_cache WHERE todo_id = :todoId ORDER BY sort_order, id")
    fun byTodo(todoId: Long): Flow<List<SubtaskCacheEntity>>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(items: List<SubtaskCacheEntity>)

    @Query("DELETE FROM subtasks_cache WHERE id = :id")
    suspend fun deleteById(id: Long)

    @Query("DELETE FROM subtasks_cache WHERE user_id = :userId")
    suspend fun clearForUser(userId: Long)
}

@Dao
interface ReminderDao {
    @Query("SELECT * FROM reminders_cache WHERE user_id = :userId AND is_enabled = 1 ORDER BY next_fire_at IS NULL, next_fire_at ASC")
    fun activeForUser(userId: Long): Flow<List<ReminderCacheEntity>>

    @Query("SELECT * FROM reminders_cache WHERE user_id = :userId AND is_enabled = 1 AND channel_local = 1 AND next_fire_at IS NOT NULL")
    suspend fun localScheduled(userId: Long): List<ReminderCacheEntity>

    @Query("SELECT * FROM reminders_cache WHERE id = :id")
    suspend fun byId(id: Long): ReminderCacheEntity?

    @Query("SELECT * FROM reminders_cache WHERE todo_id = :todoId")
    suspend fun byTodo(todoId: Long): List<ReminderCacheEntity>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertOne(rule: ReminderCacheEntity)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(items: List<ReminderCacheEntity>)

    @Query("UPDATE reminders_cache SET scheduled_for = :scheduledFor WHERE id = :id")
    suspend fun setScheduledFor(id: Long, scheduledFor: String?)

    @Query("DELETE FROM reminders_cache WHERE id = :id")
    suspend fun deleteById(id: Long)

    @Query("DELETE FROM reminders_cache WHERE user_id = :userId")
    suspend fun clearForUser(userId: Long)
}

@Dao
interface SyncMetaDao {
    @Query("SELECT cursor FROM sync_meta WHERE user_id = :userId")
    suspend fun getCursor(userId: Long): Long?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(meta: SyncMetaEntity)
}

@Dao
interface AlarmLogDao {
    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun logFire(entry: LocalAlarmLogEntity)

    @Query("UPDATE local_alarm_log SET acked_at = :ackedAt WHERE rule_id = :ruleId AND fire_at = :fireAt")
    suspend fun ack(ruleId: Long, fireAt: String, ackedAt: String)

    @Query("SELECT EXISTS(SELECT 1 FROM local_alarm_log WHERE rule_id = :ruleId AND fire_at = :fireAt)")
    suspend fun has(ruleId: Long, fireAt: String): Boolean
}

// =============================================================
// Database
// =============================================================

@Database(
    entities = [
        TodoCacheEntity::class,
        ListCacheEntity::class,
        SubtaskCacheEntity::class,
        ReminderCacheEntity::class,
        SyncMetaEntity::class,
        LocalAlarmLogEntity::class,
    ],
    version = 3,
    exportSchema = false,
)
@TypeConverters(BoolConverters::class)
abstract class AppDatabase : RoomDatabase() {
    abstract fun todoDao(): TodoDao
    abstract fun listDao(): ListDao
    abstract fun subtaskDao(): SubtaskDao
    abstract fun reminderDao(): ReminderDao
    abstract fun syncMetaDao(): SyncMetaDao
    abstract fun alarmLogDao(): AlarmLogDao
}

class BoolConverters {
    @TypeConverter fun fromBool(v: Boolean): Int = if (v) 1 else 0
    @TypeConverter fun toBool(v: Int): Boolean = v != 0
}
