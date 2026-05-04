package handlers

import (
	"net/http"
	"time"

	"github.com/youruser/taskflow/internal/middleware"
	"github.com/youruser/taskflow/internal/store"
)

// StatsHandler 数据复盘(规格 §11 阶段 11)。
//
// 路由约定:
//
//	GET /api/stats/summary                                总览(今日/本周)
//	GET /api/stats/daily?from=YYYY-MM-DD&to=YYYY-MM-DD    按天明细(用户时区)
//	GET /api/stats/weekly?from=YYYY-MM-DD&to=YYYY-MM-DD   按周明细
//	GET /api/stats/pomodoro?from=YYYY-MM-DD&to=YYYY-MM-DD 番茄聚合 + 每日明细
//
// 区间日期是用户时区下的 YYYY-MM-DD,左闭右开:from <= 当天 < to。
// 不传 from/to 时:summary 用今日;daily/weekly/pomodoro 用 [今天-30天, 今天+1天)。
type StatsHandler struct {
	Stats *store.StatsStore
	Users *store.UserStore
}

func NewStatsHandler(s *store.StatsStore, u *store.UserStore) *StatsHandler {
	return &StatsHandler{Stats: s, Users: u}
}

func (h *StatsHandler) loadLoc(r *http.Request, uid int64) *time.Location {
	user, err := h.Users.GetByID(r.Context(), uid)
	if err != nil {
		loc, _ := time.LoadLocation(store.DefaultTimezone)
		return loc
	}
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		loc, _ := time.LoadLocation(store.DefaultTimezone)
		return loc
	}
	return loc
}

func (h *StatsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	loc := h.loadLoc(r, uid)
	out, err := h.Stats.Summary(r.Context(), uid, loc, time.Now())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *StatsHandler) Daily(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	loc := h.loadLoc(r, uid)
	from, to, err := parseDailyRange(r, loc, 30)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	items, err := h.Stats.Daily(r.Context(), uid, loc, from, to)
	if err != nil {
		// 聚合的合法性错(range too large)走 400
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from":  from.Format("2006-01-02"),
		"to":    to.Format("2006-01-02"),
		"items": items,
	})
}

func (h *StatsHandler) Weekly(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	loc := h.loadLoc(r, uid)
	from, to, err := parseDailyRange(r, loc, 12*7)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	items, err := h.Stats.Weekly(r.Context(), uid, loc, from, to)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from":  from.Format("2006-01-02"),
		"to":    to.Format("2006-01-02"),
		"items": items,
	})
}

func (h *StatsHandler) Pomodoro(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	loc := h.loadLoc(r, uid)
	from, to, err := parseDailyRange(r, loc, 30)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	out, err := h.Stats.PomodoroAggregate(r.Context(), uid, loc, from, to)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// parseDailyRange 解析 ?from=YYYY-MM-DD&to=YYYY-MM-DD(用户时区)。
//
// 缺省策略:
//   - 都不传:[today - defaultDays, today + 1)
//   - 只传 from:[from, today + 1)
//   - 只传 to:  [to - defaultDays, to)
//   - 都传:    [from, to)
//
// from/to 都按用户时区 0:00 对齐。
func parseDailyRange(r *http.Request, loc *time.Location, defaultDays int) (time.Time, time.Time, error) {
	q := r.URL.Query()
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	parse := func(s string) (time.Time, error) {
		t, err := time.ParseInLocation("2006-01-02", s, loc)
		if err != nil {
			return time.Time{}, err
		}
		return t, nil
	}

	var (
		from, to time.Time
		err      error
	)
	if v := q.Get("from"); v != "" {
		from, err = parse(v)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if v := q.Get("to"); v != "" {
		to, err = parse(v)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if from.IsZero() && to.IsZero() {
		from = today.AddDate(0, 0, -defaultDays)
		to = today.AddDate(0, 0, 1)
	} else if from.IsZero() {
		from = to.AddDate(0, 0, -defaultDays)
	} else if to.IsZero() {
		to = today.AddDate(0, 0, 1)
		// 如果用户传的 from 比 today+1 还晚,就只取一天
		if !from.Before(to) {
			to = from.AddDate(0, 0, 1)
		}
	}
	return from, to, nil
}
