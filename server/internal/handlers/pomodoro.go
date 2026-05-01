package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/youruser/todoalarm/internal/middleware"
	"github.com/youruser/todoalarm/internal/models"
	"github.com/youruser/todoalarm/internal/store"
)

// PomodoroHandler 番茄专注会话(规格 §11 阶段 11)。
//
// 路由约定:
//
//	GET    /api/pomodoro/sessions                列表 / 过滤 / 分页
//	POST   /api/pomodoro/sessions                开始一个会话(active)
//	GET    /api/pomodoro/sessions/{id}           详情
//	PUT    /api/pomodoro/sessions/{id}           只允许改 note
//	DELETE /api/pomodoro/sessions/{id}           删除
//	POST   /api/pomodoro/sessions/{id}/complete  active -> completed
//	POST   /api/pomodoro/sessions/{id}/abandon   active -> abandoned
type PomodoroHandler struct {
	Pomos *store.PomodoroStore
}

func NewPomodoroHandler(p *store.PomodoroStore) *PomodoroHandler {
	return &PomodoroHandler{Pomos: p}
}

type pomodoroCreateRequest struct {
	TodoID                 *int64 `json:"todo_id"`
	PlannedDurationSeconds int    `json:"planned_duration_seconds"`
	Kind                   string `json:"kind"`
	Note                   string `json:"note"`
}

type pomodoroUpdateRequest struct {
	Note string `json:"note"`
}

func (h *PomodoroHandler) Index(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	q := r.URL.Query()
	f := store.PomodoroFilter{}
	if v := q.Get("todo_id"); v != "" {
		id, err := parseInt64(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid todo_id")
			return
		}
		f.TodoID = &id
	}
	if v := q.Get("status"); v != "" {
		switch v {
		case "active", "completed", "abandoned":
			f.Status = v
		default:
			writeError(w, http.StatusBadRequest, "bad_request", "invalid status")
			return
		}
	}
	if v := q.Get("kind"); v != "" {
		switch v {
		case "focus", "short_break", "long_break", "learning", "review":
			f.Kind = v
		default:
			writeError(w, http.StatusBadRequest, "bad_request", "invalid kind")
			return
		}
	}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid from")
			return
		}
		f.StartedAfter = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid to")
			return
		}
		f.StartedBefore = &t
	}
	if v := q.Get("limit"); v != "" {
		n, err := parseInt64(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid limit")
			return
		}
		f.Limit = int(n)
	}
	if v := q.Get("offset"); v != "" {
		n, err := parseInt64(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid offset")
			return
		}
		f.Offset = int(n)
	}

	items, err := h.Pomos.List(r.Context(), uid, f)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if items == nil {
		items = []*models.PomodoroSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *PomodoroHandler) Show(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	p, err := h.Pomos.Get(r.Context(), uid, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *PomodoroHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	var req pomodoroCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	req.Note = strings.TrimSpace(req.Note)
	if len(req.Note) > 5000 {
		writeError(w, http.StatusBadRequest, "bad_request", "note too long")
		return
	}
	p, err := h.Pomos.Create(r.Context(), uid, store.PomodoroInput{
		TodoID:                 req.TodoID,
		PlannedDurationSeconds: req.PlannedDurationSeconds,
		Kind:                   req.Kind,
		Note:                   req.Note,
	})
	if err != nil {
		// store.Create 用 fmt.Errorf 包装了校验错;特判:只有 ErrNotFound/ErrConflict 走 writeStoreError
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) {
			writeStoreError(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *PomodoroHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req pomodoroUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	p, err := h.Pomos.UpdateNote(r.Context(), uid, id, req.Note)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *PomodoroHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.Pomos.Delete(r.Context(), uid, id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PomodoroHandler) Complete(w http.ResponseWriter, r *http.Request) {
	h.finalize(w, r, true)
}

func (h *PomodoroHandler) Abandon(w http.ResponseWriter, r *http.Request) {
	h.finalize(w, r, false)
}

func (h *PomodoroHandler) finalize(w http.ResponseWriter, r *http.Request, completed bool) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var p *models.PomodoroSession
	if completed {
		p, err = h.Pomos.Complete(r.Context(), uid, id)
	} else {
		p, err = h.Pomos.Abandon(r.Context(), uid, id)
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
