package server

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/youruser/todoalarm/internal/auth"
	"github.com/youruser/todoalarm/internal/events"
	"github.com/youruser/todoalarm/internal/handlers"
	"github.com/youruser/todoalarm/internal/middleware"
	"github.com/youruser/todoalarm/internal/store"
	"github.com/youruser/todoalarm/internal/telegram"
)

// Deps 是构造路由所需的全部依赖。
type Deps struct {
	DB     *sql.DB
	Issuer *auth.Issuer
	Logger *slog.Logger

	Users         *store.UserStore
	RefreshTokens *store.RefreshTokenStore
	Lists         *store.ListStore
	Todos         *store.TodoStore
	Subtasks      *store.SubtaskStore
	Reminders     *store.ReminderStore
	Sync          *store.SyncEventStore
	Telegram      *store.TelegramStore
	Notifications *store.NotificationStore
	Pomos         *store.PomodoroStore
	Stats         *store.StatsStore

	Bot           *telegram.Client
	BotUsername   string
	WebhookSecret string
	BindTokenTTL  time.Duration

	Hub *events.Hub
}

// BuildHandler 构建顶层 http.Handler。
func BuildHandler(d Deps) http.Handler {
	mux := http.NewServeMux()

	// === 处理器 ===
	healthH := handlers.NewHealthHandler(d.DB)
	authH := handlers.NewAuthHandler(d.Issuer, d.Users, d.RefreshTokens)
	listsH := handlers.NewListsHandler(d.Lists)
	todosH := handlers.NewTodosHandler(d.Todos, d.Users)
	subtasksH := handlers.NewSubtasksHandler(d.Subtasks)
	remindersH := handlers.NewRemindersHandler(d.Reminders, d.Users)
	syncH := handlers.NewSyncHandler(d.Sync)
	telegramH := handlers.NewTelegramHandler(d.Telegram, d.Bot, d.BotUsername, d.WebhookSecret, d.BindTokenTTL, d.Logger)
	notifH := handlers.NewNotificationsHandler(d.Notifications)
	sseH := handlers.NewSSEHandler(d.Hub)
	pomoH := handlers.NewPomodoroHandler(d.Pomos)
	statsH := handlers.NewStatsHandler(d.Stats, d.Users)

	// === 公开路由(不需要认证) ===
	mux.HandleFunc("GET /healthz", healthH.Health)
	mux.HandleFunc("POST /api/auth/register", authH.Register)
	mux.HandleFunc("POST /api/auth/login", authH.Login)
	mux.HandleFunc("POST /api/auth/refresh", authH.Refresh)

	// Telegram webhook 是公开路由,但通过 X-Telegram-Bot-Api-Secret-Token 验证
	mux.HandleFunc("POST /api/telegram/webhook", telegramH.Webhook)

	// === 需要认证的路由 ===
	requireAuth := middleware.RequireAuth(d.Issuer)
	authed := func(h http.HandlerFunc) http.Handler {
		return requireAuth(h)
	}

	mux.Handle("POST /api/auth/logout", authed(authH.Logout))
	mux.Handle("GET /api/auth/me", authed(authH.Me))

	// Lists
	mux.Handle("GET /api/lists", authed(listsH.Index))
	mux.Handle("POST /api/lists", authed(listsH.Create))
	mux.Handle("PUT /api/lists/{id}", authed(listsH.Update))
	mux.Handle("DELETE /api/lists/{id}", authed(listsH.Delete))

	// Todos
	mux.Handle("GET /api/todos", authed(todosH.Index))
	mux.Handle("POST /api/todos", authed(todosH.Create))
	mux.Handle("GET /api/todos/{id}", authed(todosH.Show))
	mux.Handle("PUT /api/todos/{id}", authed(todosH.Update))
	mux.Handle("DELETE /api/todos/{id}", authed(todosH.Delete))
	mux.Handle("POST /api/todos/{id}/complete", authed(todosH.Complete))
	mux.Handle("POST /api/todos/{id}/uncomplete", authed(todosH.Uncomplete))

	// Subtasks
	mux.Handle("GET /api/todos/{todo_id}/subtasks", authed(subtasksH.Index))
	mux.Handle("POST /api/todos/{todo_id}/subtasks", authed(subtasksH.Create))
	mux.Handle("PUT /api/subtasks/{id}", authed(subtasksH.Update))
	mux.Handle("DELETE /api/subtasks/{id}", authed(subtasksH.Delete))
	mux.Handle("POST /api/subtasks/{id}/complete", authed(subtasksH.Complete))
	mux.Handle("POST /api/subtasks/{id}/uncomplete", authed(subtasksH.Uncomplete))

	// Reminders
	mux.Handle("GET /api/reminders", authed(remindersH.Index))
	mux.Handle("POST /api/reminders", authed(remindersH.Create))
	mux.Handle("GET /api/reminders/{id}", authed(remindersH.Show))
	mux.Handle("PUT /api/reminders/{id}", authed(remindersH.Update))
	mux.Handle("DELETE /api/reminders/{id}", authed(remindersH.Delete))
	mux.Handle("POST /api/reminders/{id}/enable", authed(remindersH.Enable))
	mux.Handle("POST /api/reminders/{id}/disable", authed(remindersH.Disable))

	// Sync
	mux.Handle("GET /api/sync/pull", authed(syncH.Pull))
	mux.Handle("GET /api/sync/cursor", authed(syncH.Cursor))

	// Telegram(认证后)
	mux.Handle("POST /api/telegram/bind-token", authed(telegramH.CreateBindToken))
	mux.Handle("GET /api/telegram/bind-status", authed(telegramH.GetBindStatus))
	mux.Handle("GET /api/telegram/bindings", authed(telegramH.ListBindings))
	mux.Handle("POST /api/telegram/unbind", authed(telegramH.Unbind))
	mux.Handle("POST /api/telegram/test", authed(telegramH.SendTest))

	// Notifications
	mux.Handle("GET /api/notifications", authed(notifH.Index))
	mux.Handle("GET /api/notifications/unread-count", authed(notifH.UnreadCount))
	mux.Handle("POST /api/notifications/read-all", authed(notifH.MarkAllRead))
	mux.Handle("GET /api/notifications/{id}", authed(notifH.Show))
	mux.Handle("POST /api/notifications/{id}/read", authed(notifH.MarkRead))

	// SSE 实时推送
	mux.Handle("GET /ws/events", authed(sseH.Stream))

	// Pomodoro(阶段 11)
	mux.Handle("GET /api/pomodoro/sessions", authed(pomoH.Index))
	mux.Handle("POST /api/pomodoro/sessions", authed(pomoH.Create))
	mux.Handle("GET /api/pomodoro/sessions/{id}", authed(pomoH.Show))
	mux.Handle("PUT /api/pomodoro/sessions/{id}", authed(pomoH.Update))
	mux.Handle("DELETE /api/pomodoro/sessions/{id}", authed(pomoH.Delete))
	mux.Handle("POST /api/pomodoro/sessions/{id}/complete", authed(pomoH.Complete))
	mux.Handle("POST /api/pomodoro/sessions/{id}/abandon", authed(pomoH.Abandon))

	// Stats(阶段 11)
	mux.Handle("GET /api/stats/summary", authed(statsH.Summary))
	mux.Handle("GET /api/stats/daily", authed(statsH.Daily))
	mux.Handle("GET /api/stats/weekly", authed(statsH.Weekly))
	mux.Handle("GET /api/stats/pomodoro", authed(statsH.Pomodoro))

	// 顶层 middleware:Recover -> Logger -> CORS -> mux
	chain := middleware.Chain(
		middleware.Recover(d.Logger),
		middleware.Logger(d.Logger),
		middleware.CORS(),
	)
	return chain(mux)
}
