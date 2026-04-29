package com.example.todoalarm.data.remote

import com.squareup.moshi.JsonClass

// =============================================================
// Auth
// =============================================================

@JsonClass(generateAdapter = true)
data class RegisterRequest(
    val email: String,
    val password: String,
    val display_name: String? = null,
    val timezone: String? = null,
    val device_id: String? = null,
)

@JsonClass(generateAdapter = true)
data class LoginRequest(
    val email: String,
    val password: String,
    val device_id: String? = null,
)

@JsonClass(generateAdapter = true)
data class RefreshRequest(val refresh_token: String)

@JsonClass(generateAdapter = true)
data class LogoutRequest(
    val refresh_token: String? = null,
    val all_devices: Boolean = false,
)

@JsonClass(generateAdapter = true)
data class AuthResponse(
    val access_token: String,
    val access_token_expires_at: String,
    val refresh_token: String,
    val refresh_token_expires_at: String,
    val user: UserDto,
)

@JsonClass(generateAdapter = true)
data class UserDto(
    val id: Long,
    val email: String,
    val display_name: String,
    val timezone: String,
    val created_at: String,
    val updated_at: String,
)

// =============================================================
// Lists
// =============================================================

@JsonClass(generateAdapter = true)
data class ListDto(
    val id: Long,
    val user_id: Long,
    val name: String,
    val color: String = "",
    val icon: String = "",
    val sort_order: Int = 0,
    val is_default: Boolean = false,
    val is_archived: Boolean = false,
    val created_at: String,
    val updated_at: String,
)

@JsonClass(generateAdapter = true)
data class ListsResponse(val items: List<ListDto>?)

@JsonClass(generateAdapter = true)
data class ListInput(
    val name: String,
    val color: String? = null,
    val icon: String? = null,
    val sort_order: Int? = null,
)

// =============================================================
// Todos
// =============================================================

@JsonClass(generateAdapter = true)
data class TodoDto(
    val id: Long,
    val user_id: Long,
    val list_id: Long? = null,
    val title: String,
    val description: String = "",
    val priority: Int = 0,
    val effort: Int = 0,
    val due_at: String? = null,
    val due_all_day: Boolean = false,
    val start_at: String? = null,
    val is_completed: Boolean = false,
    val completed_at: String? = null,
    val sort_order: Int = 0,
    val timezone: String = "UTC",
    val created_at: String,
    val updated_at: String,
)

@JsonClass(generateAdapter = true)
data class TodosResponse(val items: List<TodoDto>?)

@JsonClass(generateAdapter = true)
data class TodoInput(
    val title: String,
    val description: String = "",
    val priority: Int = 0,
    val effort: Int = 0,
    val list_id: Long? = null,
    val due_at: String? = null,
    val due_all_day: Boolean = false,
    val start_at: String? = null,
    val sort_order: Int = 0,
    val timezone: String? = null,
)

// =============================================================
// Subtasks
// =============================================================

@JsonClass(generateAdapter = true)
data class SubtaskDto(
    val id: Long,
    val user_id: Long,
    val todo_id: Long,
    val title: String,
    val is_completed: Boolean = false,
    val completed_at: String? = null,
    val sort_order: Int = 0,
    val created_at: String,
    val updated_at: String,
)

@JsonClass(generateAdapter = true)
data class SubtasksResponse(val items: List<SubtaskDto>?)

@JsonClass(generateAdapter = true)
data class SubtaskInput(val title: String, val sort_order: Int? = null)

// =============================================================
// Reminders
// =============================================================

@JsonClass(generateAdapter = true)
data class ReminderDto(
    val id: Long,
    val user_id: Long,
    val todo_id: Long? = null,
    val title: String = "",
    val trigger_at: String? = null,
    val rrule: String = "",
    val dtstart: String? = null,
    val timezone: String = "UTC",
    val channel_local: Boolean = true,
    val channel_telegram: Boolean = false,
    val channel_web_push: Boolean = false,
    val is_enabled: Boolean = true,
    val next_fire_at: String? = null,
    val last_fired_at: String? = null,
    val ringtone: String = "default",
    val vibrate: Boolean = true,
    val fullscreen: Boolean = true,
    val created_at: String,
    val updated_at: String,
)

