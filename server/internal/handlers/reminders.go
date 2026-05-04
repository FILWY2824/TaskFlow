package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/youruser/taskflow/internal/middleware"
	"github.com/youruser/taskflow/internal/models"
	"github.com/youruser/taskflow/internal/store"
)

type RemindersHandler struct {
	Reminders *store.ReminderStore
	Users     *store.UserStore
}

func NewRemindersHandler(r *store.ReminderStore, u *store.UserStore) *RemindersHandler {
	return &RemindersHandler{Reminders: r, Users: u}
}

type reminderRequest struct {
	TodoID          *int64     `json:"todo_id"`
	Title           string     `json:"title"`
	TriggerAt       *time.Time `json:"trigger_at"`
	RRule           string     `json:"rrule"`
	DTStart         *time.Time `json:"dtstart"`
	Timezone        string     `json:"timezone"`
	ChannelLocal    *bool      `json:"channel_local"`
	ChannelTelegram *bool      `json:"channel_telegram"`
	ChannelWebPush  *bool      `json:"channel_web_push"`
	IsEnabled       *bool      `json:"is_enabled"`
	Ringtone        string     `json:"ringtone"`
	Vibrate         *bool      `json:"vibrate"`
	Fullscreen      *bool      `json:"fullscreen"`
}

func (req *reminderRequest) validate() error {
	req.Title = strings.TrimSpace(req.Title)
	if len(req.Title) > 200 {
		return errors.New("title too long")
	}
	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			return errors.New("invalid timezone")
		}
	}
	if len(req.Ringtone) > 100 {
		return errors.New("ringtone too long")
	}
	return nil
}

// toInput 把请求结构体转成 store 层输入。
// 默认值:channel_local=true(其他 channel 默认 false),is_enabled=true,vibrate=true,fullscreen=true。
func (req *reminderRequest) toInput(defaultTZ string) store.ReminderInput {
	in := store.ReminderInput{
		TodoID:    req.TodoID,
		Title:     req.Title,
		TriggerAt: req.TriggerAt,
		RRule:     req.RRule,
		DTStart:   req.DTStart,
		Timezone:  req.Timezone,
		Ringtone:  req.Ringtone,
	}
	if in.Timezone == "" {
		in.Timezone = defaultTZ
	}
	if req.ChannelLocal != nil {
		in.ChannelLocal = *req.ChannelLocal
	} else {
		in.ChannelLocal = true
	}
	if req.ChannelTelegram != nil {
		in.ChannelTelegram = *req.ChannelTelegram
	}
	if req.ChannelWebPush != nil {
		in.ChannelWebPush = *req.ChannelWebPush
	}
	if req.IsEnabled != nil {
		in.IsEnabled = *req.IsEnabled
	} else {
		in.IsEnabled = true
	}
	if req.Vibrate != nil {
		in.Vibrate = *req.Vibrate
	} else {
		in.Vibrate = true
	}
	if req.Fullscreen != nil {
		in.Fullscreen = *req.Fullscreen
	} else {
		in.Fullscreen = true
	}
	if in.Ringtone == "" {
		in.Ringtone = "default"
	}
	return in
}

// Index GET /api/reminders?todo_id=&only_enabled=
func (h *RemindersHandler) Index(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	q := r.URL.Query()

	f := store.ReminderFilter{}
	if v := q.Get("todo_id"); v != "" {
		id, err := parseInt64(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid todo_id")
			return
		}
		f.TodoID = &id
	}
	if v := q.Get("only_enabled"); v == "true" || v == "1" {
		f.OnlyEnabled = true
	}
	items, err := h.Reminders.List(r.Context(), uid, f)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if items == nil {
		items = []*models.ReminderRule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Show GET /api/reminders/{id}
func (h *RemindersHandler) Show(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	out, err := h.Reminders.Get(r.Context(), uid, id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Create POST /api/reminders
func (h *RemindersHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	var req reminderRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	defaultTZ := store.DefaultTimezone
	if user, err := h.Users.GetByID(r.Context(), uid); err == nil {
		defaultTZ = user.Timezone
	}
	out, err := h.Reminders.Create(r.Context(), uid, req.toInput(defaultTZ))
	if err != nil {
		// 业务校验错误(无 channel、缺少 dtstart 等)归为 400
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// Update PUT /api/reminders/{id}
func (h *RemindersHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req reminderRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	defaultTZ := store.DefaultTimezone
	if user, err := h.Users.GetByID(r.Context(), uid); err == nil {
		defaultTZ = user.Timezone
	}
	out, err := h.Reminders.Update(r.Context(), uid, id, req.toInput(defaultTZ))
	if err != nil {
		// 区分 not found / 业务校验
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Delete DELETE /api/reminders/{id}
func (h *RemindersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.Reminders.Delete(r.Context(), uid, id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Enable POST /api/reminders/{id}/enable
func (h *RemindersHandler) Enable(w http.ResponseWriter, r *http.Request) {
	h.setEnabled(w, r, true)
}

// Disable POST /api/reminders/{id}/disable
func (h *RemindersHandler) Disable(w http.ResponseWriter, r *http.Request) {
	h.setEnabled(w, r, false)
}

func (h *RemindersHandler) setEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	uid := middleware.UserIDFrom(r.Context())
	id, err := parseInt64(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	out, err := h.Reminders.SetEnabled(r.Context(), uid, id, enabled)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
