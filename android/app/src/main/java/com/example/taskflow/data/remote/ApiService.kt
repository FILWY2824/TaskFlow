package com.example.taskflow.data.remote

import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.PUT
import retrofit2.http.Path
import retrofit2.http.Query

/**
 * 把 Go 后端 (server/internal/handlers) 暴露的全部 REST 端点映射成 Retrofit 接口。
 *
 * 约定:
 *   - 所有需要认证的端点都会被 OkHttp AuthInterceptor 自动加 Authorization header,
 *     接口签名里不需要再写 @Header。
 *   - 所有方法都返回 Response<T>,让 Repository 层决定如何映射 4xx / 5xx。
 *   - "noAuth" 指标(注册 / 登录 / 刷新 / 健康检查)在调用方会用 .skipAuth() 标记;
 *     拦截器看到这个 tag 就不会再插 Bearer。
 */
interface ApiService {

    // ---------- Health ----------
    @GET("/healthz")
    suspend fun healthz(): Response<Unit>

    // ---------- Auth ----------
    //
    // 三端均强制 OAuth 登录,服务端 /api/auth/register 与 /api/auth/login 在
    // OAUTH_ENABLED=true 下会返回 403。Android 端走 OAuth + poll 方案:
    //
    //   1) Custom Tabs 打开 ${baseUrl}/api/auth/oauth/start?client=android&device_id=<random>
    //   2) 用户在系统浏览器里完成 OAuth(可以走 Profile / 任意 IdP)
    //   3) 应用持续 poll /api/auth/oauth/poll?device_id=<random> 拿 handoff
    //   4) 拿到 handoff -> POST /api/auth/oauth/finalize 换本服务的 access/refresh token
    //
    // 详见 server/internal/handlers/oauth.go。

    @GET("/api/auth/config")
    suspend fun authConfig(): Response<AuthConfigDto>

    @GET("/api/auth/oauth/poll")
    suspend fun oauthPoll(@Query("device_id") deviceId: String): Response<OAuthPollResponse>

    @POST("/api/auth/oauth/finalize")
    suspend fun oauthFinalize(@Body body: OAuthFinalizeRequest): Response<AuthResponse>

    @POST("/api/auth/refresh")
    suspend fun refresh(@Body body: RefreshRequest): Response<AuthResponse>

    @POST("/api/auth/logout")
    suspend fun logout(@Body body: LogoutRequest = LogoutRequest()): Response<Unit>

    @GET("/api/auth/me")
    suspend fun me(): Response<UserDto>

    @PATCH("/api/auth/me")
    suspend fun updateMe(@Body body: UpdateMeRequest): Response<UserDto>

    // ---------- Lists ----------
    @GET("/api/lists")
    suspend fun listsAll(): Response<ListsResponse>

    @POST("/api/lists")
    suspend fun listCreate(@Body body: ListInput): Response<ListDto>

    @PUT("/api/lists/{id}")
    suspend fun listUpdate(@Path("id") id: Long, @Body body: ListInput): Response<ListDto>

    @DELETE("/api/lists/{id}")
    suspend fun listDelete(@Path("id") id: Long): Response<Unit>

    // ---------- Todos ----------
    @GET("/api/todos")
    suspend fun todosList(
        @Query("filter") filter: String? = null,
        @Query("list_id") listId: Long? = null,
        @Query("search") search: String? = null,
        @Query("limit") limit: Int? = null,
        @Query("offset") offset: Int? = null,
        @Query("order_by") orderBy: String? = null,
        @Query("include_done") includeDone: Boolean? = null,
    ): Response<TodosResponse>

    @GET("/api/todos/{id}")
    suspend fun todoGet(@Path("id") id: Long): Response<TodoDto>

    @POST("/api/todos")
    suspend fun todoCreate(@Body body: TodoInput): Response<TodoDto>

    @PUT("/api/todos/{id}")
    suspend fun todoUpdate(@Path("id") id: Long, @Body body: TodoInput): Response<TodoDto>

    @DELETE("/api/todos/{id}")
    suspend fun todoDelete(@Path("id") id: Long): Response<Unit>

    @POST("/api/todos/{id}/complete")
    suspend fun todoComplete(@Path("id") id: Long): Response<TodoDto>

    @POST("/api/todos/{id}/uncomplete")
    suspend fun todoUncomplete(@Path("id") id: Long): Response<TodoDto>

    // ---------- Subtasks ----------
    @GET("/api/todos/{todo_id}/subtasks")
    suspend fun subtasksByTodo(@Path("todo_id") todoId: Long): Response<SubtasksResponse>

    @POST("/api/todos/{todo_id}/subtasks")
    suspend fun subtaskCreate(@Path("todo_id") todoId: Long, @Body body: SubtaskInput): Response<SubtaskDto>

