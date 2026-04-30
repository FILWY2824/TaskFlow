package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/youruser/todoalarm/internal/middleware"
	"github.com/youruser/todoalarm/internal/models"
	"github.com/youruser/todoalarm/internal/store"
)

type TodosHandler struct {
	Todos *store.TodoStore
	Users *store.UserStore // 用于解析用户时区,执行 today/tomorrow 等过滤
}

func NewTodosHandler(todos *store.TodoStore, users *store.UserStore) *TodosHandler {
	return &TodosHandler{Todos: todos, Users: users}
}

// todoRequest 用于 Create/Update 的请求体。
// list_id 是 *int64,JSON null 与缺省都视为不放入任何 list。
type todoRequest struct {
	ListID      *int64     `json:"list_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    int        `json:"priority"`
	Effort      int        `json:"effort"`
	DueAt       *time.Time `json:"due_at"`
	DueAllDay   bool       `json:"due_all_day"`
	StartAt     *time.Time `json:"start_at"`
	SortOrder   int        `json:"sort_order"`
	Timezone    string     `json:"timezone"`
}

func (req *todoRequest) validate() error {
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		return errors.New("title required")
	}
	if len(req.Title) > 500 {
		return errors.New("title too long")
	}
	if len(req.Description) > 20000 {
		return errors.New("description too long")
	}
	if req.Priority < 0 || req.Priority > 4 {
		return errors.New("priority must be in [0,4]")
	}
	if req.Effort < 0 || req.Effort > 5 {
		return errors.New("effort must be in [0,5]")
	}
	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			return errors.New("invalid timezone")
		}
	}
	return nil
}

func (req *todoRequest) toInput() store.TodoInput {
	return store.TodoInput{
		ListID:      req.ListID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		Effort:      req.Effort,
		DueAt:       req.DueAt,
		DueAllDay:   req.DueAllDay,
		StartAt:     req.StartAt,
		SortOrder:   req.SortOrder,
		Timezone:    req.Timezone,
	}
}

// Index 列出 todo,支持过滤和分页。
//
// Query 参数:
//
//	filter      = today | tomorrow | this_week | recent_week | recent_month |
//	              overdue | no_date | no_list | completed | scheduled | all
//	list_id     = int
//	due_on      = YYYY-MM-DD（按用户时区取该日的 [00:00, 24:00) 区间）
//	search      = string
//	limit       = int (默认 200, 最大 500)
//	offset      = int
//	order_by    = due_at_asc | created_desc | priority_desc | sort_order
//	include_done= true | false
func (h *TodosHandler) Index(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	q := r.URL.Query()

	f := store.TodoFilter{}

	if v := q.Get("list_id"); v != "" {
		id, err := parseInt64(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid list_id")
			return
		}
		f.ListID = &id
	}
	if v := q.Get("search"); v != "" {
		f.Search = v
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid limit")
			return
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid offset")
			return
		}
		f.Offset = n
	}
	if v := q.Get("order_by"); v != "" {
		f.OrderBy = v
	}
	if v := q.Get("include_done"); v == "true" || v == "1" {
		f.IncludeDone = true
	}

	// due_on=YYYY-MM-DD：按用户时区将该日变成 [00:00, 24:00) 半开区间
	if v := q.Get("due_on"); v != "" {
		user, err := h.Users.GetByID(r.Context(), uid)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		loc, lerr := time.LoadLocation(user.Timezone)
		if lerr != nil {
			loc = time.UTC
		}
		t, perr := time.ParseInLocation("2006-01-02", v, loc)
		if perr != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid due_on (expect YYYY-MM-DD)")
			return
		}
		end := t.AddDate(0, 0, 1)
		f.DueAfter = &t
		f.DueBefore = &end
	}

	// filter 快捷方式需要用户时区做日界换算
	if filterName := q.Get("filter"); filterName != "" {
		user, err := h.Users.GetByID(r.Context(), uid)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		if err := applyFilterShortcut(&f, filterName, user.Timezone, time.Now()); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
	}

	items, err := h.Todos.List(r.Context(), uid, f)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if items == nil {
		items = []*models.Todo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// applyFilterShortcut 把 ?filter= 转成 TodoFilter 字段。
func applyFilterShortcut(f *store.TodoFilter, name, tzName string, now time.Time) error {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		// 用户时区被破坏时不要硬挂,用 UTC 兜底
		loc = time.UTC
	}
	startOfDay := func(t time.Time) time.Time {
		y, m, d := t.In(loc).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	switch name {
	case "today":
		// 今日 = 截止于今日及之前 + 仍未完成（包括过往逾期未做的任务，避免遗漏）。
		// 也就是: due_at < 明日零点 AND (due_at >= 今日零点 OR is_completed = 0)
		s := startOfDay(now)
		e := s.Add(24 * time.Hour)
		f.DueAfter = &s
		f.DueBefore = &e
		f.IncludePastIncomplete = true
	case "tomorrow":
		s := startOfDay(now).Add(24 * time.Hour)
		e := s.Add(24 * time.Hour)
		f.DueAfter = &s
		f.DueBefore = &e
	case "this_week":
		// 周一为本周起点(简化:周日 -> 6 天后)
		t := startOfDay(now)
		// Go: Sunday=0..Saturday=6;转为 Monday=0..Sunday=6
		dow := (int(t.Weekday()) + 6) % 7
		s := t.AddDate(0, 0, -dow)
		e := s.AddDate(0, 0, 7)
		f.DueAfter = &s
		f.DueBefore = &e
		f.IncludePastIncomplete = true
	case "recent_week":
		// 「近一周」：今日起未来 7 天滚动窗口（含今日）+ 过往未完成
		s := startOfDay(now)
		e := s.AddDate(0, 0, 7)
		f.DueAfter = &s
		f.DueBefore = &e
		f.IncludePastIncomplete = true
	case "recent_month":
		// 「近一个月」：今日起未来 30 天滚动窗口（含今日）+ 过往未完成
		s := startOfDay(now)
		e := s.AddDate(0, 0, 30)
		f.DueAfter = &s
		f.DueBefore = &e
		f.IncludePastIncomplete = true
	case "overdue":
		nUTC := now.UTC()
		f.DueBefore = &nUTC
		fa := false
		f.IsCompleted = &fa
	case "no_date":
		f.NoDueDate = true
	case "no_list":
		// 「未分类」：list_id IS NULL
		f.NoList = true
	case "completed":
		t := true
		f.IsCompleted = &t
		f.IncludeDone = true
	case "scheduled":
		// 「日程·全部」：所有"有日期"的任务（无日期任务严格不出现在日程里）。
		// 包含已完成；客户端可以再用 status 筛选缩到"未完成 / 已过期 / 全部"。
		f.IncludeDone = true
		// 通过一个不可能匹配的 NoDueDate=false + 一个非常早的 DueAfter,
		// 让 due_at IS NULL 的记录被排除。这里直接用零年作为下界。
		zero := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		far := time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)
		f.DueAfter = &zero
		f.DueBefore = &far
	case "all":
		f.IncludeDone = true
	default:
		return errors.New("unknown filter: " + name)
	}
	return nil
}

func (h *TodosHandler) Show(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	t, err := h.Todos.Get(r.Context(), uid, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *TodosHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	var req todoRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	// 默认时区跟随用户
	if req.Timezone == "" {
		if user, err := h.Users.GetByID(r.Context(), uid); err == nil {
			req.Timezone = user.Timezone
		}
	}
	out, err := h.Todos.Create(r.Context(), uid, req.toInput())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *TodosHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req todoRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Timezone == "" {
		if user, err := h.Users.GetByID(r.Context(), uid); err == nil {
			req.Timezone = user.Timezone
		}
	}
	out, err := h.Todos.Update(r.Context(), uid, id, req.toInput())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *TodosHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.Todos.Delete(r.Context(), uid, id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TodosHandler) Complete(w http.ResponseWriter, r *http.Request) {
	h.setCompleted(w, r, true)
}

func (h *TodosHandler) Uncomplete(w http.ResponseWriter, r *http.Request) {
	h.setCompleted(w, r, false)
}

func (h *TodosHandler) setCompleted(w http.ResponseWriter, r *http.Request, completed bool) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	out, err := h.Todos.SetCompleted(r.Context(), uid, id, completed)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