@JsonClass(generateAdapter = true)
data class RemindersResponse(val items: List<ReminderDto>?)

@JsonClass(generateAdapter = true)
data class ReminderInput(
    val todo_id: Long? = null,
    val title: String = "",
    val trigger_at: String? = null,
    val rrule: String = "",
    val dtstart: String? = null,
    val timezone: String = "UTC",
    val channel_local: Boolean = true,
    val channel_telegram: Boolean = false,
    val ringtone: String = "default",
    val vibrate: Boolean = true,
    val fullscreen: Boolean = true,
)

// =============================================================
// Notifications
// =============================================================

@JsonClass(generateAdapter = true)
data class NotificationDto(
    val id: Long,
    val user_id: Long,
    val reminder_rule_id: Long? = null,
    val todo_id: Long? = null,
    val title: String,
    val body: String = "",
    val fire_at: String,
    val is_read: Boolean = false,
    val created_at: String,
)

@JsonClass(generateAdapter = true)
data class NotificationsResponse(val items: List<NotificationDto>?, val unread_count: Int = 0)

@JsonClass(generateAdapter = true)
data class UnreadCountResponse(val count: Int)

// =============================================================
// Telegram
// =============================================================

@JsonClass(generateAdapter = true)
data class TelegramBindToken(
    val token: String,
    val expires_at: String,
    val bot_username: String,
    val deep_link_web: String,
    val deep_link_app: String,
)

@JsonClass(generateAdapter = true)
data class TelegramBinding(
    val id: Long,
    val user_id: Long,
    val chat_id: String,
    val username: String,
    val is_enabled: Boolean,
    val created_at: String,
)

@JsonClass(generateAdapter = true)
data class TelegramBindingsResponse(val items: List<TelegramBinding>?)

@JsonClass(generateAdapter = true)
data class TelegramBindStatus(val status: String, val binding: TelegramBinding? = null)

@JsonClass(generateAdapter = true)
data class TelegramUnbindRequest(val id: Long)

@JsonClass(generateAdapter = true)
data class TelegramTestRequest(val binding_id: Long)

// =============================================================
// Sync
// =============================================================

@JsonClass(generateAdapter = true)
data class SyncEvent(
    val id: Long,
    val entity_type: String,
    val entity_id: Long,
    val action: String,
    val created_at: String,
)

@JsonClass(generateAdapter = true)
data class SyncPullResponse(
    val events: List<SyncEvent>?,
    val next_cursor: Long,
    val has_more: Boolean,
)

@JsonClass(generateAdapter = true)
data class CursorResponse(val cursor: Long)

// =============================================================
// Pomodoro
// =============================================================

@JsonClass(generateAdapter = true)
data class PomodoroSessionDto(
    val id: Long,
    val user_id: Long,
    val todo_id: Long? = null,
    val started_at: String,
    val ended_at: String? = null,
    val planned_duration_seconds: Int,
    val actual_duration_seconds: Int = 0,
    val kind: String = "focus",
    val status: String = "active",
    val note: String = "",
    val created_at: String,
    val updated_at: String,
)

@JsonClass(generateAdapter = true)
data class PomodoroListResponse(val items: List<PomodoroSessionDto>?)

@JsonClass(generateAdapter = true)
data class PomodoroStartRequest(
    val todo_id: Long? = null,
    val planned_duration_seconds: Int,
    val kind: String = "focus",
    val note: String = "",
)

// =============================================================
// Stats
// =============================================================

@JsonClass(generateAdapter = true)
data class StatsSummaryDto(
    val todos_total: Int = 0,
    val todos_open: Int = 0,
    val todos_completed: Int = 0,
    val todos_overdue: Int = 0,
    val todos_due_today: Int = 0,
    val completed_today: Int = 0,
    val completed_this_week: Int = 0,
    val pomodoro_today_seconds: Int = 0,
    val pomodoro_this_week_seconds: Int = 0,
)

// =============================================================
// Generic API error envelope
// =============================================================

@JsonClass(generateAdapter = true)
data class ApiErrorBody(val error: ApiErrorDetail)

@JsonClass(generateAdapter = true)
data class ApiErrorDetail(val code: String, val message: String)
