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
	"github.com/youruser/taskflow/internal/scheduler"
	"github.com/youruser/taskflow/internal/server"
	"github.com/youruser/taskflow/internal/store"
	"github.com/youruser/taskflow/internal/telegram"
)

var version = "0.3.0"

func main() {
	cfgPath := flag.String("config", "config.toml", "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
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
		Bot:           bot,
		BotUsername:   cfg.Telegram.BotUsername,
		WebhookSecret: cfg.Telegram.WebhookSecret,
		BindTokenTTL:  bindTTL,
		Hub:           hub,
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
