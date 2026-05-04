package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/youruser/taskflow/internal/auth"
	"github.com/youruser/taskflow/internal/config"
	"github.com/youruser/taskflow/internal/db"
	"github.com/youruser/taskflow/internal/events"
	"github.com/youruser/taskflow/internal/handlers"
	"github.com/youruser/taskflow/internal/oauth"
	"github.com/youruser/taskflow/internal/scheduler"
	"github.com/youruser/taskflow/internal/server"
	"github.com/youruser/taskflow/internal/store"
	"github.com/youruser/taskflow/internal/telegram"
)

var version = "1.4.1"

func main() {
	cfgPath := flag.String("config", "config.toml", "path to config file")
	envPath := flag.String("env", "../.env", "path to root .env file (project uses single .env at repo root)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	// 先加载 .env(可选,文件不存在不报错),再读 config.toml。
	// OAuth 段配置完全来自环境变量(.env / 进程环境),URL/client_id/secret 与
	// 仓库里的 TOML 模板分离 —— 见 .env.example。
	if err := config.LoadEnvFile(*envPath); err != nil {
		fmt.Fprintf(os.Stderr, "load env file: %v\n", err)
		os.Exit(2)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(2)
	}

	logger := newLogger(cfg.Log.Level)
	slog.SetDefault(logger)

	logger.Info("starting taskflow-server",
		"version", version,
		"listen", cfg.Server.Listen,
		"db_path", cfg.Database.Path,
	)

	// 打开数据库 + 迁移
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		logger.Error("open db", "err", err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()

	migCtx, migCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := db.Migrate(migCtx, database); err != nil {
		migCancel()
		logger.Error("migrate db", "err", err)
		os.Exit(1)
	}
	migCancel()

	// 装配依赖
	issuer := auth.NewIssuer(
		cfg.Auth.JWTSecret,
		time.Duration(cfg.Auth.AccessTTLSeconds)*time.Second,
		time.Duration(cfg.Auth.RefreshTTLSeconds)*time.Second,
		cfg.Auth.BcryptCost,
	)
	syncEvents := store.NewSyncEventStore(database)
	users := store.NewUserStore(database)
	refreshTokens := store.NewRefreshTokenStore(database)
	lists := store.NewListStore(database, syncEvents)
	todos := store.NewTodoStore(database, syncEvents)
	subtasks := store.NewSubtaskStore(database, syncEvents)
	reminders := store.NewReminderStore(database, syncEvents)
	telegrams := store.NewTelegramStore(database, syncEvents)
	notifications := store.NewNotificationStore(database, syncEvents)
	pomos := store.NewPomodoroStore(database, syncEvents)
	stats := store.NewStatsStore(database)
	prefs := store.NewPreferenceStore(database)
	audits := store.NewAuditStore(database)

	// 启动管理员引导:读 ADMIN_EMAIL / ADMIN_PASSWORD,确保至少有一个管理员账号。
	// 已存在同邮箱用户 -> 提升为管理员(不改密码);不存在 -> 用 ADMIN_PASSWORD 建一个新管理员。
	bootstrapAdmin(logger, users, issuer)

	// Telegram 客户端 —— 即使没配 token 也安全:Enabled() 返回 false,各处会跳过。
	bot := telegram.NewClient(cfg.Telegram.BotToken, "")
	if bot.Enabled() {
		logger.Info("telegram bot enabled", "username", cfg.Telegram.BotUsername)
	} else {
		logger.Info("telegram bot disabled (no bot_token configured)")
	}

	// 进程内 SSE 总线
	hub := events.NewHub()
	defer hub.Shutdown()

	// 后台调度器
	var sched *scheduler.Scheduler
	if !cfg.Scheduler.Disabled {
		sched = scheduler.New(
			scheduler.Config{
				TickInterval: time.Duration(cfg.Scheduler.TickIntervalSeconds) * time.Second,
				BatchSize:    cfg.Scheduler.BatchSize,
			},
			logger,
			reminders,
			notifications,
			telegrams,
			bot,
			hub,
		)
		sched.Start()
		logger.Info("scheduler started",
			"tick_seconds", cfg.Scheduler.TickIntervalSeconds,
			"batch_size", cfg.Scheduler.BatchSize)
	} else {
		logger.Warn("scheduler disabled by config")
	}

	bindTTL := time.Duration(cfg.Telegram.BindTokenTTLSeconds) * time.Second

	// OAuth 接入认证中心(可选)。disabled 时 oauthProvider/oauthPending 都是 nil,
	// server.BuildHandler 会按本地邮箱密码模式装配路由。
	var (
		oauthProvider *oauth.Provider
		oauthPending  *oauth.PendingStore
		stopOAuthGC   func()
	)
	if cfg.OAuth.Enabled {
		oauthProvider = oauth.NewProvider(
			cfg.OAuth.Provider,
			cfg.OAuth.AuthorizeURL,
			cfg.OAuth.TokenURL,
			cfg.OAuth.UserInfoURL,
			cfg.OAuth.ClientID,
			cfg.OAuth.ClientSecret,
			cfg.OAuth.RedirectURL,
			cfg.OAuth.FrontendRedirectURL,
			cfg.OAuth.Scopes,
			cfg.OAuth.EmailField,
			cfg.OAuth.NameField,
			cfg.OAuth.SubjectField,
		)
		// state 给浏览器跳认证中心 + 跳回的总停留时间留 10 分钟,handoff 60 秒。
		oauthPending = oauth.NewPendingStore(10*time.Minute, 60*time.Second)
		stopOAuthGC = startOAuthGC(oauthPending)
		logger.Info("oauth enabled",
			"provider", cfg.OAuth.Provider,
			"authorize_url", cfg.OAuth.AuthorizeURL,
			"redirect_url", cfg.OAuth.RedirectURL)
	} else {
		logger.Info("oauth disabled (using local email/password)")
	}
	if stopOAuthGC != nil {
		defer stopOAuthGC()
	}

	startedAt := time.Now().UTC()

	handler := server.BuildHandler(server.Deps{
		DB:            database,
		Issuer:        issuer,
		Logger:        logger,
		Users:         users,
		RefreshTokens: refreshTokens,
		Lists:         lists,
		Todos:         todos,
		Subtasks:      subtasks,
		Reminders:     reminders,
		Sync:          syncEvents,
		Telegram:      telegrams,
		Notifications: notifications,
		Pomos:         pomos,
		Stats:         stats,
		Prefs:         prefs,
		Audit:         audits,
		Bot:           bot,
		BotUsername:   cfg.Telegram.BotUsername,
		WebhookSecret: cfg.Telegram.WebhookSecret,
		BindTokenTTL:  bindTTL,
		Hub:           hub,
		OAuthProvider: oauthProvider,
		OAuthPending:  oauthPending,
		DBPath:        cfg.Database.Path,
		StartedAt:     startedAt,
		Version:       version,
		SettingsView: func() handlers.SettingsView {
			return handlers.SettingsView{
				OAuthEnabled:        cfg.OAuth.Enabled,
				OAuthProvider:       cfg.OAuth.Provider,
				OAuthRedirectURL:    cfg.OAuth.RedirectURL,
				BotEnabled:          bot.Enabled(),
				BotUsername:         cfg.Telegram.BotUsername,
				AccessTTLSeconds:    cfg.Auth.AccessTTLSeconds,
				RefreshTTLSeconds:   cfg.Auth.RefreshTTLSeconds,
				BcryptCost:          cfg.Auth.BcryptCost,
				SchedulerTick:       cfg.Scheduler.TickIntervalSeconds,
				SchedulerBatch:      cfg.Scheduler.BatchSize,
				SchedulerDisabled:   cfg.Scheduler.Disabled,
				ServerListen:        cfg.Server.Listen,
				DatabasePath:        cfg.Database.Path,
				AdminBootstrapEmail: os.Getenv("ADMIN_EMAIL"),
			}
		},
	})

	// HTTP 服务器
	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           handler,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      0, // SSE 长连接需要 0 (写超时设到非零会切流)
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 周期清理过期 refresh token / 过期 bind_token
	stopCleanup := startBackgroundCleanups(logger, refreshTokens, telegrams)
	defer stopCleanup()

	// 启动
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.Server.Listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// 等待 SIGINT/SIGTERM 或启动错误
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		logger.Info("shutdown signal", "signal", sig.String())
	case err := <-errCh:
		logger.Error("server error", "err", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(),
		time.Duration(cfg.Server.ShutdownTimeoutSeconds)*time.Second)
	defer cancel()

	// 先停 HTTP server,拒绝新连接;在飞的请求等 timeout
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown", "err", err)
	} else {
		logger.Info("http server stopped")
	}

	// 再停调度器
	if sched != nil {
		schedCtx, schedCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := sched.Stop(schedCtx); err != nil {
			logger.Warn("scheduler stop", "err", err)
		} else {
			logger.Info("scheduler stopped")
		}
		schedCancel()
	}
	logger.Info("bye")
}

// startBackgroundCleanups 每小时清理一次过期 refresh token / bind token。
func startBackgroundCleanups(log *slog.Logger, rt *store.RefreshTokenStore, tg *store.TelegramStore) func() {
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		runOnce := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if n, err := rt.CleanupExpired(ctx); err != nil {
				log.Warn("cleanup refresh tokens", "err", err)
			} else if n > 0 {
				log.Info("cleanup refresh tokens", "removed", n)
			}
			if n, err := tg.CleanupExpiredTokens(ctx); err != nil {
				log.Warn("cleanup telegram bind tokens", "err", err)
			} else if n > 0 {
				log.Info("cleanup telegram bind tokens", "removed", n)
			}
		}
		runOnce()
		for {
			select {
			case <-t.C:
				runOnce()
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}

// startOAuthGC 每 5 分钟清一次 OAuth 短期凭证(state / handoff_code)。
// 返回 stop 函数;调用方在进程退出前关闭它。
func startOAuthGC(p *oauth.PendingStore) func() {
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				p.GC()
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}

// bootstrapAdmin 在启动时根据 .env 中的 ADMIN_EMAIL / ADMIN_PASSWORD 引导一个管理员账号。
//
// 行为:
//   - ADMIN_EMAIL 为空 -> 不做任何事(也不报错)。
//   - 用户已存在且已是管理员 -> 跳过(不改密码)。
//   - 用户已存在但不是管理员 -> 提升为管理员(不改密码,日志里记一笔)。
//   - 用户不存在 -> 用 ADMIN_PASSWORD 建一个新管理员;ADMIN_PASSWORD 为空时报错并退出。
//
// 设计上不在管理员已经存在时再用 ADMIN_PASSWORD 强制改密码 —— 否则
// 把 .env 的密码忘改回默认值会无声覆盖现有强密码;若你确实想强制重置,
// 可以先在管理面板里删掉这个用户,然后下次启动会重新创建。
func bootstrapAdmin(logger *slog.Logger, users *store.UserStore, issuer *auth.Issuer) {
	email := strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
	if email == "" {
		return
	}
	password := os.Getenv("ADMIN_PASSWORD")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 已存在 -> 仅确保 is_admin = 1。
	if existing, _, err := users.GetByEmailWithHash(ctx, email); err == nil {
		if existing.IsAdmin && !existing.IsDisabled {
			logger.Info("admin bootstrap: user already an admin, skipping",
				"email", email, "user_id", existing.ID)
			return
		}
		if _, err := users.EnsureAdminByEmail(ctx, email); err != nil {
			logger.Error("admin bootstrap: promote existing user failed",
				"email", email, "err", err)
			return
		}
		logger.Warn("admin bootstrap: existing user promoted to admin (password not changed)",
			"email", email)
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		logger.Error("admin bootstrap: lookup failed", "email", email, "err", err)
		return
	}

	// 不存在 -> 必须有密码才能建。
	if len(password) < 8 {
		logger.Error("admin bootstrap: ADMIN_EMAIL set but ADMIN_PASSWORD missing/too short " +
			"(>= 8 chars required) — refusing to create admin")
		return
	}
	hash, err := issuer.HashPassword(password)
	if err != nil {
		logger.Error("admin bootstrap: hash password failed", "err", err)
		return
	}
	u, err := users.CreateAdmin(ctx, email, hash, "", "")
	if err != nil {
		logger.Error("admin bootstrap: create admin failed", "email", email, "err", err)
		return
	}
	logger.Warn("admin bootstrap: created initial admin from .env — CHANGE THE PASSWORD ASAP",
		"email", email, "user_id", u.ID)
}
