package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// StatsStore 提供阶段 11 数据复盘所需的聚合查询。
//
// 设计原则:
//   - 所有按"日"聚合的查询接受用户时区(由 handler 层传入),在 SQL 中按本地日转 bucket。
//   - 不缓存(SQLite 索引 + 单用户数据量足够)。
//   - 所有 user_id 都参与 WHERE,避免越权。
type StatsStore struct {
	DB *sql.DB
}

func NewStatsStore(db *sql.DB) *StatsStore { return &StatsStore{DB: db} }

// Summary 用于 GET /api/stats/summary
type Summary struct {
	TodosTotal        int `json:"todos_total"`
	TodosOpen         int `json:"todos_open"`
	TodosCompleted    int `json:"todos_completed"`
	TodosOverdue      int `json:"todos_overdue"`
	TodosDueToday     int `json:"todos_due_today"`
	CompletedToday    int `json:"completed_today"`
	CompletedThisWk   int `json:"completed_this_week"`
	PomodoroTodaySec  int `json:"pomodoro_today_seconds"`
	PomodoroThisWkSec int `json:"pomodoro_this_week_seconds"`
}

// Summary 计算用户的总览统计(基于用户时区判定"今天"/"本周")。
func (s *StatsStore) Summary(ctx context.Context, userID int64, loc *time.Location, now time.Time) (*Summary, error) {
	if loc == nil {
		loc = time.UTC
	}
	startToday, endToday := dayRange(now, loc)
	startWk, endWk := weekRange(now, loc)
	nowUTC := now.UTC()

	out := &Summary{}

	// todos_total / todos_open / todos_completed (排除软删)
	if err := s.DB.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			SUM(CASE WHEN is_completed = 0 THEN 1 ELSE 0 END),
			SUM(CASE WHEN is_completed = 1 THEN 1 ELSE 0 END)
		FROM todos
		WHERE user_id = ? AND deleted_at IS NULL
	`, userID).Scan(&out.TodosTotal, &out.TodosOpen, &out.TodosCompleted); err != nil {
		return nil, fmt.Errorf("summary totals: %w", err)
	}

	// todos_overdue:有 due_at 且 < now 且未完成
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM todos
		WHERE user_id = ? AND deleted_at IS NULL AND is_completed = 0
			AND due_at IS NOT NULL AND due_at < ?
	`, userID, nowUTC).Scan(&out.TodosOverdue); err != nil {
		return nil, fmt.Errorf("summary overdue: %w", err)
	}

	// todos_due_today:due_at 落在用户今日(任意完成状态)
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM todos
		WHERE user_id = ? AND deleted_at IS NULL
			AND due_at IS NOT NULL AND due_at >= ? AND due_at < ?
	`, userID, startToday.UTC(), endToday.UTC()).Scan(&out.TodosDueToday); err != nil {
		return nil, fmt.Errorf("summary due today: %w", err)
	}

	// completed_today / completed_this_week:用 completed_at 落 bucket
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM todos
		WHERE user_id = ? AND deleted_at IS NULL AND is_completed = 1
			AND completed_at IS NOT NULL AND completed_at >= ? AND completed_at < ?
	`, userID, startToday.UTC(), endToday.UTC()).Scan(&out.CompletedToday); err != nil {
		return nil, fmt.Errorf("summary completed today: %w", err)
	}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM todos
		WHERE user_id = ? AND deleted_at IS NULL AND is_completed = 1
			AND completed_at IS NOT NULL AND completed_at >= ? AND completed_at < ?
	`, userID, startWk.UTC(), endWk.UTC()).Scan(&out.CompletedThisWk); err != nil {
		return nil, fmt.Errorf("summary completed week: %w", err)
	}

	// pomodoro 累计秒(只统计 focus 已完成 / 已放弃,break 不计)
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(actual_duration_seconds), 0) FROM pomodoro_sessions
		WHERE user_id = ? AND kind = 'focus' AND status IN ('completed','abandoned')
			AND started_at >= ? AND started_at < ?
	`, userID, startToday.UTC(), endToday.UTC()).Scan(&out.PomodoroTodaySec); err != nil {
		return nil, fmt.Errorf("summary pomodoro today: %w", err)
	}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(actual_duration_seconds), 0) FROM pomodoro_sessions
		WHERE user_id = ? AND kind = 'focus' AND status IN ('completed','abandoned')
			AND started_at >= ? AND started_at < ?
	`, userID, startWk.UTC(), endWk.UTC()).Scan(&out.PomodoroThisWkSec); err != nil {
		return nil, fmt.Errorf("summary pomodoro week: %w", err)
	}

	return out, nil
}

// DailyBucket 单天的统计行。
type DailyBucket struct {
	Date            string `json:"date"` // YYYY-MM-DD,用户时区
	Created         int    `json:"created"`
	Completed       int    `json:"completed"`
	PomodoroSeconds int    `json:"pomodoro_seconds"`
	PomodoroCount   int    `json:"pomodoro_count"`
}

// Daily 给定时间区间(用户本地时区,左闭右开)按天分桶。
//
// 实现:在 Go 里枚举每一天,逐日查询。用户即便高频用,30 天 * 4 sql 也只是 120 次轻查询,足够。
// 这种实现避免在 SQL 里做时区敏感的 GROUP BY(SQLite 没有原生 TZ 支持)。
func (s *StatsStore) Daily(ctx context.Context, userID int64, loc *time.Location, fromDate, toDate time.Time) ([]*DailyBucket, error) {
	if loc == nil {
		loc = time.UTC
	}
	from := startOfLocalDay(fromDate, loc)
	to := startOfLocalDay(toDate, loc) // exclusive
	if !to.After(from) {
		return []*DailyBucket{}, nil
	}
	// 上限 366 天,防滥用
	if to.Sub(from) > 367*24*time.Hour {
		return nil, fmt.Errorf("range too large, max 366 days")
	}

	var out []*DailyBucket
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		end := d.AddDate(0, 0, 1)
		b := &DailyBucket{Date: d.Format("2006-01-02")}

		if err := s.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM todos
			WHERE user_id = ? AND deleted_at IS NULL
				AND created_at >= ? AND created_at < ?
		`, userID, d.UTC(), end.UTC()).Scan(&b.Created); err != nil {
			return nil, err
		}
		if err := s.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM todos
			WHERE user_id = ? AND deleted_at IS NULL AND is_completed = 1
				AND completed_at IS NOT NULL AND completed_at >= ? AND completed_at < ?
		`, userID, d.UTC(), end.UTC()).Scan(&b.Completed); err != nil {
			return nil, err
		}
		if err := s.DB.QueryRowContext(ctx, `
			SELECT COUNT(*), COALESCE(SUM(actual_duration_seconds), 0)
			FROM pomodoro_sessions
			WHERE user_id = ? AND kind = 'focus'
				AND status IN ('completed','abandoned')
				AND started_at >= ? AND started_at < ?
		`, userID, d.UTC(), end.UTC()).Scan(&b.PomodoroCount, &b.PomodoroSeconds); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// WeeklyBucket 单周统计(ISO 周一~周日)。
type WeeklyBucket struct {
	WeekStart       string `json:"week_start"` // YYYY-MM-DD,周一
	WeekEnd         string `json:"week_end"`   // YYYY-MM-DD,下周一(exclusive)
	Created         int    `json:"created"`
	Completed       int    `json:"completed"`
	PomodoroSeconds int    `json:"pomodoro_seconds"`
	PomodoroCount   int    `json:"pomodoro_count"`
}

// Weekly 给定区间内按周聚合(以 from 所在周的周一为起点,周一对齐)。
func (s *StatsStore) Weekly(ctx context.Context, userID int64, loc *time.Location, fromDate, toDate time.Time) ([]*WeeklyBucket, error) {
	if loc == nil {
		loc = time.UTC
	}
	from := startOfLocalWeek(fromDate, loc)
	to := startOfLocalDay(toDate, loc)
	if !to.After(from) {
		return []*WeeklyBucket{}, nil
	}
	if to.Sub(from) > 53*7*24*time.Hour {
		return nil, fmt.Errorf("range too large, max 53 weeks")
	}

	var out []*WeeklyBucket
	for d := from; d.Before(to); d = d.AddDate(0, 0, 7) {
		end := d.AddDate(0, 0, 7)
		b := &WeeklyBucket{
			WeekStart: d.Format("2006-01-02"),
			WeekEnd:   end.Format("2006-01-02"),
		}
		if err := s.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM todos
			WHERE user_id = ? AND deleted_at IS NULL
				AND created_at >= ? AND created_at < ?
		`, userID, d.UTC(), end.UTC()).Scan(&b.Created); err != nil {
			return nil, err
		}
		if err := s.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM todos
			WHERE user_id = ? AND deleted_at IS NULL AND is_completed = 1
				AND completed_at IS NOT NULL AND completed_at >= ? AND completed_at < ?
		`, userID, d.UTC(), end.UTC()).Scan(&b.Completed); err != nil {
			return nil, err
		}
		if err := s.DB.QueryRowContext(ctx, `
			SELECT COUNT(*), COALESCE(SUM(actual_duration_seconds), 0)
			FROM pomodoro_sessions
			WHERE user_id = ? AND kind = 'focus'
				AND status IN ('completed','abandoned')
				AND started_at >= ? AND started_at < ?
		`, userID, d.UTC(), end.UTC()).Scan(&b.PomodoroCount, &b.PomodoroSeconds); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// PomodoroTotals 用于 GET /api/stats/pomodoro
type PomodoroTotals struct {
	From          string         `json:"from"`
	To            string         `json:"to"`
	TotalSessions int            `json:"total_sessions"`
	TotalSeconds  int            `json:"total_seconds"`
	ByStatus      map[string]int `json:"by_status"`       // active/completed/abandoned -> count
	ByKind        map[string]int `json:"by_kind_seconds"` // focus/short_break/long_break -> seconds
	Daily         []*DailyBucket `json:"daily"`
}

func (s *StatsStore) PomodoroAggregate(ctx context.Context, userID int64, loc *time.Location, fromDate, toDate time.Time) (*PomodoroTotals, error) {
	if loc == nil {
		loc = time.UTC
	}
	from := startOfLocalDay(fromDate, loc)
	to := startOfLocalDay(toDate, loc)
	if !to.After(from) {
		return &PomodoroTotals{
			From:     from.Format("2006-01-02"),
			To:       to.Format("2006-01-02"),
			ByStatus: map[string]int{},
			ByKind:   map[string]int{},
			Daily:    []*DailyBucket{},
		}, nil
	}

	out := &PomodoroTotals{
		From:     from.Format("2006-01-02"),
		To:       to.Format("2006-01-02"),
		ByStatus: map[string]int{},
		ByKind:   map[string]int{},
	}

	// 总数 / 总秒
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(actual_duration_seconds), 0)
		FROM pomodoro_sessions
		WHERE user_id = ? AND started_at >= ? AND started_at < ?
	`, userID, from.UTC(), to.UTC()).Scan(&out.TotalSessions, &out.TotalSeconds); err != nil {
		return nil, err
	}

	// by_status
	rows, err := s.DB.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM pomodoro_sessions
		WHERE user_id = ? AND started_at >= ? AND started_at < ?
		GROUP BY status
	`, userID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			rows.Close()
			return nil, err
		}
		out.ByStatus[status] = n
	}
	rows.Close()

	// by_kind 秒
	rows2, err := s.DB.QueryContext(ctx, `
		SELECT kind, COALESCE(SUM(actual_duration_seconds), 0) FROM pomodoro_sessions
		WHERE user_id = ? AND started_at >= ? AND started_at < ?
		GROUP BY kind
	`, userID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	for rows2.Next() {
		var kind string
		var sec int
		if err := rows2.Scan(&kind, &sec); err != nil {
			rows2.Close()
			return nil, err
		}
		out.ByKind[kind] = sec
	}
	rows2.Close()

	// 每日明细复用 Daily(只统计 focus 番茄)
	daily, err := s.Daily(ctx, userID, loc, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	out.Daily = daily
	return out, nil
}

// === time helpers ===

func startOfLocalDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// dayRange 给定时刻所在的本地日 [start, end)。
func dayRange(now time.Time, loc *time.Location) (time.Time, time.Time) {
	s := startOfLocalDay(now, loc)
	return s, s.AddDate(0, 0, 1)
}

// startOfLocalWeek 周一为周起点。
func startOfLocalWeek(t time.Time, loc *time.Location) time.Time {
	t = startOfLocalDay(t, loc)
	// Go: Sunday=0..Saturday=6;转 Monday=0..Sunday=6
	dow := (int(t.Weekday()) + 6) % 7
	return t.AddDate(0, 0, -dow)
}

// weekRange 给定时刻所在的本地周 [start, end),周一对齐。
func weekRange(now time.Time, loc *time.Location) (time.Time, time.Time) {
	s := startOfLocalWeek(now, loc)
	return s, s.AddDate(0, 0, 7)
}
