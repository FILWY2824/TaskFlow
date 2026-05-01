// Package scheduler 周期扫描到期的 reminder_rules,创建 notifications,
// 投递到 telegram 通道并写 delivery 日志,最后用 RRULE 推进 next_fire_at。
//
// 关键设计:
//
//   - 单进程内只跑一个 scheduler,内存里没有"已派发"集合;每次扫描都依据数据库
//     的 next_fire_at <= now 判断,处理完立刻推进 next_fire_at 到严格 > now,
//     这样多核 / SIGSTOP / 时钟跳变都不会重复触发。
//
//   - "channel_local" 不在服务端做任何事;它属于 Android/Windows 端的 AlarmManager
//     / 本地调度器职责。服务端只负责创建 notification 行(供通知中心展示)和把
//     channel_telegram / channel_web_push 投出去。
//
//   - 投递失败不阻塞推进 next_fire_at。失败会写一条 status=failed 的 delivery 行,
//     让用户在通知中心能看到"上次没送出"。重试机制留作 phase 5 的扩展项,MVP 不做。
//
//   - tick interval 默认 5 秒,意味着提醒触发延迟在 0~5s 之间,对人类感知足够。
//     若需要秒级精度,客户端那边的本地强提醒(AlarmManager / Windows ScheduledToast)
//     才是权威源,服务端 telegram 推送是补充。
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/youruser/todoalarm/internal/events"
	"github.com/youruser/todoalarm/internal/rrule"
	"github.com/youruser/todoalarm/internal/store"
	"github.com/youruser/todoalarm/internal/telegram"
)

// Config 调度器参数。零值之外可调:
//
//	TickInterval = 5s
//	BatchSize    = 200
//	MaxAttempts  = 1   (本期不做重试)
type Config struct {
	TickInterval time.Duration
	BatchSize    int
}

