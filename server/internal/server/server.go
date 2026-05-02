package server

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/youruser/taskflow/internal/auth"
	"github.com/youruser/taskflow/internal/events"
	"github.com/youruser/taskflow/internal/handlers"
	"github.com/youruser/taskflow/internal/middleware"
	"github.com/youruser/taskflow/internal/oauth"
	"github.com/youruser/taskflow/internal/store"
	"github.com/youruser/taskflow/internal/telegram"
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
	Prefs         *store.PreferenceStore
	Audit         *store.AuditStore

	Bot           *telegram.Client
	BotUsername   string
	WebhookSecret string
	BindTokenTTL  time.Duration

	Hub *events.Hub

	// OAuthProvider 与 OAuthPending 都为 nil 时,本地邮箱注册/登录走原流程,
	// /api/auth/config 返回 oauth_enabled=false。两者必须成对出现(都设或都不设)。
	OAuthProvider *oauth.Provider
	OAuthPending  *oauth.PendingStore

	// 管理面板:数据库文件路径(给 /api/admin/system 报磁盘占用)、进程启动时间(报 uptime)、版本字符串。
	DBPath    string
	StartedAt time.Time
	Version   string

	// 管理面板设置摘要(GET /api/admin/settings)。Server 装配时按需提供。
	SettingsView func() handlers.SettingsView
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
	prefsH := handlers.NewPreferencesHandler(d.Prefs)

	// === 公开路由(不需要认证) ===
	mux.HandleFunc("GET /healthz", healthH.Health)

	// 本地邮箱注册/登录:OAuth 启用时关闭(返回 403),否则保持原行为。
	oauthEnabled := d.OAuthProvider != nil && d.OAuthPending != nil
	if oauthEnabled {
		oauthH := handlers.NewOAuthHandler(d.OAuthProvider, d.OAuthPending, d.Issuer, d.Users, d.RefreshTokens, d.Logger)
		// 关闭本地凭证流(避免与认证中心账号脱节)。
		mux.Handle("POST /api/auth/register", handlers.DisabledLocalAuthHandler())
		mux.Handle("POST /api/auth/login", handlers.DisabledLocalAuthHandler())
		// OAuth 流程
		mux.HandleFunc("GET /api/auth/oauth/start", oauthH.Start)
		mux.HandleFunc("GET /api/auth/oauth/callback", oauthH.Callback)
		mux.HandleFunc("POST /api/auth/oauth/finalize", oauthH.Finalize)
		// 桌面 / Android 客户端轮询拿 handoff(代替 OS 自定义 URL scheme)
		mux.HandleFunc("GET /api/auth/oauth/poll", oauthH.Poll)
		// 桌面 / Android 用户在浏览器里完成 OAuth 后看到的"请回客户端"静态页
		mux.HandleFunc("GET /api/auth/oauth/done", oauthH.Done)
		mux.HandleFunc("GET /api/auth/config", oauthH.Config)
	} else {
		mux.HandleFunc("POST /api/auth/register", authH.Register)
		mux.HandleFunc("POST /api/auth/login", authH.Login)
		mux.HandleFunc("GET /api/auth/config", handlers.AuthConfigDisabled)
	}
	// refresh / logout 在两种模式下都用同一份(本服务自己的 JWT,与外部 IdP 无关)。
	mux.HandleFunc("POST /api/auth/refresh", authH.Refresh)

	// Telegram webhook 是公开路由,但通过 X-Telegram-Bot-Api-Secret-Token 验证
	mux.HandleFunc("POST /api/telegram/webhook", telegramH.Webhook)
	// Telegram 配置探测端点：前端打开绑定页时用来判断管理员是否启用了集成
	mux.HandleFunc("GET /api/telegram/config", telegramH.GetConfig)

	// === 需要认证的路由 ===
	requireAuth := middleware.RequireAuth(d.Issuer)
	authed := func(h http.HandlerFunc) http.Handler {
		return requireAuth(h)
	}

	mux.Handle("POST /api/auth/logout", authed(authH.Logout))
	mux.Handle("GET /api/auth/me", authed(authH.Me))
	mux.Handle("PATCH /api/auth/me", authed(authH.UpdateMe))

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

	// 跨端用户偏好(规格 §17 阶段 13):每端只展示自己 scope 的开关,
	// 但全部都经过这一组路由持久化到服务端,登录到任何端都能拿到完整集合。
	mux.Handle("GET /api/me/preferences", authed(prefsH.List))
	mux.Handle("PUT /api/me/preferences", authed(prefsH.PutBulk))
	mux.Handle("PUT /api/me/preferences/{scope}/{key}", authed(prefsH.PutOne))
	mux.Handle("DELETE /api/me/preferences/{scope}/{key}", authed(prefsH.DeleteOne))

	// === 管理面板路由(管理员独占) ===
	//
	// RequireAdmin 在 RequireAuth 之后再加一道权限闸,非管理员一律 403。
	// 所有写操作都会写一条 audit_log。
	if d.Audit != nil {
		adminH := handlers.NewAdminHandler(
			d.DB, d.Issuer, d.Users, d.RefreshTokens, d.Audit,
			d.Logger, d.DBPath, d.StartedAt, d.Version, oauthEnabled,
		)
		if d.SettingsView != nil {
			adminH.SetSettingsProvider(d.SettingsView)
		}
		requireAdmin := middleware.RequireAdmin(d.Users)
		admined := func(h http.HandlerFunc) http.Handler {
			// 链路:RequireAuth -> RequireAdmin -> 业务 handler。
			// 都是 func(http.Handler) http.Handler,直接组合即可。
			return requireAuth(requireAdmin(h))
		}
		mux.Handle("GET /api/admin/system", admined(adminH.System))
		mux.Handle("GET /api/admin/settings", admined(adminH.Settings))
		mux.Handle("GET /api/admin/users", admined(adminH.ListUsers))
		mux.Handle("POST /api/admin/users", admined(adminH.CreateUser))
		mux.Handle("PATCH /api/admin/users/{id}", admined(adminH.PatchUser))
		mux.Handle("DELETE /api/admin/users/{id}", admined(adminH.DeleteUser))
		mux.Handle("GET /api/admin/audit", admined(adminH.ListAudit))
		mux.Handle("POST /api/admin/cleanup", admined(adminH.Cleanup))
	}

	// 顶层 middleware:Recover -> Logger -> CORS -> mux
	chain := middleware.Chain(
		middleware.Recover(d.Logger),
		middleware.Logger(d.Logger),
		middleware.CORS(),
	)
	return chain(mux)
}