    @PUT("/api/subtasks/{id}")
    suspend fun subtaskUpdate(@Path("id") id: Long, @Body body: SubtaskInput): Response<SubtaskDto>

    @DELETE("/api/subtasks/{id}")
    suspend fun subtaskDelete(@Path("id") id: Long): Response<Unit>

    @POST("/api/subtasks/{id}/complete")
    suspend fun subtaskComplete(@Path("id") id: Long): Response<SubtaskDto>

    @POST("/api/subtasks/{id}/uncomplete")
    suspend fun subtaskUncomplete(@Path("id") id: Long): Response<SubtaskDto>

    // ---------- Reminders ----------
    @GET("/api/reminders")
    suspend fun remindersList(@Query("todo_id") todoId: Long? = null): Response<RemindersResponse>

    @GET("/api/reminders/{id}")
    suspend fun reminderGet(@Path("id") id: Long): Response<ReminderDto>

    @POST("/api/reminders")
    suspend fun reminderCreate(@Body body: ReminderInput): Response<ReminderDto>

    @PUT("/api/reminders/{id}")
    suspend fun reminderUpdate(@Path("id") id: Long, @Body body: ReminderInput): Response<ReminderDto>

    @DELETE("/api/reminders/{id}")
    suspend fun reminderDelete(@Path("id") id: Long): Response<Unit>

    @POST("/api/reminders/{id}/enable")
    suspend fun reminderEnable(@Path("id") id: Long): Response<ReminderDto>

    @POST("/api/reminders/{id}/disable")
    suspend fun reminderDisable(@Path("id") id: Long): Response<ReminderDto>

    // ---------- Notifications ----------
    @GET("/api/notifications")
    suspend fun notifications(
        @Query("only_unread") onlyUnread: Boolean? = null,
        @Query("limit") limit: Int? = null,
        @Query("offset") offset: Int? = null,
    ): Response<NotificationsResponse>

    @GET("/api/notifications/unread-count")
    suspend fun unreadCount(): Response<UnreadCountResponse>

    @POST("/api/notifications/{id}/read")
    suspend fun notificationMarkRead(@Path("id") id: Long): Response<Unit>

    @POST("/api/notifications/read-all")
    suspend fun notificationsMarkAllRead(): Response<Unit>

    // ---------- Telegram ----------
    @POST("/api/telegram/bind-token")
    suspend fun telegramBindToken(): Response<TelegramBindToken>

    @GET("/api/telegram/bind-status")
    suspend fun telegramBindStatus(@Query("token") token: String): Response<TelegramBindStatus>

    @GET("/api/telegram/bindings")
    suspend fun telegramBindings(): Response<TelegramBindingsResponse>

    @POST("/api/telegram/unbind")
    suspend fun telegramUnbind(@Body body: TelegramUnbindRequest): Response<Unit>

    @POST("/api/telegram/test")
    suspend fun telegramTest(@Body body: TelegramTestRequest): Response<Unit>

    // ---------- Sync ----------
    @GET("/api/sync/pull")
    suspend fun syncPull(
        @Query("since") since: Long,
        @Query("limit") limit: Int = 200,
    ): Response<SyncPullResponse>

    @GET("/api/sync/cursor")
    suspend fun syncCursor(): Response<CursorResponse>

    // ---------- Pomodoro ----------
    @GET("/api/pomodoro/sessions")
    suspend fun pomodoroList(@Query("limit") limit: Int? = 50): Response<PomodoroListResponse>

    @POST("/api/pomodoro/sessions")
    suspend fun pomodoroCreate(@Body body: PomodoroStartRequest): Response<PomodoroSessionDto>

    @POST("/api/pomodoro/sessions/{id}/complete")
    suspend fun pomodoroComplete(@Path("id") id: Long): Response<PomodoroSessionDto>

    @POST("/api/pomodoro/sessions/{id}/abandon")
    suspend fun pomodoroAbandon(@Path("id") id: Long): Response<PomodoroSessionDto>

    // ---------- Stats ----------
    @GET("/api/stats/summary")
    suspend fun statsSummary(): Response<StatsSummaryDto>

    // ---------- Preferences (规格 §17) ----------
    @GET("/api/me/preferences")
    suspend fun preferencesList(@Query("scope") scope: String? = null): Response<PreferencesListResponse>

    @PUT("/api/me/preferences/{scope}/{key}")
    suspend fun preferencePut(
        @Path("scope") scope: String,
        @Path("key") key: String,
        @Body body: PreferencePutRequest,
    ): Response<PreferenceDto>

    @PUT("/api/me/preferences")
    suspend fun preferencesBulk(@Body body: PreferenceBulkRequest): Response<PreferencesListResponse>

    @DELETE("/api/me/preferences/{scope}/{key}")
    suspend fun preferenceDelete(
        @Path("scope") scope: String,
        @Path("key") key: String,
    ): Response<Unit>
}