// Scheduler 一个长生命周期的后台服务。
type Scheduler struct {
	cfg           Config
	logger        *slog.Logger
	reminders     *store.ReminderStore
	notifications *store.NotificationStore
	telegrams     *store.TelegramStore
	bot           *telegram.Client
	hub           *events.Hub

	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// New 构造调度器。bot 可以是 disabled(token 未配置),那时 telegram 通道全部记 skipped。
func New(cfg Config, logger *slog.Logger,
	reminders *store.ReminderStore,
	notifications *store.NotificationStore,
	telegrams *store.TelegramStore,
	bot *telegram.Client,
	hub *events.Hub) *Scheduler {

	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 5 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 200
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		cfg:           cfg,
		logger:        logger.With("component", "scheduler"),
		reminders:     reminders,
		notifications: notifications,
		telegrams:     telegrams,
		bot:           bot,
		hub:           hub,
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
}

// Start 启动后台 goroutine,立即返回。Stop() 之前可重复调用 Start —— 但内部 sync.Once 保证只跑一次。
func (s *Scheduler) Start() {
	s.once.Do(func() {
		go s.loop()
	})
}

// Stop 优雅停止;最多等 ctx 截止。
func (s *Scheduler) Stop(ctx context.Context) error {
	close(s.stop)
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) loop() {
	defer close(s.done)
	t := time.NewTicker(s.cfg.TickInterval)
	defer t.Stop()

	// 启动时立即扫一次,然后按 tick 节奏继续。
	s.tick()
	for {
		select {
		case <-s.stop:
			s.logger.Info("scheduler stopping")
			return
		case <-t.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	// panic 守卫 —— 调度器死了相当于整个产品的提醒都不工作,要尽量保活。
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("scheduler tick panic", "panic", rec)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	due, err := s.reminders.ListDue(ctx, now, s.cfg.BatchSize)
	if err != nil {
		s.logger.Error("list due", "err", err)
		return
	}
	if len(due) == 0 {
		return
	}
	s.logger.Debug("dispatching due reminders", "count", len(due))

	for _, d := range due {
		s.dispatchOne(ctx, now, d)
	}
}

// dispatchOne 处理一条 due 提醒。
//
// 步骤:
//  1. 创建 notification 行(就算所有通道都 skip,通知中心也要有这条记录)。
//  2. 推一次 SSE 给在线客户端(尽力而为)。
//  3. 对每个开启的服务端通道(目前只有 telegram)做投递并写 delivery log。
//  4. 调 rrule 算出严格 > now 的下一次,RecordFire 推进 / 关闭。
func (s *Scheduler) dispatchOne(ctx context.Context, now time.Time, d *store.DueReminder) {
	title := strings.TrimSpace(d.Title)
	if title == "" {
		title = "TaskFlow 提醒"
	}
	body := s.buildBody(d)

	notif, err := s.notifications.Create(ctx, store.CreateNotificationInput{
		UserID:         d.UserID,
		ReminderRuleID: &d.ID,
		TodoID:         d.TodoID,
		Title:          title,
		Body:           body,
		FireAt:         d.NextFireAt,
	})
	if err != nil {
		s.logger.Error("create notification", "err", err, "rule_id", d.ID)
		return
	}

	// 推送 SSE(在线 web/桌面客户端可以立刻把它弹到通知中心而不用等下一次 sync 拉取)
	s.hub.Publish(d.UserID, events.Event{
		Type:           "notification",
		NotificationID: notif.ID,
		ReminderRuleID: d.ID,
		TodoID:         deref(d.TodoID),
		Title:          title,
		Body:           body,
		FireAtUnix:     d.NextFireAt.Unix(),
	})

	// channel_telegram
	if d.ChannelTelegram {
		s.deliverTelegram(ctx, notif.ID, d.UserID, title, body)
	}

	// channel_web_push 留给后续 phase。MVP 仅记 skipped。
	if d.ChannelWebPush {
		_ = s.notifications.LogDelivery(ctx, notif.ID, "web_push", "skipped",
			"web push not implemented in MVP", 0, nil)
	}

	// channel_local 是客户端职责,服务端不做任何投递。
	// 不写 delivery 行(用户每条都看到一条 skipped 也吵)。

	// 推进 next_fire_at
	next, err := rrule.ComputeNextFire(d.TriggerAt, d.RRule, d.DTStart, d.Timezone, now)
	if err != nil {
		// RRULE 算不出来不应该发生(创建时已 Validate 过),但保底处理:停掉这条。
		s.logger.Error("compute next fire", "err", err, "rule_id", d.ID)
		next = nil
	}
	if err := s.reminders.RecordFire(ctx, d.UserID, d.ID, now, next); err != nil {
		s.logger.Error("record fire", "err", err, "rule_id", d.ID)
	}
}

func (s *Scheduler) deliverTelegram(ctx context.Context, notifID, userID int64, title, body string) {
	if !s.bot.Enabled() {
		_ = s.notifications.LogDelivery(ctx, notifID, "telegram", "skipped",
			"telegram bot_token not configured", 0, nil)
		return
	}
	bindings, err := s.telegrams.ListEnabledBindingsForUser(ctx, userID)
	if err != nil {
		_ = s.notifications.LogDelivery(ctx, notifID, "telegram", "failed",
			"list bindings: "+err.Error(), 1, nil)
		return
	}
	if len(bindings) == 0 {
		_ = s.notifications.LogDelivery(ctx, notifID, "telegram", "skipped",
			"no enabled bindings", 0, nil)
		return
	}

	text := title
	if body != "" {
		text = title + "\n\n" + body
	}

	// 同一用户多端绑定(罕见但允许)逐个发送,任意一个成功就视为成功。
	var lastErr error
	delivered := false
	for _, b := range bindings {
		// SendMessage 用独立的子 ctx,避免单个发送拖慢整条 dispatch
		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		_, err := s.bot.SendMessage(sendCtx, b.ChatID, text, telegram.SendMessageOptions{})
		cancel()
		if err == nil {
			delivered = true
			break
		}
		lastErr = err
	}
	now := time.Now().UTC()
	if delivered {
		_ = s.notifications.LogDelivery(ctx, notifID, "telegram", "delivered", "", 1, &now)
		return
	}
	errStr := ""
	if lastErr != nil {
		errStr = lastErr.Error()
	}
	_ = s.notifications.LogDelivery(ctx, notifID, "telegram", "failed", errStr, 1, nil)
}

// buildBody 拼一段简单的 body。后续可加任务详情、deep-link。
func (s *Scheduler) buildBody(d *store.DueReminder) string {
	loc, err := time.LoadLocation(d.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return fmt.Sprintf("到点啦 ⏰  时间:%s",
		d.NextFireAt.In(loc).Format("2006-01-02 15:04 MST"))
}

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
